package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

type workClock struct{ now time.Time }

func (c *workClock) tick(d time.Duration) { c.now = c.now.Add(d) }

func workStore(t *testing.T, clock *workClock) *Store {
	t.Helper()
	s, err := Open(Options{Dir: t.TempDir(), Now: func() time.Time { return clock.now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func put(t *testing.T, s *Store, clock *workClock, sid string, typ event.Type, data any) {
	t.Helper()
	clock.tick(time.Second)
	b, _ := json.Marshal(data)
	if _, err := s.Append(context.Background(), sid, typ, b); err != nil {
		t.Fatal(err)
	}
}

func commitCmd(cmd string) map[string]any {
	return map[string]any{"toolCallId": "t", "title": "Bash", "rawInput": map[string]any{"command": cmd}}
}

// GUI-051: a commit is the work of the session that made it, once, however many
// sessions in the checkout saw HEAD move; its slice is the usage up to the end of
// the turn it landed in, late adjustments included.
func TestWorkCommitsGoToTheSessionThatMadeThem(t *testing.T) {
	ctx := context.Background()
	clock := &workClock{now: time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)}
	s := workStore(t, clock)
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin", ProviderID: "p", Model: "m", ProjectID: "proj"})
	child, _ := s.CreateSession(ctx, session.Session{Title: "a's helper", HarnessID: "builtin", ProviderID: "p", Model: "m", ProjectID: "proj", ParentID: a.ID})
	b, _ := s.CreateSession(ctx, session.Session{Title: "b", HarnessID: "builtin", ProviderID: "p", Model: "m", ProjectID: "proj"})
	two := []event.CommitInfo{{SHA: "c1", At: 1, Added: 3, Deleted: 1, Files: 1}, {SHA: "c2", At: 2, Added: 5, Deleted: 0, Files: 2}}

	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "c0"})
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 10, OutputTokens: 1})
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	put(t, s, clock, child.ID, event.UsageUpdate, event.UsageData{InputTokens: 100, OutputTokens: 1})
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 20, OutputTokens: 1})
	put(t, s, clock, a.ID, event.ToolCall, commitCmd(`git add -A && git commit -m "two things"`))
	// Another tool call finishing first changes only the count of changed files.
	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "c0", Dirty: 3})
	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "c2", Commits: two})
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 5, Adjustment: true})
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	committedAt := clock.now
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 7, OutputTokens: 1})
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	// b only saw HEAD move, and is folded first, as the more recent.
	put(t, s, clock, b.ID, event.GitState, event.GitStateData{Repo: true, Head: "c0"})
	put(t, s, clock, b.ID, event.ToolCall, commitCmd("git log --oneline"))
	put(t, s, clock, b.ID, event.GitState, event.GitStateData{Repo: true, Head: "c2", Commits: two})
	put(t, s, clock, b.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})

	if left, err := s.FoldWork(ctx, time.Now().Add(time.Minute), nil); err != nil || left != 0 {
		t.Fatalf("fold: %d %v", left, err)
	}
	commits, usage, unattributed, err := s.WorkSince(ctx, clock.now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || unattributed != 0 {
		t.Fatalf("commits %+v, unattributed %d", commits, unattributed)
	}
	for _, c := range commits {
		if c.RootID != a.ID || c.SessionID != a.ID || !c.UnitTS.Equal(committedAt.Truncate(time.Millisecond)) {
			t.Errorf("commit %+v, want a's at %v", c, committedAt)
		}
	}
	byInput := map[int64]WorkUsage{}
	for _, u := range usage {
		byInput[u.Input] = u
	}
	if len(usage) != 4 || byInput[25].RootID != a.ID || byInput[100].RootID != a.ID || byInput[100].SessionID != child.ID || byInput[7].RootID != a.ID {
		t.Fatalf("usage rows: %+v", usage)
	}
	if !byInput[25].TS.Equal(committedAt.Truncate(time.Millisecond)) {
		t.Errorf("the adjustment after the commit is not in its turn: %+v", byInput[25])
	}

	// Folding again, after b moved on, changes nothing.
	put(t, s, clock, b.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	s.FoldWork(ctx, time.Now().Add(time.Minute), nil)
	commits, _, unattributed, _ = s.WorkSince(ctx, clock.now.Add(-time.Hour))
	if len(commits) != 2 || commits[0].RootID != a.ID || unattributed != 0 {
		t.Fatalf("after folding again: %+v, %d", commits, unattributed)
	}
}

// A commit the app made between turns is a unit then; one no session made stays
// unattributed.
func TestWorkCommitsTheAppMadeAndNobodyMade(t *testing.T) {
	ctx := context.Background()
	clock := &workClock{now: time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)}
	s := workStore(t, clock)
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin", ProviderID: "p", Model: "m"})
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 10})
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "x1", Cause: event.CauseCommit, Commits: []event.CommitInfo{{SHA: "x1", Added: -1, Deleted: -1, Files: -1}}})
	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "x2", Commits: []event.CommitInfo{{SHA: "x2", At: clock.now.Unix(), Added: 1, Deleted: 1, Files: 1}}})
	s.FoldWork(ctx, time.Now().Add(time.Minute), nil)
	commits, _, unattributed, _ := s.WorkSince(ctx, clock.now.Add(-time.Hour))
	if len(commits) != 1 || commits[0].SHA != "x1" || unattributed != 1 {
		t.Fatalf("commits %+v, unattributed %d", commits, unattributed)
	}
}

// Transcripts from before commits were listed are read with git, once.
func TestWorkBackfillAsksGitOnce(t *testing.T) {
	ctx := context.Background()
	clock := &workClock{now: time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)}
	s := workStore(t, clock)
	clock.now = clock.now.Add(-48 * time.Hour)
	a, _ := s.CreateSession(ctx, session.Session{Title: "old", HarnessID: "claude-code"})
	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "o1"})
	put(t, s, clock, a.ID, event.ToolCall, commitCmd("git commit -am fix"))
	put(t, s, clock, a.ID, event.GitState, event.GitStateData{Repo: true, Head: "o2"})
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	asked := 0
	resolve := func(_ context.Context, sid, old, now string) ([]event.CommitInfo, bool) {
		asked++
		if sid != a.ID || old != "o1" || now != "o2" {
			t.Errorf("asked about %s %s..%s", sid, old, now)
		}
		return []event.CommitInfo{{SHA: "o2", Added: 2, Deleted: 0, Files: 1}}, true
	}
	s.FoldWork(ctx, time.Now().Add(time.Minute), resolve)
	put(t, s, clock, a.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	s.FoldWork(ctx, time.Now().Add(time.Minute), resolve)
	commits, _, _, _ := s.WorkSince(ctx, clock.now.Add(-time.Hour))
	if asked != 1 || len(commits) != 1 || commits[0].SHA != "o2" || commits[0].Added != 2 {
		t.Fatalf("asked %d times, commits %+v", asked, commits)
	}
}

// GUI-051: a limit's run is read from its first report to its highest, however
// parallel sessions ordered their reports, and the five-minute buckets stay.
func TestLimitSpansAndBuckets(t *testing.T) {
	ctx := context.Background()
	clock := &workClock{now: time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)}
	s := workStore(t, clock)
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "claude-code", ProviderID: "", Model: "opus"})
	report := func(used float64) {
		put(t, s, clock, a.ID, event.LimitUpdate, event.LimitData{Source: "Agent", Windows: []event.LimitWindow{{ID: "five_hour", Label: "5-hour limit", UsedPercent: used, ResetsAt: 1_800_000_000, Status: "allowed"}}})
	}
	report(10)
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 1000, Model: "opus"})
	report(30)
	report(25)
	clock.tick(10 * time.Minute)
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 500, Model: "opus"})
	report(40)
	clock.tick(time.Hour)
	put(t, s, clock, a.ID, event.UsageUpdate, event.UsageData{InputTokens: 99, Model: "opus"})
	s.IndexAll(ctx)
	s.FoldWork(ctx, time.Now().Add(time.Minute), nil)
	spans, err := s.LimitSpans(ctx, clock.now.Add(-24*time.Hour))
	if err != nil || len(spans) != 1 {
		t.Fatalf("spans %+v %v", spans, err)
	}
	sp := spans[0]
	if sp.FirstUse != 10 || sp.Peak != 40 || sp.Reports != 4 || len(sp.Harnesses) != 1 || sp.Harnesses[0] != "claude-code" {
		t.Fatalf("span %+v", sp)
	}
	rows, err := s.BucketUsage(ctx, sp.First, sp.PeakAt, sp.Harnesses)
	if err != nil || len(rows) != 1 || rows[0].Input != 1500 {
		t.Fatalf("usage in the run: %+v %v", rows, err)
	}
	if rows, _ := s.BucketUsage(ctx, sp.First, sp.PeakAt, []string{"other"}); len(rows) != 0 {
		t.Errorf("another agent's usage counted: %+v", rows)
	}
}
