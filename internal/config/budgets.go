package config

import (
	"fmt"
	"math"
	"time"
)

// Budget is an amount of estimated spending for a day or a month, in US dollars,
// for every session or one profile's (PRV-017). It warns; it stops nothing that runs,
// and the queue starts nothing under a budget that is reached.
type Budget struct {
	// Period is day or month, in local time.
	Period string  `yaml:"period" json:"period"`
	Amount float64 `yaml:"amount" json:"amount"`
	// ProfileID limits the budget to sessions on one profile; empty, every session.
	ProfileID string `yaml:"profile,omitempty" json:"profileId"`
}

// Budget periods.
const (
	BudgetDay   = "day"
	BudgetMonth = "month"
)

// BudgetWarnAt is the share of a budget that warns.
const BudgetWarnAt = 0.8

// Start is the first day of the period now is in, as YYYY-MM-DD.
func (b Budget) Start(now time.Time) string {
	now = now.In(time.Local)
	if b.Period == BudgetMonth {
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format(time.DateOnly)
	}
	return now.Format(time.DateOnly)
}

func validateBudgets(budgets []Budget) ([]Budget, []Issue) {
	var issues []Issue
	out := budgets[:0:0]
	for i, b := range budgets {
		bad := ""
		switch {
		case b.Period != BudgetDay && b.Period != BudgetMonth:
			bad = fmt.Sprintf("the period is day or month, not %q", b.Period)
		case !(b.Amount > 0) || math.IsInf(b.Amount, 0):
			bad = "a budget needs an amount above zero"
		}
		if bad != "" {
			issues = append(issues, Issue{Field: fmt.Sprintf("budgets[%d]", i), Message: bad})
			continue
		}
		out = append(out, b)
	}
	return out, issues
}
