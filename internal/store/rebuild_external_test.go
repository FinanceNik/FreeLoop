package store

import (
	"context"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/session"
)

// SES-003: a rebuilt index still knows which agent session to load, or the agent
// silently starts anew under a transcript of the old conversation.
func TestARebuildKeepsTheAgentSession(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	ses, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "claude-code"})
	if err := s.SetExternalID(ctx, ses.ID, "agent-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM sessions`); err != nil {
		t.Fatal(err)
	}
	if err := s.rebuildSession(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSession(ctx, ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalID != "agent-1" {
		t.Errorf("after a rebuild the agent session is %q", got.ExternalID)
	}
}

// Sessions go on while one is being opened, however long its transcript.
func TestOpeningOneSessionDoesNotHoldTheStore(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin"})
	b, _ := s.CreateSession(ctx, session.Session{Title: "b", HarnessID: "builtin"})
	// Many callers on both at once: every one gets the same open transcript.
	got := make(chan *transcript, 40)
	for i := range 40 {
		id := a.ID
		if i%2 == 1 {
			id = b.ID
		}
		go func() {
			tr, err := s.transcript(ctx, id)
			if err != nil {
				t.Error(err)
			}
			got <- tr
		}()
	}
	seen := map[*transcript]bool{}
	for range 40 {
		seen[<-got] = true
	}
	if len(seen) != 2 {
		t.Errorf("%d transcripts for two sessions", len(seen))
	}
}
