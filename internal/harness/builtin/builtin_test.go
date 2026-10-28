package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
				f.broker.Decide(r.ID, option, "")
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

	reason, err := f.sess.Prompt(context.Background(), "show me the demo")
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

	if _, err := f.sess.Prompt(context.Background(), "plan it"); err != nil {
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
	reason, _ := f.sess.Prompt(ctx, "run it")
	if reason != harness.StopCancelled {
		t.Errorf("reason = %s", reason)
	}
	h := f.sess.(*session).history
	if len(h) != 3 || h[2].Parts[0].Type != provider.PartToolResult {
		t.Fatalf("history after cancel = %+v; every tool_use needs a result", h)
	}

	// The next prompt continues the same context.
	f.prov = scripted.New("p", scripted.Step{Text: "fine"})
	f.sess.(*session).p.Provider = f.prov
	if _, err := f.sess.Prompt(context.Background(), "never mind"); err != nil {
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
	if _, err := f.sess.Prompt(context.Background(), "go"); err != nil {
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
	if _, err := f.sess.Prompt(context.Background(), "edit"); err != nil {
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
	if _, err := f.sess.Prompt(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	r := f.prov.Requests()[1].Messages[2].Parts[0]
	if !r.IsError || !strings.Contains(r.Text, "protected_path") {
		t.Errorf("result = %+v", r)
	}
}
