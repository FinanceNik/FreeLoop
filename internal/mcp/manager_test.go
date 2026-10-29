package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

// testServer offers echo, which repeats its text and the TOKEN variable.
func testServer() *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
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
