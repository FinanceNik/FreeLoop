package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func openTest(t *testing.T, dir string, onAppend func(event.Event)) *Store {
	t.Helper()
	s, err := Open(Options{Dir: dir, OnAppend: onAppend, SyncInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func text(s string) json.RawMessage {
	b, _ := json.Marshal(event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: s}})
	return b
}

func TestCreateListAppendRead(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()

	a, err := s.CreateSession(ctx, session.Session{Title: "first", HarnessID: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	b, err := s.CreateSession(ctx, session.Session{Title: "second", HarnessID: "builtin"})
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("list = %+v, want second then first", list)
	}

	for i, msg := range []string{"one", "two", "three"} {
		ev, err := s.Append(ctx, a.ID, event.Notice, text(msg))
		if err != nil {
			t.Fatal(err)
		}
		if ev.Seq != int64(i+1) || ev.V != event.Version || ev.SID != a.ID {
			t.Fatalf("event %d = %+v", i, ev)
		}
	}

	evs, err := s.Events(ctx, a.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0].Seq != 2 || evs[1].Seq != 3 {
		t.Fatalf("events after 1 = %+v", evs)
	}
	var mc event.MessageChunk
	if err := json.Unmarshal(evs[1].Data, &mc); err != nil || mc.Content.Text != "three" {
		t.Fatalf("data = %s", evs[1].Data)
	}

	got, err := s.GetSession(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSeq != 3 {
		t.Errorf("LastSeq = %d, want 3", got.LastSeq)
	}
	list, _ = s.ListSessions(ctx)
	if list[0].ID != a.ID {
		t.Errorf("appending did not move the session to the top of the list")
	}
}

func TestAppendUpdatesStatus(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	a, _ := s.CreateSession(ctx, session.Session{Title: "x", HarnessID: "builtin"})
	if _, err := s.Append(ctx, a.ID, event.SessionStatus, json.RawMessage(`{"status":"running"}`)); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSession(ctx, a.ID)
	if got.Status != session.StatusRunning {
		t.Errorf("status = %q, want running", got.Status)
	}
}

func TestAppendTracksModeAndRedacts(t *testing.T) {
	ctx := context.Background()
	s, err := Open(Options{Dir: t.TempDir(), Redact: func(b json.RawMessage) json.RawMessage {
		return json.RawMessage(strings.ReplaceAll(string(b), "sk-secret", "[REDACTED]"))
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, _ := s.CreateSession(ctx, session.Session{Title: "x", ProjectRoot: "/r", Mode: "ask"})
	s.Append(ctx, a.ID, event.CurrentModeUpdate, json.RawMessage(`{"currentModeId":"plan"}`))
	s.Append(ctx, a.ID, event.UserMessageChunk, text("my key is sk-secret"))
	got, _ := s.GetSession(ctx, a.ID)
	if got.Mode != "plan" || got.ProjectRoot != "/r" {
		t.Errorf("session = %+v", got)
	}
	evs, _ := s.Events(ctx, a.ID, 0)
	if strings.Contains(string(evs[1].Data), "sk-secret") || !strings.Contains(string(evs[1].Data), "[REDACTED]") {
		t.Errorf("not redacted: %s", evs[1].Data)
	}
}

func TestAppendRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	a, _ := s.CreateSession(ctx, session.Session{Title: "x", HarnessID: "builtin"})
	if _, err := s.Append(ctx, a.ID, "bogus", nil); err == nil {
		t.Error("unknown type accepted")
	}
	if _, err := s.Append(ctx, a.ID, event.Notice, json.RawMessage(`[1]`)); err == nil {
		t.Error("non-object data accepted")
	}
	if _, err := s.Append(ctx, "ses_missing", event.Notice, nil); err != ErrNotFound {
		t.Errorf("unknown session: err = %v, want ErrNotFound", err)
	}
}

func TestDataIsStoredOnOneLine(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir, nil)
	a, _ := s.CreateSession(ctx, session.Session{Title: "x", HarnessID: "builtin"})
	if _, err := s.Append(ctx, a.ID, event.Notice, json.RawMessage("{\n  \"a\": 1\n}")); err != nil {
		t.Fatal(err)
	}
	s.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "sessions", a.ID, "events.jsonl"))
	if strings.Count(string(b), "\n") != 1 {
		t.Errorf("transcript = %q, want one line", b)
	}
}

func TestReopenRecoversSeqAndTruncatesTornTail(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir, nil)
	a, _ := s.CreateSession(ctx, session.Session{Title: "x", HarnessID: "builtin"})
	for range 2 {
		if _, err := s.Append(ctx, a.ID, event.Notice, nil); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	// Simulate a crash mid-write: a partial third line.
	path := filepath.Join(dir, "sessions", a.ID, "events.jsonl")
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"v":1,"seq":3,"ts":"2026-`)
	f.Close()

	s = openTest(t, dir, nil)
	defer s.Close()
	ev, err := s.Append(ctx, a.ID, event.Notice, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Seq != 3 {
		t.Errorf("seq after recovery = %d, want 3", ev.Seq)
	}
	evs, err := s.Events(ctx, a.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 {
		t.Errorf("got %d events after recovery, want 3", len(evs))
	}
}

func TestOnAppendOrderUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var seen []int64
	s := openTest(t, t.TempDir(), func(ev event.Event) {
		mu.Lock()
		seen = append(seen, ev.Seq)
		mu.Unlock()
	})
	defer s.Close()
	a, _ := s.CreateSession(ctx, session.Session{Title: "x", HarnessID: "builtin"})

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if _, err := s.Append(ctx, a.ID, event.Notice, nil); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for i, seq := range seen {
		if seq != int64(i+1) {
			t.Fatalf("OnAppend saw seq %d at position %d: %v", seq, i, seen)
		}
	}
}

func TestFilePermissions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir, nil)
	a, _ := s.CreateSession(ctx, session.Session{Title: "x", HarnessID: "builtin"})
	s.Append(ctx, a.ID, event.Notice, nil)
	s.Close()
	fi, err := os.Stat(filepath.Join(dir, "sessions", a.ID, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("transcript mode = %v, want 0600", fi.Mode().Perm())
	}
}
