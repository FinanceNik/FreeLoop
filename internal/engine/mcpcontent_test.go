package engine

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

func lastUserText(req provider.ChatRequest) string {
	var sb strings.Builder
	for _, p := range req.Messages[len(req.Messages)-1].Parts {
		sb.WriteString(p.Text)
		sb.WriteString("\n")
	}
	return sb.String()
}

// MCP-007: a session lists its servers' resources and prompts; an attached
// resource's content is in the next user turn and the transcript names it;
// a prompt runs as the user's message; servers the session does not use
// are refused.
func TestMCPResourcesAndPrompts(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	prov := scripted.New("p", scripted.Step{Text: "one"}, scripted.Step{Text: "two"}, scripted.Step{Text: "three"})
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
	root := t.TempDir()
	os.WriteFile(root+"/secret.txt", []byte("the vault code is 0451"), 0o644)
	ses, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "p", Model: "m"})

	mc, err := e.SessionMCP(ctx, ses.ID)
	if err != nil || len(mc.Resources) != 1 || mc.Resources[0].URI != "notes://readme" || len(mc.Prompts) != 1 ||
		mc.Prompts[0].Command != "mcp__test__review" || !mc.Prompts[0].Arguments[0].Required {
		t.Fatalf("context = %+v, %v", mc, err)
	}

	if _, err := e.PromptWith(ctx, ses.ID, PromptRequest{Text: "When do we deploy?", Resources: []ResourceRef{{Server: "test", URI: "notes://readme", Name: "readme"}, {Server: "other", URI: "x://y"}}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	got := lastUserText(prov.Requests()[0])
	if !strings.Contains(got, "When do we deploy?") || !strings.Contains(got, `<resource server="test" uri="notes://readme">`) || !strings.Contains(got, "Deploys go out on Tuesdays.") {
		t.Errorf("user turn = %q", got)
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	var linked, skipped bool
	for _, ev := range evs {
		if ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), `"name":"test: readme"`) {
			linked = true
		}
		if ev.Type == event.Notice && strings.Contains(string(ev.Data), "other: x://y") {
			skipped = true
		}
	}
	if !linked || !skipped {
		t.Errorf("transcript: resource link %v, notice for the other server %v", linked, skipped)
	}

	if _, err := e.RunMCPPrompt(ctx, ses.ID, "test", "review", map[string]string{"file": "a.go"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	if got := lastUserText(prov.Requests()[1]); !strings.Contains(got, "Review a.go carefully.") {
		t.Errorf("prompt turn = %q", got)
	}
	// The server's text is not the user's: an @ in it attaches no file.
	if _, err := e.RunMCPPrompt(ctx, ses.ID, "test", "review", map[string]string{"file": "@secret.txt"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	if got := lastUserText(prov.Requests()[2]); strings.Contains(got, "0451") || !strings.Contains(got, "Review @secret.txt carefully.") {
		t.Errorf("prompt turn with an @ = %q", got)
	}
	if _, err := e.RunMCPPrompt(ctx, ses.ID, "other", "review", nil); err == nil {
		t.Error("ran a prompt of a server the session does not use")
	}
}
