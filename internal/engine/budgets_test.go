package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// PRV-017: a budget adds up the priced usage of its period, for every session or one
// profile's, says what it leaves out, warns once per level, and holds the queue.
func TestBudgets(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	var notices []Notice
	e.onNotice = func(n Notice) { notices = append(notices, n) }
	cfg := e.Config()
	cfg.Prices = []config.Price{{Model: "m", Input: 1_000_000}} // a dollar a token
	cfg.Budgets = []config.Budget{{Period: config.BudgetDay, Amount: 10}, {Period: config.BudgetMonth, Amount: 100, ProfileID: "other"}}
	cfg.Notifications.Enabled = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	ses, err := e.store.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin", ProviderID: "s", Model: "m", ProfileID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	spend := func(d event.UsageData) {
		e.emit(ses.ID, event.UsageUpdate, d)
	}
	now := time.Now()
	spend(event.UsageData{InputTokens: 8, Model: "m"})
	spend(event.UsageData{InputTokens: 5, Model: "unpriced"})
	spend(event.UsageData{Measured: event.MeasuredNone})

	states, err := e.budgetStates(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0].Spent != 8 || states[0].Level != BudgetWarning || !states[0].Unpriced || states[0].Unreported != 1 {
		t.Errorf("the daily budget = %+v", states[0])
	}
	if states[1].Spent != 0 || states[1].Level != "" {
		t.Errorf("another profile's budget = %+v", states[1])
	}
	e.checkBudgets(ctx, now)
	e.checkBudgets(ctx, now)
	if len(notices) != 1 || notices[0].Title != "Budget nearly spent" {
		t.Errorf("notices at 80%% = %+v", notices)
	}

	spend(event.UsageData{InputTokens: 3, Model: "m"})
	e.checkBudgets(ctx, now)
	e.checkBudgets(ctx, now)
	if len(notices) != 2 || notices[1].Title != "Budget reached" {
		t.Errorf("notices when reached = %+v", notices)
	}

	// The queue starts nothing under it.
	if _, err := e.AddTask(config.Task{Prompt: "go", ProjectRoot: root, ProfileID: "main"}); err != nil {
		t.Fatal(err)
	}
	e.runQueue(ctx, now)
	q, _ := e.Queue()
	if q.Tasks[0].State != config.TaskWaiting || !strings.Contains(q.Tasks[0].Note, "daily budget of $10.00 is reached") {
		t.Errorf("the queue under a reached budget = %+v", q.Tasks[0])
	}
}

// PRV-008: an adjustment, the tokens an agent's result counts and its calls did not
// show (a subagent's model, say), has no calls of its own; its tokens still cost, in the
// day's total and in a budget.
func TestAnAdjustmentAloneStillCosts(t *testing.T) {
	ctx := context.Background()
	e, _ := queueEngine(t)
	cfg := e.Config()
	cfg.Prices = []config.Price{{Model: "sub", Input: 1_000_000}}
	cfg.Budgets = []config.Budget{{Period: config.BudgetDay, Amount: 100}}
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	ses, err := e.store.CreateSession(ctx, session.Session{Title: "a", HarnessID: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	e.emit(ses.ID, event.UsageUpdate, event.UsageData{InputTokens: 7, Model: "sub", Adjustment: true})
	days, err := e.Usage(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Cost != 7 {
		t.Errorf("the day's cost = %+v", days)
	}
	states, _ := e.budgetStates(ctx, time.Now())
	if len(states) != 1 || states[0].Spent != 7 {
		t.Errorf("the budget = %+v", states)
	}
}
