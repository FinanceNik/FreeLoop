package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func modeEvent(mode string) json.RawMessage {
	b, _ := json.Marshal(event.CurrentModeData{CurrentModeID: mode})
	return b
}

// failModeWrites makes the index refuse a change of mode, as a full disk or a
// locked database would, and returns what ends it.
func failModeWrites(t *testing.T, s *Store) func() {
	t.Helper()
	if _, err := s.db.Exec(`CREATE TRIGGER fail_mode BEFORE UPDATE OF mode ON sessions BEGIN SELECT RAISE(ABORT, 'refused'); END`); err != nil {
		t.Fatal(err)
	}
	return func() {
		if _, err := s.db.Exec(`DROP TRIGGER fail_mode`); err != nil {
			t.Fatal(err)
		}
	}
}

// The transcript is the source of truth: a session must not go back to a looser
// mode because the index missed the change (SEC-001).
func TestALostModeChangeIsRepairedByTheNextAppend(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	ses, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin", Mode: "bypass"})
	restore := failModeWrites(t, s)
	if _, err := s.Append(ctx, ses.ID, event.CurrentModeUpdate, modeEvent("plan")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSession(ctx, ses.ID); got.Mode != "bypass" {
		t.Fatalf("the failure was not simulated: %s", got.Mode)
	}
	restore()
	if _, err := s.Append(ctx, ses.ID, event.TurnStarted, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSession(ctx, ses.ID); got.Mode != "plan" {
		t.Errorf("after the next append the mode is %s", got.Mode)
	}
}

func TestALostModeChangeIsRepairedAfterARestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir, nil)
	ses, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin", Mode: "bypass"})
	restore := failModeWrites(t, s)
	if _, err := s.Append(ctx, ses.ID, event.CurrentModeUpdate, modeEvent("plan")); err != nil {
		t.Fatal(err)
	}
	restore()
	s.Close()

	s = openTest(t, dir, nil)
	defer s.Close()
	if err := s.Reconcile(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSession(ctx, ses.ID); got.Mode != "plan" {
		t.Errorf("after a restart the mode is %s", got.Mode)
	}
}

// Replaying must not bring back a status: a session that was running when the
// app stopped is not running now.
func TestRepairLeavesStatusAlone(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir, nil)
	ses, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin", Mode: "ask"})
	status, _ := json.Marshal(event.SessionStatusData{Status: "running"})
	if _, err := s.Append(ctx, ses.ID, event.SessionStatus, status); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE sessions SET status = 'idle', last_seq = 0, indexed_seq = 0`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = openTest(t, dir, nil)
	defer s.Close()
	if err := s.Reconcile(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	s.indexed.Wait()
	if got, _ := s.GetSession(ctx, ses.ID); got.Status != "idle" {
		t.Errorf("the status came back as %s", got.Status)
	}
}
