package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/acp/acptest"
	"github.com/FinanceNik/Freeloop/internal/mcp"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

type echoIn struct {
	Text string `json:"text"`
}

// The engine's test binary doubles as a stdio MCP server whose echo tool
// repeats its text and the TOKEN secret.
func TestMain(m *testing.M) {
	if acptest.Serve() {
		return
	}
	if os.Getenv("FREELOOP_OAUTH_TEST_SERVER") != "" {
		newFakeOAuth(nil)
		select {}
	}
	if os.Getenv("FREELOOP_MCP_TEST_SERVER") == "1" {
		s := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
		sdk.AddTool(s, &sdk.Tool{Name: "echo", Description: "Echo text."},
			func(ctx context.Context, req *sdk.CallToolRequest, in echoIn) (*sdk.CallToolResult, struct{}, error) {
				return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "echo: " + in.Text + " " + os.Getenv("TOKEN")}}}, struct{}{}, nil
			})
		// deploy asks the user where to deploy (MCP-008).
		sdk.AddTool(s, &sdk.Tool{Name: "deploy", Description: "Deploy."},
			func(_ context.Context, req *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, struct{}, error) {
				// The 2026-07-28 revision's round trip: ask with
				// InputRequests, finish when the call comes back answered.
				if len(req.Params.InputResponses) == 0 {
					return &sdk.CallToolResult{InputRequests: sdk.InputRequestMap{"where": &sdk.ElicitParams{
						Message: "Where to?", RequestedSchema: json.RawMessage(`{"type":"object","properties":{"env":{"type":"string","enum":["staging","production"]}},"required":["env"]}`),
					}}}, struct{}{}, nil
				}
				text := "deploy declined"
				if res, ok := req.Params.InputResponses["where"].(*sdk.ElicitResult); ok && res.Action == "accept" {
					text = fmt.Sprintf("deploying to %v", res.Content["env"])
				}
				return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}}, struct{}{}, nil
			})
		s.AddResource(&sdk.Resource{URI: "notes://readme", Name: "readme"},
			func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
				return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: req.Params.URI, Text: "Deploys go out on Tuesdays."}}}, nil
			})
		s.AddPrompt(&sdk.Prompt{Name: "review", Arguments: []*sdk.PromptArgument{{Name: "file", Required: true}}},
			func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
				return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{{Role: "user", Content: &sdk.TextContent{Text: "Review " + req.Params.Arguments["file"] + " carefully."}}}}, nil
			})
		s.Run(context.Background(), &sdk.StdioTransport{})
		return
	}
	os.Exit(m.Run())
}

func hasTool(req provider.ChatRequest, name string) bool {
	return slices.ContainsFunc(req.Tools, func(t provider.ToolSpec) bool { return t.Name == name })
}

// MCP-001, MCP-003, MCP-005: a server runs only after its configuration is
// approved, its tools reach the built-in harness namespaced, a rule allow
// mcp__test__* approves them, its secret is redacted, and a change to the
// configuration needs a new approval.
func TestMCPServersInSessions(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	prov := scripted.New("p",
		scripted.Step{Calls: []scripted.Call{{Name: "mcp__test__echo", Input: map[string]any{"text": "hello"}}}},
		scripted.Step{Text: "done"},
	)
	e.mu.Lock()
	e.providers.built["p"] = &configuredProvider{cfg: config.Provider{ID: "p", Name: "p", Type: "scripted"}, p: prov}
	e.mu.Unlock()

	exe, _ := os.Executable()
	info, err := e.SaveMCPServer(SaveMCPServerRequest{
		Server:  config.MCPServer{ID: "test", Name: "Test", Transport: "stdio", Command: exe, Env: map[string]string{"FREELOOP_MCP_TEST_SERVER": "1"}, Enabled: true},
		Secrets: map[string]string{"TOKEN": "tok-9f8e7d6c5b4a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !info.NeedsApproval || !slices.Equal(info.SecretNames, []string{"TOKEN"}) {
		t.Fatalf("saved = %+v", info)
	}
	b, _ := os.ReadFile(e.dataDir + "/" + config.MCPFileName)
	if strings.Contains(string(b), "tok-9f8e7d6c5b4a") {
		t.Fatal("the secret is in mcp.yaml")
	}
	if err := e.StartMCPServer(ctx, "test"); err == nil {
		t.Fatal("an unapproved server started")
	}

	// Unapproved: the session sees no MCP tools.
	ses, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "p", Model: "m"})
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)
	if hasTool(prov.Requests()[0], "mcp__test__echo") {
		t.Fatal("an unapproved server's tool was offered")
	}

	if _, err := e.ApproveMCPServer(ctx, "test", "stale"); err == nil {
		t.Error("approved a fingerprint that is not the current one")
	}
	if info, err = e.ApproveMCPServer(ctx, "test", info.Fingerprint); err != nil || info.Status.State != mcp.Running || len(info.Status.Tools) != 2 {
		t.Fatalf("approve = %+v, %v", info, err)
	}

	// Approved and enabled: the tool is offered and asks, unless a rule
	// allows the server's namespace.
	if err := e.AddRule(permission.Rule{Effect: permission.Allow, Tool: "mcp__test__*"}); err != nil {
		t.Fatal(err)
	}
	n := len(prov.Requests())
	e.Prompt(ctx, ses.ID, "go again")
	waitIdle(t, ch, ses.ID)
	reqs := prov.Requests()
	if !hasTool(reqs[n], "mcp__test__echo") {
		t.Fatalf("tools = %v", toolNames(reqs[n]))
	}
	if got := lastToolResult(reqs[len(reqs)-1]); !strings.HasPrefix(got, "echo: hello") {
		t.Errorf("tool result = %q", got)
	}
	if !decided(t, e, ses.ID, permission.Allowed) {
		t.Error("the rule did not allow the MCP tool")
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	for _, ev := range evs {
		if strings.Contains(string(ev.Data), "tok-9f8e7d6c5b4a") {
			t.Fatalf("the secret reached the transcript in %s", ev.Type)
		}
	}
	if !slices.ContainsFunc(evs, func(ev event.Event) bool { return strings.Contains(string(ev.Data), "[REDACTED]") }) {
		t.Error("the redacted secret is not in the transcript")
	}

	// The session source lists the running server's tool.
	if sm := (sessionMCP{e: e, projectID: ses.ProjectID, root: ses.ProjectRoot}); len(sm.Tools()) != 2 {
		t.Errorf("session tools = %+v", sm.Tools())
	}

	// A changed configuration stops the server and asks again.
	srv, _ := e.mcpServer("test")
	srv.Args = []string{"--changed"}
	if info, err = e.SaveMCPServer(SaveMCPServerRequest{Server: srv}); err != nil || !info.NeedsApproval || info.Status.State == mcp.Running {
		t.Fatalf("changed = %+v, %v", info, err)
	}
	if !slices.Equal(info.SecretNames, []string{"TOKEN"}) {
		t.Error("saving without secrets dropped the stored one")
	}
	if err := e.DeleteMCPServer("test"); err != nil {
		t.Fatal(err)
	}
	if len(e.ListMCPServers()) != 0 {
		t.Error("the server is still listed")
	}
}

func TestMCPToolNames(t *testing.T) {
	if got := mcpToolName("github", "create.issue"); got != "mcp__github__create_issue" {
		t.Errorf("name = %q", got)
	}
	long := mcpToolName("server", strings.Repeat("x", 80))
	if len(long) != 64 || !strings.HasPrefix(long, "mcp__server__") || long == mcpToolName("server", strings.Repeat("x", 81)) {
		t.Errorf("long name = %q", long)
	}
}

// MCP-008: a server's request for input during a tool call is a form on
// that call, asked even when a rule allows the tool; the answers go back to
// the server, which finishes the call with them, and declining tells it so.
func TestMCPInputRequests(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	prov := scripted.New("p",
		scripted.Step{Calls: []scripted.Call{{Name: "mcp__test__deploy", Input: map[string]any{}}}},
		scripted.Step{Text: "done"},
	)
	e.mu.Lock()
	e.providers.built["p"] = &configuredProvider{cfg: config.Provider{ID: "p", Name: "p", Type: "scripted"}, p: prov}
	e.mu.Unlock()
	exe, _ := os.Executable()
	info, err := e.SaveMCPServer(SaveMCPServerRequest{Server: config.MCPServer{ID: "test", Name: "Test", Transport: "stdio", Command: exe, Env: map[string]string{"FREELOOP_MCP_TEST_SERVER": "1"}, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ApproveMCPServer(ctx, "test", info.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if err := e.AddRule(permission.Rule{Effect: permission.Allow, Tool: "mcp__test__*"}); err != nil {
		t.Fatal(err)
	}
	ses, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "p", Model: "m"})

	answer := func(option string, answers map[string]any) <-chan permission.Request {
		got := make(chan permission.Request, 1)
		go func() {
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				for _, r := range e.PendingPermissions() {
					if r.SessionID == ses.ID {
						got <- r
						e.Decide(DecideRequest{RequestID: r.ID, OptionID: option, Answers: answers})
						return
					}
				}
				time.Sleep(time.Millisecond)
			}
			close(got)
		}()
		return got
	}

	got := answer("accept", map[string]any{"env": "staging"})
	e.Prompt(ctx, ses.ID, "deploy")
	waitIdle(t, ch, ses.ID)
	r, ok := <-got
	if !ok {
		t.Fatalf("the server's input request was not asked; tool result = %q", lastToolResult(prov.Requests()[len(prov.Requests())-1]))
	}
	if r.Form == nil || r.Form.Server != "test" || r.Form.Message != "Where to?" || !strings.Contains(string(r.Form.Schema), "production") {
		t.Fatalf("form = %+v", r.Form)
	}
	if r.Tool != "mcp__test__input" || r.ToolCallID == "" {
		t.Errorf("request = %+v", r)
	}
	if got := lastToolResult(prov.Requests()[len(prov.Requests())-1]); got != "deploying to staging" {
		t.Errorf("accepted: tool result = %q", got)
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	for _, ev := range evs {
		if ev.Type == event.PermissionRequested && strings.Contains(string(ev.Data), "staging") && !strings.Contains(string(ev.Data), "production") {
			t.Errorf("the answer reached the transcript: %s", ev.Data)
		}
		if ev.Type == event.PermissionDecided && strings.Contains(string(ev.Data), "staging") {
			t.Errorf("the answer reached the transcript: %s", ev.Data)
		}
	}

	got = answer("decline", nil)
	e.Prompt(ctx, ses.ID, "deploy again")
	waitIdle(t, ch, ses.ID)
	if _, ok := <-got; !ok {
		t.Fatal("the second input request was not asked")
	}
	if got := lastToolResult(prov.Requests()[len(prov.Requests())-1]); got != "deploy declined" {
		t.Errorf("declined: tool result = %q", got)
	}
}
