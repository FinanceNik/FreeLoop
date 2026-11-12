package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// GUI-051: with commits on both sides of the window, the first commit in it
// takes on only what was spent since the commit before it, and the usage read is
// the window's and that one session's, by index.
func TestWorkSinceStartsTheFirstCommitAfterTheOneBefore(t *testing.T) {
	ctx := context.Background()
	clock := &workClock{now: time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)}
	s := workStore(t, clock)
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin", ProviderID: "p", Model: "m"})
	other, _ := s.CreateSession(ctx, session.Session{Title: "other", HarnessID: "builtin", ProviderID: "p", Model: "m"})
	turn := func(sid string, in int64, sha string) {
		put(t, s, clock, sid, event.UsageUpdate, event.UsageData{ProviderID: "p", Model: "m", InputTokens: in})
		if sha != "" {
			put(t, s, clock, sid, event.ToolCall, commitCmd("git commit -m x"))
			put(t, s, clock, sid, event.GitState, event.GitStateData{Repo: true, Head: sha, Commits: []event.CommitInfo{{SHA: sha, Added: 1, Files: 1}}})
		}
		put(t, s, clock, sid, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	}
	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "h0"})
	turn(a.ID, 1000, "old1")
	turn(a.ID, 2000, "old2")
	turn(other.ID, 5, "")
	clock.tick(24 * time.Hour)
	since := clock.now
	clock.tick(time.Hour)
	turn(a.ID, 30, "new1")
	turn(a.ID, 40, "new2")
	if left, err := s.FoldWork(ctx, time.Now().Add(time.Minute), nil); err != nil || left != 0 {
		t.Fatalf("fold: %d %v", left, err)
	}
	commits, usage, _, err := s.WorkSince(ctx, since)
	if err != nil {
		t.Fatal(err)
	}
	var in, before []string
	for _, c := range commits {
		if c.Before {
			before = append(before, c.SHA)
		} else {
			in = append(in, c.SHA)
		}
	}
	if len(before) != 1 || before[0] != "old2" || len(in) != 2 || in[0] != "new1" {
		t.Fatalf("commits %+v", commits)
	}
	var total int64
	for _, u := range usage {
		total += u.Input
		if u.Input == 1000 {
			t.Errorf("usage from before the commit before the window is read: %+v", u)
		}
	}
	if total != 2000+30+40 { // the 2000 is the turn the commit before the window ended
		t.Errorf("usage %+v", usage)
	}

	// The reads use indexes.
	for _, q := range []string{workCommitsSince, workCommitBefore, workUsageSince, workUsageOfRoot} {
		args := []any{since.UnixMilli()}
		if q == workCommitBefore {
			args = []any{a.ID, since.UnixMilli()}
		}
		if q == workUsageOfRoot {
			args = []any{a.ID, int64(0), since.UnixMilli()}
		}
		rows, err := s.db.Query(`EXPLAIN QUERY PLAN `+q, args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			rows.Scan(&id, &parent, &unused, &detail)
			plan += detail + "; "
		}
		rows.Close()
		if !strings.Contains(plan, "USING INDEX") && !strings.Contains(plan, "USING COVERING INDEX") {
			t.Errorf("no index for %q: %s", q, plan)
		}
	}
}

// GUI-051: a call that names no model is priced by the model in force when it was
// made, not the one the session has now.
func TestWorkUsageKeepsTheModelInForce(t *testing.T) {
	ctx := context.Background()
	clock := &workClock{now: time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)}
	s := workStore(t, clock)
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "claude-code", ProviderID: "", Model: "small"})
	// An agent that does not always name its model; the first call is the only name.
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{Model: "big", InputTokens: 1})
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 10})
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	put(t, s, clock, a.ID, event.ModelChanged, event.ModelChangedData{Model: "small"})
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 100})
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "h0"})
	put(t, s, clock, a.ID, event.ToolCall, commitCmd("git commit -m x"))
	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "h1", Commits: []event.CommitInfo{{SHA: "h1"}}})
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	s.FoldWork(ctx, time.Now().Add(time.Minute), nil)
	_, usage, _, err := s.WorkSince(ctx, clock.now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]string{}
	for _, u := range usage {
		got[u.Input] = u.Model
	}
	if got[11] != "big" || got[100] != "small" {
		t.Errorf("models by input: %v", got)
	}
}
