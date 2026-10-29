package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

type log struct {
	mu     sync.Mutex
	events []event.Event
}

func (l *log) emit(typ event.Type, data any) {
	b, _ := json.Marshal(data)
	l.mu.Lock()
	l.events = append(l.events, event.Event{Seq: int64(len(l.events) + 1), Type: typ, Data: b})
	l.mu.Unlock()
}

func (l *log) all() []event.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]event.Event{}, l.events...)
}

func (l *log) count(typ event.Type) int {
	n := 0
	for _, e := range l.all() {
		if e.Type == typ {
			n++
		}
	}
	return n
}

type fixture struct {
	root   string
	log    *log
	broker *permission.Broker
	prov   *scripted.Provider
	sess   harness.Session
}

func setup(t *testing.T, mode permission.Mode, steps ...scripted.Step) *fixture {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{ID: "local", Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{root: env.Root(), log: &log{}, prov: scripted.New("p", steps...)}
	f.broker = permission.NewBroker(policy, func(_ string, typ event.Type, data any) { f.log.emit(typ, data) })
	f.sess, err = Harness{}.NewSession(context.Background(), harness.Params{
		SessionID: "s", Env: env, Broker: f.broker, Provider: f.prov, Model: "scripted", Mode: mode, Emit: f.log.emit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// answer approves every request the broker queues with option until stop
// is closed.
func (f *fixture) answer(option string, stop <-chan struct{}) {
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, r := range f.broker.Pending() {
				f.broker.Decide(r.ID, option, "", "")
			}
			time.Sleep(time.Millisecond)
		}
	}()
}

func TestToolLoopEndToEnd(t *testing.T) {
	f := setup(t, permission.ModeAcceptEdits, scripted.Demo("p").Steps()...)
	os.WriteFile(filepath.Join(f.root, "AGENTS.md"), []byte("Always answer in haiku."), 0o644)
	stop := make(chan struct{})
	defer close(stop)
	f.answer("allow_once", stop) // the shell call asks in accept-edits mode

	reason, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "show me the demo"})
	if err != nil || reason != harness.StopEndTurn {
		t.Fatalf("Prompt = %s, %v", reason, err)
	}
	b, err := os.ReadFile(filepath.Join(f.root, "freeloop-demo.txt"))
	if err != nil || !strings.Contains(string(b), "scripted demo") {
		t.Fatalf("demo file = %q, %v", b, err)
	}
	if n := f.log.count(event.ToolCall); n != 3 {
		t.Errorf("tool_call events = %d, want 3", n)
	}
	if n := f.log.count(event.PermissionRequested); n != 1 {
		t.Errorf("permission_requested = %d, want 1 (only the shell call)", n)
	}
	if n := f.log.count(event.PermissionDecided); n != 3 {
		t.Errorf("permission_decided = %d, want 3 (every call is audited)", n)
	}
	reqs := f.prov.Requests()
	if len(reqs) != 4 {
		t.Fatalf("model calls = %d, want 4", len(reqs))
	}
	if !strings.Contains(reqs[0].System, "Always answer in haiku.") {
		t.Error("AGENTS.md not in the system prompt")
	}
	last := reqs[3].Messages
	if len(last) != 7 { // user, (assistant, tool results) x3
		t.Errorf("history before the last call has %d messages, want 7", len(last))
	}
	res := last[6].Parts[0]
	if res.Type != provider.PartToolResult || !strings.Contains(res.Text, "Exit code 0") {
		t.Errorf("shell result = %+v", res)
	}

	// The emitted history rebuilds the context exactly.
	restored := Restore(f.log.all())
	got, _ := json.Marshal(restored)
	want, _ := json.Marshal(f.sess.(*session).history)
	if string(got) != string(want) {
		t.Errorf("restored history differs:\n got %s\nwant %s", got, want)
	}
}

func TestPlanModeRefusesWritesAndApprovalSwitchesMode(t *testing.T) {
	f := setup(t, permission.ModePlan,
		scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "x.txt", "content": "x"}}}},
		scripted.Step{Calls: []scripted.Call{{Name: "present_plan", Input: map[string]any{"plan": "1. write x.txt"}}}},
		scripted.Step{Text: "ok"},
	)
	stop := make(chan struct{})
	defer close(stop)
	f.answer("approve_accept_edits", stop)

	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "plan it"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "x.txt")); err == nil {
		t.Error("write ran in plan mode")
	}
	reqs := f.prov.Requests()
	for _, tool := range reqs[0].Tools {
		if tool.Name == "write_file" || tool.Name == "shell" {
			t.Errorf("plan mode offered %s", tool.Name)
		}
	}
	if !strings.Contains(reqs[1].Messages[2].Parts[0].Text, "plan mode is read-only") {
		t.Errorf("refusal not reported to the model: %+v", reqs[1].Messages[2])
	}
	if f.sess.Mode() != permission.ModeAcceptEdits {
		t.Errorf("mode after approval = %s", f.sess.Mode())
	}
	if f.log.count(event.CurrentModeUpdate) != 1 {
		t.Error("mode change not recorded")
	}
}

func TestCancelWhileWaitingForApproval(t *testing.T) {
	f := setup(t, permission.ModeAsk,
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "echo hi"}}}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for len(f.broker.Pending()) == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	reason, _ := f.sess.Prompt(ctx, harness.Prompt{Text: "run it"})
	if reason != harness.StopCancelled {
		t.Errorf("reason = %s", reason)
	}
	h := f.sess.(*session).history
	if len(h) != 3 || h[2].Parts[0].Type != provider.PartToolResult {
		t.Fatalf("history after cancel = %+v; every tool_use needs a result", h)
	}

	// The next prompt continues the same context.
	f.prov = scripted.New("p", scripted.Step{Text: "fine"})
	f.sess.(*session).SetModel(f.prov, "scripted", 0)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "never mind"}); err != nil {
		t.Fatal(err)
	}
	msgs := f.prov.Requests()[0].Messages
	if n := len(msgs); n != 3 || len(msgs[2].Parts) != 2 || msgs[2].Parts[1].Text != "never mind" {
		t.Errorf("follow-up prompt not merged into the trailing user message: %+v", msgs)
	}
}

func TestInvalidInputAndUnknownTool(t *testing.T) {
	f := setup(t, permission.ModeAsk,
		scripted.Step{Calls: []scripted.Call{
			{Name: "read_file", Input: map[string]any{}},
			{Name: "teleport", Input: map[string]any{}},
		}},
		scripted.Step{Text: "done"},
	)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	results := f.prov.Requests()[1].Messages[2].Parts
	if !results[0].IsError || !strings.Contains(results[0].Text, `"path"`) {
		t.Errorf("missing path result = %+v", results[0])
	}
	if !results[1].IsError || !strings.Contains(results[1].Text, "unknown tool") {
		t.Errorf("unknown tool result = %+v", results[1])
	}
}

func TestEditFile(t *testing.T) {
	f := setup(t, permission.ModeAcceptEdits,
		scripted.Step{Calls: []scripted.Call{
			{Name: "edit_file", Input: map[string]any{"path": "a.txt", "old_string": "x", "new_string": "y"}},
			{Name: "edit_file", Input: map[string]any{"path": "a.txt", "old_string": "x", "new_string": "y", "replace_all": true}},
			{Name: "edit_file", Input: map[string]any{"path": "a.txt", "old_string": "nope", "new_string": "y"}},
		}},
		scripted.Step{Text: "done"},
	)
	os.WriteFile(filepath.Join(f.root, "a.txt"), []byte("x x"), 0o644)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "edit"}); err != nil {
		t.Fatal(err)
	}
	results := f.prov.Requests()[1].Messages[2].Parts
	if !results[0].IsError || !strings.Contains(results[0].Text, "occurs 2 times") {
		t.Errorf("ambiguous edit = %+v", results[0])
	}
	if results[1].IsError {
		t.Errorf("replace_all failed: %+v", results[1])
	}
	if !results[2].IsError {
		t.Errorf("missing old_string accepted")
	}
	if b, _ := os.ReadFile(filepath.Join(f.root, "a.txt")); string(b) != "y y" {
		t.Errorf("file = %q", b)
	}
}

func TestProtectedPathRefusedInBypass(t *testing.T) {
	f := setup(t, permission.ModeBypass,
		scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": ".git/config", "content": "x"}}}},
		scripted.Step{Text: "done"},
	)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	r := f.prov.Requests()[1].Messages[2].Parts[0]
	if !r.IsError || !strings.Contains(r.Text, "protected_path") {
		t.Errorf("result = %+v", r)
	}
}

func TestDisabledToolsAreNotOffered(t *testing.T) {
	f := setup(t, permission.ModeBypass,
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "echo hi"}}}},
		scripted.Step{Text: "ok"},
	)
	f.sess.(*session).p.DisabledTools = []string{"shell"}
	s := f.sess.(*session)
	s.tools = slices.DeleteFunc(s.tools, func(t *tool) bool { return t.spec.Name == "shell" })
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	for _, tool := range f.prov.Requests()[0].Tools {
		if tool.Name == "shell" {
			t.Error("disabled tool offered")
		}
	}
	if r := f.prov.Requests()[1].Messages[2].Parts[0]; !r.IsError || !strings.Contains(r.Text, "turned off") {
		t.Errorf("result = %+v", r)
	}
}

// Review finding: a crash during a tool call left a tool_use without a
// result, which the API rejects on every later prompt.
func TestRestoredHistoryWithUnansweredCallIsRepaired(t *testing.T) {
	f := setup(t, permission.ModeAsk, scripted.Step{Text: "ok"})
	s := f.sess.(*session)
	history := []provider.Message{
		{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: "run it"}}},
		{Role: provider.RoleAssistant, Parts: []provider.Part{{Type: provider.PartToolUse, ToolCallID: "c1", ToolName: "shell", Input: json.RawMessage(`{"command":"make"}`)}}},
	}
	p := s.p
	p.History = history
	sess, err := Harness{}.NewSession(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Prompt(context.Background(), harness.Prompt{Text: "continue"}); err != nil {
		t.Fatal(err)
	}
	msgs := f.prov.Requests()[0].Messages
	if len(msgs) != 3 || msgs[2].Parts[0].Type != provider.PartToolResult || msgs[2].Parts[0].ToolCallID != "c1" {
		t.Fatalf("history sent = %+v", msgs)
	}
	if msgs[2].Parts[len(msgs[2].Parts)-1].Text != "continue" {
		t.Errorf("the new prompt was not appended after the repair: %+v", msgs[2])
	}
}

func TestInstructionFileLinkedOutsideIsNotRead(t *testing.T) {
	f := setup(t, permission.ModeAsk, scripted.Step{Text: "ok"})
	outside := filepath.Join(t.TempDir(), "notes.md")
	os.WriteFile(outside, []byte("PRIVATE NOTES"), 0o600)
	os.Symlink(outside, filepath.Join(f.root, "AGENTS.md"))
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.prov.Requests()[0].System, "PRIVATE NOTES") {
		t.Error("an AGENTS.md linked outside the project went into the system prompt")
	}
}

// HRN-010: a nested AGENTS.md joins the prompt once a file under its folder
// is touched, and not before.
func TestNestedAgentsFiles(t *testing.T) {
	f := setup(t, permission.ModeAsk,
		scripted.Step{Calls: []scripted.Call{{Name: "read_file", Input: map[string]any{"path": "web/src/app.ts"}}}},
		scripted.Step{Text: "done"},
	)
	os.WriteFile(filepath.Join(f.root, "AGENTS.md"), []byte("Root rule."), 0o644)
	os.MkdirAll(filepath.Join(f.root, "web", "src"), 0o755)
	os.MkdirAll(filepath.Join(f.root, "api"), 0o755)
	os.WriteFile(filepath.Join(f.root, "web", "AGENTS.md"), []byte("Use pnpm in web."), 0o644)
	os.WriteFile(filepath.Join(f.root, "api", "AGENTS.md"), []byte("API rule."), 0o644)
	os.WriteFile(filepath.Join(f.root, "web", "src", "app.ts"), []byte("x"), 0o644)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	reqs := f.prov.Requests()
	if strings.Contains(reqs[0].System, "Use pnpm in web.") {
		t.Error("nested file loaded before anything under it was touched")
	}
	if !strings.Contains(reqs[1].System, "Use pnpm in web.") || !strings.Contains(reqs[1].System, "Instructions for web/") {
		t.Errorf("nested file missing after reading web/src/app.ts:\n%s", reqs[1].System)
	}
	if strings.Contains(reqs[1].System, "API rule.") {
		t.Error("an unrelated nested file was loaded")
	}
	if !strings.Contains(reqs[1].System, "Root rule.") {
		t.Error("root AGENTS.md missing")
	}
}

func TestPromptEmbedsResources(t *testing.T) {
	f := setup(t, permission.ModeAsk, scripted.Step{Text: "ok"})
	p := harness.Prompt{Text: "look at @a.go", Resources: []harness.Resource{{Path: "a.go", URI: "file:///r/a.go", Text: "package a"}}}
	if _, err := f.sess.Prompt(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	var links []event.ContentBlock
	for _, ev := range f.log.all() {
		if ev.Type == event.UserMessageChunk {
			var c event.MessageChunk
			json.Unmarshal(ev.Data, &c)
			if c.Content.Type == "resource_link" {
				links = append(links, c.Content)
			}
		}
	}
	if len(links) != 1 || links[0].Name != "a.go" || links[0].URI != "file:///r/a.go" {
		t.Errorf("resource links = %+v", links)
	}
	parts := f.prov.Requests()[0].Messages[0].Parts
	if len(parts) != 2 || parts[0].Text != "look at @a.go" || parts[1].Text != "<file path=\"a.go\">\npackage a\n</file>" {
		t.Errorf("user parts = %+v", parts)
	}
}

// A failed turn leaves the session in error, so the attention view lists it
// (GUI-002); the next prompt starts it running again.
func TestFailedTurnSetsErrorStatus(t *testing.T) {
	f := setup(t, permission.ModeAsk, scripted.Step{Calls: []scripted.Call{{Name: "list_dir", Input: make(chan int)}}})
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "go"}); err == nil {
		t.Fatal("prompt succeeded")
	}
	evs := f.log.all()
	last := evs[len(evs)-1]
	if last.Type != event.SessionStatus || !strings.Contains(string(last.Data), `"error"`) {
		t.Errorf("last event = %s %s, want session_status error", last.Type, last.Data)
	}
	if f.log.count(event.Error) != 1 {
		t.Errorf("error events = %d", f.log.count(event.Error))
	}
}

// HRN-003: a profile's tool list and turn cap hold even when the model
// calls a tool it was not offered.
func TestProfileToolsAndTurnCap(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, _ := environment.NewLocal(environment.LocalOptions{ID: "local", Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	l := &log{}
	prov := scripted.New("p",
		scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "x.txt", "content": "x"}}}},
		scripted.Step{Calls: []scripted.Call{{Name: "list_dir", Input: map[string]any{"path": "."}}}},
		scripted.Step{Text: "done"},
	)
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { l.emit(typ, data) })
	s, err := Harness{}.NewSession(context.Background(), harness.Params{
		SessionID: "s", Env: env, Broker: broker, Provider: prov, Model: "scripted", Mode: permission.ModeAcceptEdits,
		Tools: []string{"list_dir"}, MaxTurns: 2, Emit: l.emit,
	})
	if err != nil {
		t.Fatal(err)
	}
	reason, err := s.Prompt(context.Background(), harness.Prompt{Text: "go"})
	if err != nil || reason != harness.StopMaxTurns {
		t.Fatalf("Prompt = %s, %v; want max_turns", reason, err)
	}
	if _, err := os.Stat(filepath.Join(root, "x.txt")); err == nil {
		t.Error("a tool outside the profile ran")
	}
	if !strings.Contains(fmt.Sprint(prov.Requests()[1].Messages), "not part of this session's profile") {
		t.Error("the model was not told the tool is outside the profile")
	}
}

type fakeMCP struct{ calls int }

func (f *fakeMCP) Tools() []harness.MCPTool {
	return []harness.MCPTool{{Name: "mcp__srv__do", Description: "Do it.", InputSchema: map[string]any{"type": "object"}, Server: "srv", Tool: "do"}}
}

func (f *fakeMCP) Call(context.Context, string, json.RawMessage) (string, bool, error) {
	f.calls++
	return "did it", false, nil
}

// MCP-003: MCP tools are offered outside plan mode only, and every call
// asks the broker under its namespaced name.
func TestMCPToolsAskAndStayOutOfPlanMode(t *testing.T) {
	f := setup(t, permission.ModeAsk, scripted.Step{Calls: []scripted.Call{{Name: "mcp__srv__do", Input: map[string]any{}}}}, scripted.Step{Text: "ok"})
	src := &fakeMCP{}
	s := f.sess.(*session)
	s.p.MCP = src
	stop := make(chan struct{})
	defer close(stop)
	var asked []string
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, r := range f.broker.Pending() {
				asked = append(asked, r.Tool)
				f.broker.Decide(r.ID, "allow_once", "", "")
			}
			time.Sleep(time.Millisecond)
		}
	}()
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	if src.calls != 1 || len(asked) != 1 || asked[0] != "mcp__srv__do" {
		t.Errorf("calls = %d, asked = %v", src.calls, asked)
	}
	if !slices.ContainsFunc(s.toolSpecs(permission.ModeAsk), func(t provider.ToolSpec) bool { return t.Name == "mcp__srv__do" }) {
		t.Error("not offered in ask mode")
	}
	if slices.ContainsFunc(s.toolSpecs(permission.ModePlan), func(t provider.ToolSpec) bool { return t.Name == "mcp__srv__do" }) {
		t.Error("offered in plan mode")
	}
}
