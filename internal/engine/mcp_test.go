package engine

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

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
	if os.Getenv("FREELOOP_MCP_TEST_SERVER") == "1" {
		s := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
		sdk.AddTool(s, &sdk.Tool{Name: "echo", Description: "Echo text."},
			func(ctx context.Context, req *sdk.CallToolRequest, in echoIn) (*sdk.CallToolResult, struct{}, error) {
				return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "echo: " + in.Text + " " + os.Getenv("TOKEN")}}}, struct{}{}, nil
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
	if info, err = e.ApproveMCPServer(ctx, "test", info.Fingerprint); err != nil || info.Status.State != mcp.Running || len(info.Status.Tools) != 1 {
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
	if sm := (sessionMCP{e: e, projectID: ses.ProjectID, root: ses.ProjectRoot}); len(sm.Tools()) != 1 {
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
