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
)

func openTest(t *testing.T, dir string) *Engine {
	t.Helper()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory()})
	if err != nil {
		t.Fatal(err)
	}
	return e
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

	ses, err := e.CreateSession(ctx, CreateSessionRequest{Title: "  demo  "})
	if err != nil {
		t.Fatal(err)
	}
	if ses.Title != "demo" || ses.LastSeq != 1 {
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
	ses, err := e.CreateSession(context.Background(), CreateSessionRequest{})
	if err != nil || ses.Title != "New session" {
		t.Fatalf("CreateSession = %+v, %v", ses, err)
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
	ses, _ := e.CreateSession(ctx, CreateSessionRequest{})
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
