package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
)

type echoIn struct {
	Text string `json:"text"`
}

type echoOut struct{}

// testServer offers echo, which repeats its text and the TOKEN variable,
// a notes://readme resource and a review prompt (MCP-007).
func testServer() *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	s.AddResource(&sdk.Resource{URI: "notes://readme", Name: "readme", Description: "The team's notes.", MIMEType: "text/plain"},
		func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: req.Params.URI, MIMEType: "text/plain", Text: "Deploys go out on Tuesdays."}}}, nil
		})
	s.AddPrompt(&sdk.Prompt{Name: "review", Description: "Review a file.", Arguments: []*sdk.PromptArgument{{Name: "file", Description: "The file to review", Required: true}, {Name: "focus"}}},
		func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
			return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{
				{Role: "user", Content: &sdk.TextContent{Text: "Review " + req.Params.Arguments["file"] + " for " + req.Params.Arguments["focus"] + "."}},
				{Role: "user", Content: &sdk.EmbeddedResource{Resource: &sdk.ResourceContents{URI: "notes://style", Text: "Prefer small functions."}}},
			}}, nil
		})
	sdk.AddTool(s, &sdk.Tool{Name: "echo", Description: "Echo text.", Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true}},
		func(ctx context.Context, req *sdk.CallToolRequest, in echoIn) (*sdk.CallToolResult, echoOut, error) {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: in.Text + " " + os.Getenv("TOKEN")}}}, echoOut{}, nil
		})
	return s
}

// The test binary doubles as a stdio MCP server.
func TestMain(m *testing.M) {
	if os.Getenv("FREELOOP_MCP_TEST_SERVER") == "1" {
		testServer().Run(context.Background(), &sdk.StdioTransport{})
		return
	}
	os.Exit(m.Run())
}

type allow struct{}

func (allow) CheckRead(string) error  { return nil }
func (allow) CheckWrite(string) error { return nil }

func TestStdioServer(t *testing.T) {
	env, err := environment.NewLocal(environment.LocalOptions{Root: t.TempDir(), Guard: allow{}, Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	m := New(env)
	defer m.StopAll()
	exe, _ := os.Executable()
	l := Launch{
		Config: config.MCPServer{ID: "test", Transport: "stdio", Command: exe, Env: map[string]string{"FREELOOP_MCP_TEST_SERVER": "1"}},
		Env:    map[string]string{"TOKEN": "from-keychain"},
	}
	ctx := context.Background()
	start := time.Now()
	if err := m.Start(ctx, l); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("tools took %s", time.Since(start))
	}
	st := m.Status("test")
	if st.State != Running || len(st.Tools) != 1 || st.Tools[0].Name != "echo" || !st.Tools[0].ReadOnly || st.Protocol == "" {
		t.Fatalf("status = %+v", st)
	}
	res, err := m.Call(ctx, "test", "echo", []byte(`{"text":"hi"}`))
	if err != nil || res.Text != "hi from-keychain" || res.IsError {
		t.Errorf("call = %+v, %v", res, err)
	}
	if _, err := m.Call(ctx, "test", "missing", nil); err == nil {
		if r, _ := m.Call(ctx, "test", "missing", nil); !r.IsError {
			t.Error("an unknown tool succeeded")
		}
	}
	m.Stop("test")
	if st := m.Status("test"); st.State != Stopped {
		t.Errorf("after stop = %+v", st)
	}

	// A command that exits reports its error output.
	bad := Launch{Config: config.MCPServer{ID: "bad", Transport: "stdio", Command: "sh", Args: []string{"-c", "echo boom >&2; exit 3"}}}
	if err := m.Start(ctx, bad); err == nil || !strings.Contains(m.Status("bad").Error, "boom") {
		t.Errorf("failed start = %v, %+v", err, m.Status("bad"))
	}
}

func TestHTTPServerGetsHeaders(t *testing.T) {
	var auth string
	h := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		auth = r.Header.Get("Authorization")
		return testServer()
	}, nil)
	srv := httptest.NewServer(h)
	defer srv.Close()
	m := New(nil)
	defer m.StopAll()
	l := Launch{Config: config.MCPServer{ID: "remote", Transport: "http", URL: srv.URL, Headers: map[string]string{"X-Plain": "1"}}, Headers: map[string]string{"Authorization": "Bearer secret"}}
	if err := m.Start(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	if st := m.Status("remote"); st.State != Running || len(st.Tools) != 1 {
		t.Fatalf("status = %+v", st)
	}
	if auth != "Bearer secret" {
		t.Errorf("Authorization = %q", auth)
	}
}

func TestWarning(t *testing.T) {
	for _, c := range []struct {
		s    config.MCPServer
		warn bool
	}{
		{config.MCPServer{Transport: "stdio", Command: "npx", Args: []string{"-y", "@x/server"}}, true},
		{config.MCPServer{Transport: "stdio", Command: "/opt/homebrew/bin/uvx", Args: []string{"mcp-server-git"}}, true},
		{config.MCPServer{Transport: "stdio", Command: "docker", Args: []string{"run", "-i", "ghcr.io/x/y"}}, true},
		{config.MCPServer{Transport: "stdio", Command: "/usr/local/bin/my-server"}, false},
		{config.MCPServer{Transport: "http", URL: "https://x"}, false},
	} {
		if got := Warning(c.s) != ""; got != c.warn {
			t.Errorf("Warning(%s %v) = %v", c.s.Command, c.s.Args, got)
		}
	}
}

// MCP-007: a server's resources and prompts are listed, a resource is
// read as text and a prompt renders with its arguments.
func TestResourcesAndPrompts(t *testing.T) {
	env, err := environment.NewLocal(environment.LocalOptions{Root: t.TempDir(), Guard: allow{}, Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	m := New(env)
	defer m.StopAll()
	exe, _ := os.Executable()
	ctx := context.Background()
	if err := m.Start(ctx, Launch{Config: config.MCPServer{ID: "test", Transport: "stdio", Command: exe, Env: map[string]string{"FREELOOP_MCP_TEST_SERVER": "1"}}}); err != nil {
		t.Fatal(err)
	}
	res, err := m.Resources(ctx, "test")
	if err != nil || len(res) != 1 || res[0].URI != "notes://readme" || res[0].Description != "The team's notes." {
		t.Fatalf("resources = %+v, %v", res, err)
	}
	text, err := m.ReadResource(ctx, "test", "notes://readme")
	if err != nil || text != "Deploys go out on Tuesdays." {
		t.Errorf("read = %q, %v", text, err)
	}
	prompts, err := m.Prompts(ctx, "test")
	if err != nil || len(prompts) != 1 || len(prompts[0].Arguments) != 2 || !prompts[0].Arguments[0].Required || prompts[0].Arguments[1].Required {
		t.Fatalf("prompts = %+v, %v", prompts, err)
	}
	p, err := m.GetPrompt(ctx, "test", "review", map[string]string{"file": "a.go", "focus": "errors"})
	if err != nil || p.Text != "Review a.go for errors." || len(p.Resources) != 1 || p.Resources[0].Text != "Prefer small functions." {
		t.Errorf("prompt = %+v, %v", p, err)
	}
	if _, err := m.Resources(ctx, "nope"); err == nil {
		t.Error("a server that does not run listed resources")
	}
}

// A server's configured headers, which can hold secrets, go to its own host
// only, not to one it redirects to.
func TestHeadersStayOnTheServersHost(t *testing.T) {
	var leaked atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNotFound)
	}))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/mcp", http.StatusTemporaryRedirect)
	}))
	defer first.Close()
	client := &http.Client{Transport: headers{set: map[string]string{"Authorization": "Bearer secret"}, host: hostOf(first.URL), next: http.DefaultTransport}}
	resp, err := client.Get(first.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if v, _ := leaked.Load().(string); v != "" {
		t.Errorf("the redirect target got %q", v)
	}
}
