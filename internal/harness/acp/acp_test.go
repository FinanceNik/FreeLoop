package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/acp/acptest"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

func TestMain(m *testing.M) {
	if acptest.Serve() {
		return
	}
	os.Exit(m.Run())
}

type recorder struct {
	mu     sync.Mutex
	events []event.Event
}

func (r *recorder) emit(typ event.Type, data any) {
	b, _ := json.Marshal(data)
	r.mu.Lock()
	r.events = append(r.events, event.Event{Type: typ, Data: b})
	r.mu.Unlock()
}

func (r *recorder) text(typ event.Type) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var sb strings.Builder
	for _, e := range r.events {
		if e.Type == typ {
			sb.Write(e.Data)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// HRN-005, MCP-004: a reference agent edits a file with the edit approved
// through the broker, receives the session's MCP servers, and a terminal
// request it did not ask permission for goes to the broker too.
func TestReferenceAgent(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"}}}
	s, err := h.NewSession(context.Background(), harness.Params{
		SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeAsk, Emit: rec.emit,
		MCPServers: []harness.MCPServerSpec{
			{Name: "github", Transport: "stdio", Command: "gh-mcp", Env: map[string]string{"TOKEN": "x"}},
			{Name: "remote", Transport: "http", URL: "https://example.com/mcp"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	stop := make(chan struct{})
	defer close(stop)
	var mu sync.Mutex
	var asked []permission.Request
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
			}
			for _, r := range broker.Pending() {
				mu.Lock()
				asked = append(asked, r)
				mu.Unlock()
				opt := r.Options[0].ID
				if r.Tool == "shell" {
					opt = "allow_once"
				}
				broker.Decide(r.ID, opt, "", "")
			}
		}
	}()
	reason, err := s.Prompt(context.Background(), harness.Prompt{Text: "edit and use the terminal"})
	if err != nil || reason != harness.StopEndTurn {
		t.Fatalf("Prompt = %s, %v", reason, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "notes.txt")); string(b) != "hello edited" {
		t.Errorf("notes.txt = %q", b)
	}
	mu.Lock()
	if len(asked) != 2 || asked[0].Tool != "edit_file" || asked[0].Paths[0] != filepath.Join(env.Root(), "notes.txt") || asked[1].Tool != "shell" || asked[1].Command != "sh -c echo from-terminal" {
		t.Errorf("asked = %+v", asked)
	}
	mu.Unlock()
	msgs := rec.text(event.AgentMessageChunk)
	if !strings.Contains(msgs, "mcp: github=gh-mcp,remote=https://example.com/mcp") || !strings.Contains(msgs, "terminal: from-terminal") {
		t.Errorf("agent said %s", msgs)
	}
	if calls := rec.text(event.ToolCall); !strings.Contains(calls, `"tool":"edit_file"`) || strings.Contains(calls, "sessionUpdate") {
		t.Errorf("tool_call events = %s", calls)
	}
	if !strings.Contains(rec.text(event.UsageUpdate), `"inputTokens":10`) || !strings.Contains(rec.text(event.TurnEnded), "end_turn") {
		t.Error("usage or turn end missing")
	}
}

// A write the agent did not ask permission for is refused when the user
// denies it, and a protected path is refused by the environment anyway.
func TestUnaskedWritesGoThroughTheBroker(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, _ := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root)})
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	s := &session{p: harness.Params{SessionID: "x", Broker: broker, Emit: rec.emit}, env: env, mode: permission.ModeAsk, terms: map[string]*terminal{}}
	go func() {
		for range 200 {
			for _, r := range broker.Pending() {
				broker.Decide(r.ID, "reject_once", "", "")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	if _, err := s.WriteTextFile(context.Background(), sdk.WriteTextFileRequest{Path: filepath.Join(root, "x.txt"), Content: "x"}); err == nil {
		t.Error("an unasked write was allowed")
	}
	if _, err := os.Stat(filepath.Join(root, "x.txt")); err == nil {
		t.Error("the file was written")
	}
	if _, err := s.WriteTextFile(context.Background(), sdk.WriteTextFileRequest{Path: filepath.Join(root, ".git", "config"), Content: "x"}); err == nil {
		t.Error("a write into .git was allowed")
	}
}
