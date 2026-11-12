package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// GUI-051: the first commit in the window costs what was spent since the commit
// before it, not everything the session spent before the window.
func TestWorkCostFirstCommitInTheWindowCountsOnlyItsOwnWork(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	cfg := e.Config()
	cfg.Prices = []config.Price{{Model: "m", Input: 1, Output: 0}}
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
		time.Sleep(3 * time.Millisecond)
	}
	turn := func(millions int64, sha string) {
		put(event.UsageUpdate, event.UsageData{ProviderID: "p", Model: "m", InputTokens: millions * 1_000_000})
		put(event.ToolCall, map[string]any{"toolCallId": sha, "rawInput": map[string]any{"command": "git commit -m x"}})
		put(event.GitState, event.GitStateData{Repo: true, Head: sha, Commits: []event.CommitInfo{{SHA: sha, Added: 1, Files: 1}}})
		put(event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	}
	put(event.GitState, event.GitStateData{Repo: true, Head: "h0"})
	turn(50, "c1")
	turn(70, "c2")
	since := time.Now()
	time.Sleep(10 * time.Millisecond)
	turn(3, "c3")
	turn(4, "c4")

	e.foldWork(ctx)
	rep, err := e.workCost(ctx, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Commits) != 2 || rep.Commits[0].SHA != "c3" || rep.Commits[1].SHA != "c4" {
		t.Fatalf("commits = %+v", rep.Commits)
	}
	if c := rep.Commits[0]; c.Cost < 2.999 || c.Cost > 3.001 {
		t.Errorf("first commit in the window cost %v, want 3", c.Cost)
	}
	if c := rep.Commits[1]; c.Cost < 3.999 || c.Cost > 4.001 {
		t.Errorf("second commit cost %v, want 4", c.Cost)
	}
}
