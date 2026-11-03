package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func limitEvent(t *testing.T, used float64, resets int64, status string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(event.LimitData{Source: "Agent", Windows: []event.LimitWindow{{ID: "five_hour", Label: "5-hour limit", UsedPercent: used, ResetsAt: resets, Status: status}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// GUI-028: one row per run of a limit, with its peak, from any session's reports.
func TestLimitPeriods(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir, nil)
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "agent"})
	b, _ := s.CreateSession(ctx, session.Session{Title: "b", HarnessID: "agent"})
	const earlier, later = int64(2000000000), int64(2000018000)
	for _, r := range []struct {
		sid    string
		used   float64
		resets int64
		status string
	}{
		{a.ID, 40, earlier, "allowed"},
		{b.ID, 90, earlier, "warning"},
		{a.ID, 100, earlier, "rejected"},
		{a.ID, 5, later, "allowed"},
		{b.ID, 12, later, "allowed"},
	} {
		if _, err := s.Append(ctx, r.sid, event.LimitUpdate, limitEvent(t, r.used, r.resets, r.status)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	s.indexed.Wait()
	check := func(s *Store) {
		t.Helper()
		got, err := s.LimitPeriods(ctx, time.Unix(earlier-1, 0))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("%+v", got)
		}
		if got[0].ResetsAt.Unix() != later || got[0].Used != 12 || got[0].Peak != 12 || got[0].Refused != 0 {
			t.Errorf("the run going on: %+v", got[0])
		}
		if got[1].ResetsAt.Unix() != earlier || got[1].Peak != 100 || got[1].Warned != 1 || got[1].Refused != 1 || got[1].Label != "5-hour limit" {
			t.Errorf("the run before it: %+v", got[1])
		}
		if got, _ := s.LimitPeriods(ctx, time.Unix(earlier+1, 0)); len(got) != 1 {
			t.Errorf("since is ignored: %+v", got)
		}
	}
	check(s)
	// Catching up from the transcripts does not count anything twice.
	if _, err := s.db.Exec(`UPDATE sessions SET indexed_seq = 0`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = openTest(t, dir, nil)
	defer s.Close()
	s.IndexAll(ctx)
	check(s)
}
