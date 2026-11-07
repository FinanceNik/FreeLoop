package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
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
	if len(asked) != 2 || asked[0].Tool != "edit_file" || asked[0].Paths[0] != filepath.Join(env.Root(), "notes.txt") || asked[1].Tool != "shell" || asked[1].Command != "sh -c 'echo from-terminal'" {
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

// PRV-008, GUI-023: an agent that reports each model call has each recorded once,
// with the stream's final output, its model and its hour-long cache writes. A result
// adds what the messages did not show, and a call cut off by the end of the turn
// still counts. The turn's own usage would count it twice, so it is left
// out.
func TestUsagePerModelCall(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"}, Skills: config.SkillsClaudePlugin}}
	s, err := h.NewSession(context.Background(), harness.Params{SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeBypass, Emit: rec.emit})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Prompt(context.Background(), harness.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	var got []event.UsageData
	rec.mu.Lock()
	for _, e := range rec.events {
		if e.Type == event.UsageUpdate {
			var d event.UsageData
			json.Unmarshal(e.Data, &d)
			got = append(got, d)
		}
	}
	rec.mu.Unlock()
	want := []event.UsageData{
		{InputTokens: 3, OutputTokens: 4, CacheWriteTokens: 20, CacheWrite1hTokens: 20, Model: "test-model"},
		{InputTokens: 2, OutputTokens: 192, CacheReadTokens: 100, CacheWriteTokens: 50, CacheWrite1hTokens: 50, ContextTokens: 152, Model: "test-model"},
		{InputTokens: 1000, OutputTokens: 80, CacheWriteTokens: 30, Model: "test-model", Adjustment: true},
		{ContextTokens: 150, ContextWindow: 1000},
		{InputTokens: 5, OutputTokens: 1, CacheReadTokens: 200, ContextTokens: 205, ContextWindow: 1000, Model: "test-model"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("usage events = %+v, want %+v", got, want)
	}
}

// PRV-008: an agent that reports only a turn's total has it priced by the model it
// says it is on, and a turn it reports nothing for is said to have none, never $0.
func TestUsagePerTurn(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	for _, c := range []struct {
		name, prompt string
		model        bool
		want         event.UsageData
	}{
		{"no model setting", "go", false, event.UsageData{InputTokens: 10, OutputTokens: 5, Measured: event.MeasuredTurn}},
		{"a model setting", "go", true, event.UsageData{InputTokens: 10, OutputTokens: 5, Measured: event.MeasuredTurn, Model: "agent-model"}},
		{"nothing reported", "silent", true, event.UsageData{Measured: event.MeasuredNone}},
	} {
		rec := &recorder{}
		broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
		vars := map[string]string{acptest.EnvVar: "1"}
		if c.model {
			vars[acptest.ModelVar] = "1"
		}
		h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: vars}}
		s, err := h.NewSession(context.Background(), harness.Params{SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeBypass, Emit: rec.emit})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Prompt(context.Background(), harness.Prompt{Text: c.prompt}); err != nil {
			t.Fatal(err)
		}
		s.Close()
		var got []event.UsageData
		rec.mu.Lock()
		for _, e := range rec.events {
			if e.Type == event.UsageUpdate {
				var d event.UsageData
				json.Unmarshal(e.Data, &d)
				got = append(got, d)
			}
		}
		rec.mu.Unlock()
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: usage events = %+v, want %+v", c.name, got, c.want)
		}
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
	// A refused change tells the agent again in the background, which reads these.
	set := func(m permission.Mode, a approval) {
		s.mu.Lock()
		s.mode, s.approved = m, a
		s.mu.Unlock()
	}
	for _, m := range []permission.Mode{permission.ModeBypass, permission.ModeAcceptEdits, permission.ModeAsk} {
		s.agentMode(m)
		if s.Mode() != permission.ModePlan {
			t.Fatalf("the agent changed plan mode to %s on its own", m)
		}
	}
	if strings.Contains(rec.text(event.CurrentModeUpdate), "bypass") {
		t.Error("the refused change reached the transcript")
	}
	if !strings.Contains(rec.text(event.ModeRefused), `"to":"bypass"`) {
		t.Error("the user was not told")
	}

	set(permission.ModePlan, approval{kind: permission.KindSwitchMode, until: time.Now().Add(time.Minute)})
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
	set(permission.ModePlan, approval{kind: permission.KindEdit, until: time.Now().Add(time.Minute)})
	s.agentMode(permission.ModeAsk)
	if s.Mode() != permission.ModePlan {
		t.Error("an approval of something else loosened the mode")
	}

	set(permission.ModeAcceptEdits, approval{})
	s.agentMode(permission.ModePlan)
	if s.Mode() != permission.ModePlan {
		t.Error("the agent could not tighten the mode")
	}
}

// Auto or bypass the user chose approving the agent's plan holds when the app allows
// it, and otherwise the session asks: it used to stay in plan mode, and said the
// agent had tried it on its own (2026-11-06).
func TestTheUserChoosesTheModeApprovingAPlan(t *testing.T) {
	rec := &recorder{}
	var refusal error
	s := &session{p: harness.Params{SessionID: "x", Emit: rec.emit, AllowUnattended: func(permission.Mode) error { return refusal }}, preset: config.HarnessPreset{Name: "Agent"}}
	approve := func(byUser bool) {
		s.mu.Lock()
		s.mode = permission.ModePlan
		s.approved = approval{kind: permission.KindSwitchMode, until: time.Now().Add(time.Minute), byUser: byUser}
		s.mu.Unlock()
	}

	approve(true)
	s.agentMode(permission.ModeBypass)
	if s.Mode() != permission.ModeBypass {
		t.Errorf("the user's choice of bypass left %s", s.Mode())
	}

	approve(true)
	refusal = errors.New("bypass mode is off")
	s.agentMode(permission.ModeBypass)
	if s.Mode() != permission.ModeAsk {
		t.Errorf("a refused choice left %s, not ask", s.Mode())
	}
	if got := rec.text(event.ModeRefused); !strings.Contains(got, `"chosen":true`) || !strings.Contains(got, "bypass mode is off") {
		t.Errorf("the user was not told why: %s", got)
	}

	// A rule or the mode approving the plan is no choice of the user's.
	approve(false)
	refusal = nil
	s.agentMode(permission.ModeAuto)
	if s.Mode() != permission.ModePlan {
		t.Errorf("an approval not by the user reached %s", s.Mode())
	}
}

// HRN-007: several of the app's modes can share one of the agent's, and a mode
// the preset does not name changes nothing.
func TestAgentReportsItsMode(t *testing.T) {
	rec := &recorder{}
	preset := config.HarnessPreset{Name: "Agent", Modes: map[string]string{"ask": "asks", "accept_edits": "asks", "plan": "reads", "bypass": "asks"}}
	s := &session{p: harness.Params{SessionID: "x", Emit: rec.emit}, mode: permission.ModeAcceptEdits, preset: preset}
	set := func(m permission.Mode) {
		s.mu.Lock()
		s.mode = m
		s.mu.Unlock()
	}
	s.agentReported("asks")
	if s.Mode() != permission.ModeAcceptEdits {
		t.Errorf("reporting the mode it is in left %s", s.Mode())
	}
	set(permission.ModePlan)
	s.agentReported("asks")
	if s.Mode() != permission.ModePlan {
		t.Errorf("a loosening report took effect: %s", s.Mode())
	}
	set(permission.ModeAsk)
	s.agentReported("reads")
	if s.Mode() != permission.ModePlan {
		t.Errorf("tightening left %s", s.Mode())
	}
	// With no mapping for it there is no agent to call back, so this ends here.
	u := &session{p: harness.Params{SessionID: "x", Emit: rec.emit}, mode: permission.ModeAsk, preset: config.HarnessPreset{Name: "Agent", Modes: map[string]string{"ask": "asks"}}}
	u.agentReported("yolo")
	if u.Mode() != permission.ModeAsk || !strings.Contains(rec.text(event.Notice), "yolo") {
		t.Errorf("an unnamed mode: %s, %s", u.Mode(), rec.text(event.Notice))
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

// SEC-003: the line judged for an agent's terminal is what runs. A variable whose name
// is not one, which once made `git x=1 sh evil.sh` of `sh evil.sh` and so passed an
// allow rule for git, is refused; arguments and values are quoted as the shell reads
// them, so a rule sees each as one word.
func TestATerminalIsJudgedAsItRuns(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"}}}
	hs, err := h.NewSession(context.Background(), harness.Params{SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeAsk, Emit: rec.emit})
	if err != nil {
		t.Fatal(err)
	}
	defer hs.Close()
	s := hs.(*session)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.CreateTerminal(ctx, sdk.CreateTerminalRequest{SessionId: "s1", Command: "sh", Args: []string{"evil.sh"}, Env: []sdk.EnvVariable{{Name: "git x", Value: "1"}}}); err == nil || !strings.Contains(err.Error(), "not a variable name") {
		t.Errorf("a variable named %q: %v", "git x", err)
	}
	// Asked about, the line names each word as it runs; refused here, nothing runs.
	go func() {
		for range 200 {
			if p := broker.Pending(); len(p) == 1 {
				if got := p[0].Command; got != `GIT_X='a; rm -rf ~' sh -c 'echo hi'` {
					t.Errorf("judged %q", got)
				}
				broker.Decide(p[0].ID, "reject_once", "", "")
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("nothing was asked")
	}()
	if _, err := s.CreateTerminal(ctx, sdk.CreateTerminalRequest{SessionId: "s1", Command: "sh", Args: []string{"-c", "echo hi"}, Env: []sdk.EnvVariable{{Name: "GIT_X", Value: "a; rm -rf ~"}}}); err == nil {
		t.Error("a refused terminal ran")
	}
}
