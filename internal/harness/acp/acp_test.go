package acp

import (
	"context"
	"encoding/json"
	"errors"
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

// Review finding: an agent's current_mode_update was taken as it came, so it
// could put the session in bypass and approve itself. It can tighten the mode;
// it loosens it only after the user approved its request, and never to bypass.
func TestAgentCannotLoosenTheMode(t *testing.T) {
	rec := &recorder{}
	s := &session{p: harness.Params{SessionID: "x", Emit: rec.emit}, mode: permission.ModePlan, preset: config.HarnessPreset{Name: "Agent"}}
	for _, m := range []permission.Mode{permission.ModeBypass, permission.ModeAcceptEdits, permission.ModeAsk} {
		s.agentMode(m)
		if s.Mode() != permission.ModePlan {
			t.Fatalf("the agent changed plan mode to %s on its own", m)
		}
	}
	if strings.Contains(rec.text(event.CurrentModeUpdate), "bypass") {
		t.Error("the refused change reached the transcript")
	}
	if !strings.Contains(rec.text(event.Notice), "on its own") {
		t.Error("the user was not told")
	}

	s.approved = approval{kind: permission.KindSwitchMode, until: time.Now().Add(time.Minute)}
	s.agentMode(permission.ModeBypass)
	if s.Mode() != permission.ModePlan {
		t.Error("an approved switch reached bypass")
	}
	s.agentMode(permission.ModeAcceptEdits)
	if s.Mode() != permission.ModeAcceptEdits {
		t.Errorf("an approved switch left the mode %s", s.Mode())
	}
	s.agentMode(permission.ModeBypass)
	if s.Mode() != permission.ModeAcceptEdits {
		t.Error("the approval was used twice")
	}
	s.approved = approval{kind: permission.KindEdit, until: time.Now().Add(time.Minute)}
	s.mode = permission.ModePlan
	s.agentMode(permission.ModeAsk)
	if s.Mode() != permission.ModePlan {
		t.Error("an approval of something else loosened the mode")
	}

	s.mode = permission.ModeAcceptEdits
	s.agentMode(permission.ModePlan)
	if s.Mode() != permission.ModePlan {
		t.Error("the agent could not tighten the mode")
	}
}

// HRN-007: several of the app's modes can share one of the agent's, and a mode
// the preset does not name changes nothing.
func TestAgentReportsItsMode(t *testing.T) {
	rec := &recorder{}
	preset := config.HarnessPreset{Name: "Agent", Modes: map[string]string{"ask": "asks", "accept_edits": "asks", "plan": "reads", "bypass": "asks"}}
	s := &session{p: harness.Params{SessionID: "x", Emit: rec.emit}, mode: permission.ModeAcceptEdits, preset: preset}
	s.agentReported("asks")
	if s.Mode() != permission.ModeAcceptEdits {
		t.Errorf("reporting the mode it is in left %s", s.Mode())
	}
	s.mode = permission.ModePlan
	s.agentReported("asks")
	if s.Mode() != permission.ModePlan {
		t.Errorf("a loosening report took effect: %s", s.Mode())
	}
	s.mode = permission.ModeAsk
	s.agentReported("reads")
	if s.Mode() != permission.ModePlan {
		t.Errorf("tightening left %s", s.Mode())
	}
	// With no mapping for it there is no agent to call back, so this ends here.
	s.preset.Modes = map[string]string{"ask": "asks"}
	s.mode = permission.ModeAsk
	s.agentReported("yolo")
	if s.Mode() != permission.ModeAsk || !strings.Contains(rec.text(event.Notice), "yolo") {
		t.Errorf("an unnamed mode: %s, %s", s.Mode(), rec.text(event.Notice))
	}
}

// HRN-007: a file that is not there is the protocol's "not found", which is
// what an agent about to create it checks for, and not a failure.
func TestReadingAMissingFile(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, _ := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root)})
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	s := &session{p: harness.Params{SessionID: "x", Broker: broker, Emit: rec.emit}, env: env, mode: permission.ModeAsk, terms: map[string]*terminal{}}
	_, err := s.ReadTextFile(context.Background(), sdk.ReadTextFileRequest{Path: filepath.Join(env.Root(), "new.txt")})
	var re *sdk.RequestError
	if !errors.As(err, &re) || re.Code != resourceNotFound {
		t.Errorf("reading a missing file: %v", err)
	}
	s.preset.MissingFileIsEmpty = true
	res, err := s.ReadTextFile(context.Background(), sdk.ReadTextFileRequest{Path: filepath.Join(env.Root(), "new.txt")})
	if err != nil || res.Content != "" {
		t.Errorf("with MissingFileIsEmpty: %q, %v", res.Content, err)
	}
}

// HRN-007: a message an agent sends for machines stays out of the conversation.
func TestHiddenAgentText(t *testing.T) {
	rec := &recorder{}
	s := &session{p: harness.Params{SessionID: "x", Emit: rec.emit}, preset: config.HarnessPreset{HideText: []string{"[MODE_UPDATE]"}}}
	for _, text := range []string{"[MODE_UPDATE] plan", "real words"} {
		if err := s.SessionUpdate(context.Background(), sdk.SessionNotification{SessionId: "x", Update: sdk.UpdateAgentMessageText(text)}); err != nil {
			t.Fatal(err)
		}
	}
	got := rec.text(event.AgentMessageChunk)
	if strings.Contains(got, "MODE_UPDATE") || !strings.Contains(got, "real words") {
		t.Errorf("conversation = %s", got)
	}
}
