package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// BudgetState is a budget and what has been spent against it this period (PRV-017).
type BudgetState struct {
	config.Budget
	// Since is the period's first day, YYYY-MM-DD.
	Since string  `json:"since"`
	Spent float64 `json:"spent"`
	// Unpriced says some usage had no price, and Unreported counts turns an agent
	// reported no tokens for: neither is in Spent, so it may be more.
	Unpriced   bool  `json:"unpriced"`
	Unreported int64 `json:"unreported"`
	// Level is "", warning (from BudgetWarnAt) or reached.
	Level string `json:"level"`
}

// Budget levels.
const (
	BudgetWarning = "warning"
	BudgetReached = "reached"
)

// Budgets returns every budget with its spending so far this period.
func (e *Engine) Budgets(ctx context.Context) ([]BudgetState, error) {
	return e.budgetStates(ctx, time.Now())
}

func (e *Engine) budgetStates(ctx context.Context, now time.Time) ([]BudgetState, error) {
	cfg := e.Config()
	out := []BudgetState{}
	if len(cfg.Budgets) == 0 {
		return out, nil
	}
	since := ""
	for _, b := range cfg.Budgets {
		if s := b.Start(now); since == "" || s < since {
			since = s
		}
	}
	rows, err := e.store.UsageRowsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	prices := config.EffectivePrices(cfg)
	for _, b := range cfg.Budgets {
		st := BudgetState{Budget: b, Since: b.Start(now)}
		for _, r := range rows {
			if r.Day < st.Since || (b.ProfileID != "" && r.ProfileID != b.ProfileID) {
				continue
			}
			st.Unreported += r.Unreported
			if r.Calls == 0 {
				continue
			}
			if p, ok := config.FindPrice(prices, r.ProviderID, r.Model); ok {
				st.Spent += p.Cost(r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheWriteTokens, r.CacheWrite1hTokens)
			} else {
				st.Unpriced = true
			}
		}
		switch {
		case st.Spent >= b.Amount:
			st.Level = BudgetReached
		case st.Spent >= b.Amount*config.BudgetWarnAt:
			st.Level = BudgetWarning
		}
		out = append(out, st)
	}
	return out, nil
}

// budgetName says which budget it is, in a sentence.
func (b BudgetState) name() string {
	period := "daily"
	if b.Period == config.BudgetMonth {
		period = "monthly"
	}
	name := fmt.Sprintf("The %s budget of $%.2f", period, b.Amount)
	if b.ProfileID != "" {
		name += " for profile " + b.ProfileID
	}
	return name
}

// budgetHeld returns why the queue should not start a session on profileID: a budget
// that covers it is reached, or "".
func budgetHeld(states []BudgetState, profileID string) string {
	for _, b := range states {
		if b.Level == BudgetReached && (b.ProfileID == "" || b.ProfileID == profileID) {
			return fmt.Sprintf("%s is reached ($%.2f spent)", b.name(), b.Spent)
		}
	}
	return ""
}

// checkBudgets tells the user once per period and level that a budget is near or
// reached (PRV-017). It runs with the schedule loop.
func (e *Engine) checkBudgets(ctx context.Context, now time.Time) {
	states, err := e.budgetStates(ctx, now)
	if err != nil || len(states) == 0 {
		return
	}
	cfg := e.Config().Notifications
	for _, b := range states {
		if b.Level == "" {
			continue
		}
		key := fmt.Sprintf("%s %s %.2f %s", b.Period, b.ProfileID, b.Amount, b.Since)
		e.budgetsMu.Lock()
		if e.budgetsWarned == nil {
			e.budgetsWarned = map[string]string{}
		}
		was := e.budgetsWarned[key]
		fresh := was != b.Level && was != BudgetReached
		if fresh {
			e.budgetsWarned[key] = b.Level
		}
		e.budgetsMu.Unlock()
		if !fresh || e.onNotice == nil || !cfg.Enabled {
			continue
		}
		title := "Budget nearly spent"
		if b.Level == BudgetReached {
			title = "Budget reached"
		}
		body := fmt.Sprintf("%s: $%.2f spent so far. Running sessions go on; the queue waits.", b.name(), b.Spent)
		if b.Level == BudgetWarning {
			body = fmt.Sprintf("%s: $%.2f spent so far, %.0f%%.", b.name(), b.Spent, 100*b.Spent/b.Amount)
		}
		e.onNotice(Notice{Kind: "budget", Title: title, Body: body})
	}
}
