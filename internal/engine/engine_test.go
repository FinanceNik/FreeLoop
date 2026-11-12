package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func openTest(t *testing.T, dir string) *Engine {
	t.Helper()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// demoSession creates a session on the scripted demo provider in a fresh
// project folder.
func demoSession(t *testing.T, e *Engine, title, mode string) (session.Session, string) {
	t.Helper()
	root := t.TempDir()
	ses, err := e.CreateSession(context.Background(), CreateSessionRequest{Title: title, ProjectRoot: root, ProviderID: DemoProviderID, Mode: mode})
	if err != nil {
		t.Fatal(err)
	}
	return ses, root
}

func next(t *testing.T, ch <-chan event.Event) event.Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event published")
		return event.Event{}
	}
}

// M0 definition of done: an event appended through the engine lands in the
// session's JSONL file and reaches subscribers.
func TestAppendedEventIsPersistedThenPublished(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	ch, cancel := e.Subscribe()
	defer cancel()

	ses, _ := demoSession(t, e, "  demo  ", "")
	// A new built-in session records its status and advertises /compact.
	if ses.Title != "demo" || ses.LastSeq != 2 || ses.Model != "scripted" || ses.Mode != "ask" {
		t.Errorf("session = %+v", ses)
	}
	if ev := next(t, ch); ev.Type != event.SessionStatus || ev.Seq != 1 {
		t.Errorf("first event = %+v, want session_status seq 1", ev)
	}
	if ev := next(t, ch); ev.Type != event.AvailableCommandsUpdate || ev.Seq != 2 {
		t.Errorf("second event = %+v, want available_commands_update seq 2", ev)
	}

	ev, err := e.AppendEvent(ctx, AppendEventRequest{
		SessionID: ses.ID,
		Type:      event.UserMessageChunk,
		Data:      json.RawMessage(`{"content":{"type":"text","text":"hello"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := next(t, ch); got.Seq != ev.Seq || got.Type != event.UserMessageChunk {
		t.Errorf("published %+v, want %+v", got, ev)
	}

	b, err := os.ReadFile(filepath.Join(dir, "sessions", ses.ID, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 || !strings.Contains(lines[2], `"text":"hello"`) {
		t.Errorf("transcript = %q", b)
	}
	e.Close()

	// Replay after a restart returns the same events.
	e = openTest(t, dir)
	defer e.Close()
	evs, err := e.Events(ctx, ses.ID, 0, 0)
	if err != nil || len(evs) != 3 {
		t.Fatalf("Events after reopen = %v, %v", evs, err)
	}
	list, _ := e.ListSessions(ctx)
	if len(list) != 1 || list[0].ID != ses.ID || list[0].LastSeq != 3 {
		t.Errorf("ListSessions after reopen = %+v", list)
	}
}

func TestDefaultTitle(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "", "bypass")
	if ses.Title != "New session" {
		t.Errorf("title = %q", ses.Title)
	}
	if ses.Mode != "ask" {
		t.Errorf("a new session started in %s mode; bypass must never be the initial mode", ses.Mode)
	}
	if _, err := e.CreateSession(context.Background(), CreateSessionRequest{ProviderID: DemoProviderID}); err == nil {
		t.Error("session without a project folder accepted")
	}
}

func TestAppendErrors(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	if _, err := e.AppendEvent(ctx, AppendEventRequest{Type: event.Notice}); err == nil {
		t.Error("missing session ID accepted")
	}
	if _, err := e.AppendEvent(ctx, AppendEventRequest{SessionID: "ses_nope", Type: event.Notice}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown session: err = %v", err)
	}
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	_, cancel := e.bus.subscribe(1) // never read
	defer cancel()
	ses, _ := demoSession(t, e, "", "")
	done := make(chan struct{})
	go func() {
		for range 100 {
			e.AppendEvent(ctx, AppendEventRequest{SessionID: ses.ID, Type: event.Notice})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("append blocked on a slow subscriber")
	}
}

func TestConfigIssuesDoNotFailOpen(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("ui: [broken\n"), 0o600)
	e := openTest(t, dir)
	defer e.Close()
	if len(e.ConfigIssues()) != 1 {
		t.Errorf("ConfigIssues = %v", e.ConfigIssues())
	}
}

func TestCloseEndsSubscriptions(t *testing.T) {
	e := openTest(t, t.TempDir())
	ch, cancel := e.Subscribe()
	e.Close()
	if _, ok := <-ch; ok {
		t.Error("channel still open after Close")
	}
	cancel() // must not panic
}

// waitFor reads events until one matches or the deadline passes.
func waitFor(t *testing.T, ch <-chan event.Event, match func(event.Event) bool) event.Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-ch:
			if match(ev) {
				return ev
			}
		case <-deadline:
			t.Fatal("timed out waiting for an event")
		}
	}
}

func TestPromptRunsTheAgentLoop(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	ses, root := demoSession(t, e, "demo", "ask")

	if _, err := e.Prompt(ctx, ses.ID, "go"); err != nil {
		t.Fatal(err)
	}
	// Ask mode: the write asks first.
	req := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	var pr struct{ RequestID, Tool string }
	json.Unmarshal(req.Data, &pr)
	if pr.Tool != "write_file" || len(e.PendingPermissions()) != 1 {
		t.Fatalf("first request = %+v, pending = %v", pr, e.PendingPermissions())
	}
	if err := e.Decide(DecideRequest{RequestID: pr.RequestID, OptionID: "allow_once"}); err != nil {
		t.Fatal(err)
	}
	req = waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	json.Unmarshal(req.Data, &pr)
	e.Decide(DecideRequest{RequestID: pr.RequestID, OptionID: "allow_once"})
	end := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.TurnEnded })
	if !strings.Contains(string(end.Data), "end_turn") {
		t.Errorf("turn ended with %s", end.Data)
	}
	if _, err := os.Stat(filepath.Join(root, "freeloop-demo.txt")); err != nil {
		t.Errorf("demo file missing: %v", err)
	}
	waitFor(t, ch, func(ev event.Event) bool {
		return ev.Type == event.SessionStatus && strings.Contains(string(ev.Data), "idle")
	})
	got, _ := e.store.GetSession(ctx, ses.ID)
	if got.Status != session.StatusIdle {
		t.Errorf("status = %s", got.Status)
	}
}

func TestCancelAndRecovery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	ch, unsub := e.Subscribe()
	ses, _ := demoSession(t, e, "demo", "ask")
	e.Prompt(ctx, ses.ID, "go")
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	unsub()

	// Simulate a crash: the store closes with the turn still waiting.
	e.mu.Lock()
	e.runtimes = map[string]*runtime{}
	e.mu.Unlock()
	e.bus.close()
	e.store.Close()

	e = openTest(t, dir)
	defer e.Close()
	got, _ := e.store.GetSession(ctx, ses.ID)
	if got.Status != session.StatusIdle {
		t.Errorf("status after recovery = %s", got.Status)
	}
	evs, _ := e.Events(ctx, ses.ID, 0, 0)
	var types []string
	for _, ev := range evs[len(evs)-3:] {
		types = append(types, string(ev.Type))
	}
	if strings.Join(types, ",") != "permission_decided,turn_ended,session_status" {
		t.Errorf("recovery events = %v", types)
	}
}

func TestPromptQueueAndCancel(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	ses, _ := demoSession(t, e, "demo", "ask")
	e.Prompt(ctx, ses.ID, "first")
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	res, err := e.Prompt(ctx, ses.ID, "second")
	if err != nil || !res.Queued {
		t.Fatalf("second prompt = %+v, %v", res, err)
	}
	start := time.Now()
	e.Cancel(ses.ID)
	end := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.TurnEnded })
	if !strings.Contains(string(end.Data), "cancelled") || time.Since(start) > time.Second {
		t.Errorf("cancel: %s after %v", end.Data, time.Since(start))
	}
	if !hasNotice(t, e, ses.ID, "You stopped the turn.") {
		t.Error("no notice says who stopped the turn")
	}
}

// Quitting stops a running turn and says so, since the agent reports it as
// any other cancel.
func TestQuitSaysWhyTheTurnStopped(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	ch, unsub := e.Subscribe()
	ses, _ := demoSession(t, e, "demo", "ask")
	e.Prompt(ctx, ses.ID, "go")
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	unsub()
	e.Close()
	e = openTest(t, dir)
	defer e.Close()
	if !hasNotice(t, e, ses.ID, "Stopped because Freeloop quit.") {
		t.Error("no notice says the quit stopped the turn")
	}
}

func hasNotice(t *testing.T, e *Engine, sid, msg string) bool {
	t.Helper()
	evs, err := e.Events(context.Background(), sid, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range evs {
		if ev.Type == event.Notice && strings.Contains(string(ev.Data), msg) {
			return true
		}
	}
	return false
}

func TestProvidersKeepKeysInTheKeychain(t *testing.T) {
	dir := t.TempDir()
	mem := secrets.NewMemory()
	e, err := Open(Options{DataDir: dir, Secrets: mem, Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	info, err := e.SaveProvider(SaveProviderRequest{Name: "Anthropic", Type: "anthropic", APIKey: "sk-ant-test-0123456789", DefaultModel: "claude-opus-5-5"})
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "anthropic" || !info.HasKey {
		t.Errorf("info = %+v", info)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "providers.yaml"))
	if strings.Contains(string(b), "sk-ant-test") || !strings.Contains(string(b), "keychain://freeloop/providers/anthropic") {
		t.Errorf("providers.yaml = %s", b)
	}
	if v, _ := mem.Get(secrets.Ref{Service: "freeloop", Account: "providers/anthropic"}); !strings.Contains(v, "sk-ant-test-0123456789") {
		t.Errorf("keychain value = %q", v)
	}

	// A known key pasted into a session is redacted in the transcript.
	root := t.TempDir()
	ses, err := e.CreateSession(context.Background(), CreateSessionRequest{ProjectRoot: root, ProviderID: "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	e.AppendEvent(context.Background(), AppendEventRequest{SessionID: ses.ID, Type: event.UserMessageChunk,
		Data: json.RawMessage(`{"content":{"type":"text","text":"key sk-ant-test-0123456789"}}`)})
	tb, _ := os.ReadFile(filepath.Join(dir, "sessions", ses.ID, "events.jsonl"))
	if strings.Contains(string(tb), "sk-ant-test") {
		t.Errorf("transcript holds the key: %s", tb)
	}

	if err := e.DeleteProvider("anthropic"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get(secrets.Ref{Service: "freeloop", Account: "providers/anthropic"}); err == nil {
		t.Error("key left in the keychain after delete")
	}
}

func TestBypassNeedsOptIn(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "", "")
	if err := e.SetMode(context.Background(), ses.ID, "bypass"); err == nil {
		t.Error("bypass allowed without opt-in")
	}
	if err := e.SetMode(context.Background(), ses.ID, "plan"); err != nil {
		t.Fatal(err)
	}
	got, _ := e.store.GetSession(context.Background(), ses.ID)
	if got.Mode != "plan" {
		t.Errorf("mode = %s", got.Mode)
	}
}

// Review finding: a prompt that fetched its runtime just before
// stopRuntime removed it started a turn on the stopped runtime, which
// nothing could cancel. It now fails and asks the user to send it again.
func TestPromptRacingAStop(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "", "ask") // the default title makes enqueue emit
	r, err := e.runtimeFor(ctx, ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	b := e.streamFor(ses.ID)
	b.mu.Lock() // enqueue blocks in emit after runtimeFor, before r.mu
	errc := make(chan error, 1)
	go func() {
		_, err := e.Prompt(ctx, ses.ID, "hello")
		errc <- err
	}()
	time.Sleep(100 * time.Millisecond)
	e.stopRuntime(ses.ID)
	b.mu.Unlock()
	// Either the prompt reached the stopped runtime and was refused, or,
	// on a slow machine, it fetched a new one after the stop; never does a
	// turn run on the stopped runtime.
	err = <-errc
	r.mu.Lock()
	busy := r.busy
	r.mu.Unlock()
	if busy {
		t.Error("a turn started on the stopped runtime")
	}
	if err != nil && !errors.Is(err, errStopped) {
		t.Errorf("prompt during a stop = %v", err)
	}
	if err == nil {
		e.mu.Lock()
		now := e.runtimes[ses.ID]
		e.mu.Unlock()
		if now == nil || now == r {
			t.Error("the prompt ran without a live runtime")
		}
	}
}

// GUI-004: a message sent while a turn runs waits, and goes out as the next turn once
// that one ends, without the user sending it again.
func TestAQueuedMessageGoesOutWhenTheTurnEnds(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "a.txt", "content": "a\n"}}}},
		scripted.Step{Text: "first done"},
		scripted.Step{Text: "second done"},
	)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "first")
	req := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	if res, err := e.Prompt(ctx, ses.ID, "second"); err != nil || !res.Queued {
		t.Fatalf("second prompt = %+v, %v", res, err)
	}
	_ = req
	// Every write is allowed; the script starts over each turn, so the second asks too.
	stop := make(chan struct{})
	defer close(stop)
	approveAll(e, stop)
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.TurnEnded })
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.TurnEnded })
	evs, _ := e.Events(ctx, ses.ID, 0, 0)
	var order []string
	for _, ev := range evs {
		switch ev.Type {
		case event.TurnEnded:
			order = append(order, "end")
		case event.UserMessageChunk:
			var d event.MessageChunk
			json.Unmarshal(ev.Data, &d)
			order = append(order, d.Content.Text)
		}
	}
	if got := strings.Join(order, " | "); got != "first | end | second | end" {
		t.Errorf("the turns went %s", got)
	}
}
