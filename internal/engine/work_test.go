package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// GUI-051: a HEAD that moved forward lists its new commits with their lines; one
// that did not (an amend, a switch of branch) lists none, and an untrusted project
// gets the commits without running anything that could read its config.
func TestNewCommitsFollowAFastForwardOnly(t *testing.T) {
	dir := gitRepo(t)
	home := t.TempDir()
	env, err := environment.NewLocal(environment.LocalOptions{Root: dir, Guard: permission.NewPolicy(home, filepath.Join(home, "d")).Guard(dir)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	old := gitIn(t, dir, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("one\ntwo\nthree\n"), 0o644)
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "b")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c\n"), 0o644)
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "a and c")
	now := gitIn(t, dir, "rev-parse", "HEAD")

	got := newCommits(ctx, env, old, now, true)
	if len(got) != 2 || got[0].SHA != now || got[0].Files != 2 || got[0].Added != 2 || got[0].Deleted != 1 || got[1].Added != 3 || got[1].At == 0 {
		t.Fatalf("commits = %+v", got)
	}
	if got := newCommits(ctx, env, old, now, false); len(got) != 2 || got[0].Added != -1 || got[0].Files != -1 {
		t.Errorf("untrusted = %+v", got)
	}
	gitIn(t, dir, "commit", "-q", "--amend", "-m", "a and c, amended")
	amended := gitIn(t, dir, "rev-parse", "HEAD")
	if got := newCommits(ctx, env, now, amended, true); len(got) != 0 {
		t.Errorf("an amend listed %+v", got)
	}
	gitIn(t, dir, "checkout", "-q", "-b", "other", old)
	if got := newCommits(ctx, env, amended, old, true); len(got) != 0 {
		t.Errorf("a switch back listed %+v", got)
	}
	if got := newCommits(ctx, env, "", now, true); got != nil {
		t.Errorf("a first look listed %+v", got)
	}
}

// GUI-051: the report splits a turn's usage among the commits that landed in it,
// keeps spend that no commit followed apart, and says what a percent of a limit
// bought.
func TestWorkCostReport(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	cfg := e.Config()
	cfg.Prices = []config.Price{{Model: "m", Input: 1, Output: 10}}
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	ses, _ := demoSession(t, e, "work", "")
	put := func(typ event.Type, data any) {
		t.Helper()
		b, _ := json.Marshal(data)
		if _, err := e.store.Append(ctx, ses.ID, typ, b); err != nil {
			t.Fatal(err)
		}
	}
	limit := func(used float64) {
		put(event.LimitUpdate, event.LimitData{Source: "Agent", Windows: []event.LimitWindow{{ID: "five_hour", Label: "5-hour limit", UsedPercent: used, ResetsAt: time.Now().Add(3 * time.Hour).Unix(), Status: "allowed"}}})
	}
	limit(5)
	put(event.UsageUpdate, event.UsageData{ProviderID: "p", Model: "m", InputTokens: 2_000_000, OutputTokens: 100_000})
	put(event.ToolCall, map[string]any{"toolCallId": "1", "rawInput": map[string]any{"command": "git commit -m x"}})
	put(event.GitState, event.GitStateData{Repo: true, Head: "h0"})
	put(event.GitState, event.GitStateData{Repo: true, Head: "h2", Commits: []event.CommitInfo{{SHA: "s1", Added: 1, Deleted: 0, Files: 1}, {SHA: "s2", Added: 9, Deleted: 1, Files: 2}}})
	put(event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	limit(25)
	put(event.UsageUpdate, event.UsageData{ProviderID: "p", Model: "m", InputTokens: 1_000_000})
	put(event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	limit(26)

	rep, err := e.WorkCost(ctx, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Commits) != 2 {
		t.Fatalf("commits = %+v", rep.Commits)
	}
	for _, c := range rep.Commits {
		// (2 + 1) dollars for the turn, shared by two commits.
		if c.Cost < 1.4999 || c.Cost > 1.5001 || c.LandedWith != 2 || c.Input != 1_000_000 || c.Model != "m" || c.SessionID != ses.ID || c.Week != weekOf(time.Now()) {
			t.Errorf("commit = %+v", c)
		}
	}
	var committed, not float64
	for _, s := range rep.Spend {
		if s.Committed {
			committed += s.Cost
		} else {
			not += s.Cost
		}
	}
	if committed < 2.999 || committed > 3.001 || not < 0.999 || not > 1.001 {
		t.Errorf("spend: committed %v, not %v (%+v)", committed, not, rep.Spend)
	}
	// From 5% to 26% is the run's step; its spend is what the agent used in it.
	if len(rep.Limits) != 1 || rep.Limits[0].Percent != 21 || rep.Limits[0].Spend < 3.999 || rep.Limits[0].Spend > 4.001 || rep.Limits[0].Runs != 1 || rep.Limits[0].Label != "5-hour limit" {
		t.Errorf("limits = %+v", rep.Limits)
	}
}

func TestWeekOf(t *testing.T) {
	for in, want := range map[string]string{"2026-11-11": "2026-11-09", "2026-11-09": "2026-11-09", "2026-11-15": "2026-11-09", "2026-11-16": "2026-11-16"} {
		d, _ := time.ParseInLocation(time.DateOnly, in, time.Local)
		if got := weekOf(d.Add(13 * time.Hour)); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}
