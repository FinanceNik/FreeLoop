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
	if ses.Title != "demo" || ses.LastSeq != 1 || ses.Model != "scripted" || ses.Mode != "ask" {
		t.Errorf("session = %+v", ses)
	}
	if ev := next(t, ch); ev.Type != event.SessionStatus || ev.Seq != 1 {
		t.Errorf("first event = %+v, want session_status seq 1", ev)
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
	if len(lines) != 2 || !strings.Contains(lines[1], `"text":"hello"`) {
		t.Errorf("transcript = %q", b)
	}
	e.Close()

	// Replay after a restart returns the same events.
	e = openTest(t, dir)
	defer e.Close()
	evs, err := e.Events(ctx, ses.ID, 0)
	if err != nil || len(evs) != 2 {
		t.Fatalf("Events after reopen = %v, %v", evs, err)
	}
	list, _ := e.ListSessions(ctx)
	if len(list) != 1 || list[0].ID != ses.ID || list[0].LastSeq != 2 {
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
	evs, _ := e.Events(ctx, ses.ID, 0)
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
	if v, _ := mem.Get(secrets.Ref{Service: "freeloop", Account: "providers/anthropic"}); v != "sk-ant-test-0123456789" {
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
