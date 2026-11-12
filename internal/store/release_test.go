package store

import (
	"context"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// OQ-053: a released session holds no open file, and the next event opens it
// again and carries on from the last seq.
func TestReleaseClosesTheTranscriptAndTheNextAppendOpensIt(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	a, err := s.CreateSession(ctx, session.Session{Title: "idle", HarnessID: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(ctx, a.ID, event.Notice, text("one")); err != nil {
		t.Fatal(err)
	}
	open := func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, ok := s.files[a.ID]
		return ok
	}
	if !open() {
		t.Fatal("the transcript is not open after an append")
	}
	s.Release(a.ID)
	if open() {
		t.Fatal("the transcript is still open after Release")
	}
	s.Release(a.ID) // a session with nothing open is fine
	ev, err := s.Append(ctx, a.ID, event.Notice, text("two"))
	if err != nil || ev.Seq != 2 {
		t.Fatalf("append after release = %+v, %v", ev, err)
	}
	if evs, _ := s.Events(ctx, a.ID, 0); len(evs) != 2 {
		t.Errorf("events = %d", len(evs))
	}
}
