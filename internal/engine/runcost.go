package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// What a workflow run cost (WFL-015): from the usage each of its sessions recorded,
// priced with the current table as the Usage page prices it. Planning is the session
// the run was started from, up to the moment it began; Issues and Reviews are the
// run's sessions. An unpriced model, or an agent that gave no tokens, is said, never
// counted as nothing.

// CostByModel is what one model cost in a part of a run.
type CostByModel struct {
	ProviderID string  `json:"providerId"`
	Model      string  `json:"model"`
	Cost       float64 `json:"cost"`
	Priced     bool    `json:"priced"`
}

// CostPart is a part of a run's cost: the priced sum, its models, and whether
// anything in it has no price or no tokens.
type CostPart struct {
	Cost     float64       `json:"cost"`
	Unpriced bool          `json:"unpriced"`
	Models   []CostByModel `json:"models"`
}

// IssueCost is an issue's session and review, with the commit the issue ended at.
type IssueCost struct {
	CostPart
	Issue  string `json:"issue"`
	Commit string `json:"commit"`
}

// RunCost is a run's cost, in parts and by issue.
type RunCost struct {
	RunID    string      `json:"runId"`
	Planning CostPart    `json:"planning"`
	Issues   CostPart    `json:"issues"`
	Reviews  CostPart    `json:"reviews"`
	Total    float64     `json:"total"`
	Unpriced bool        `json:"unpriced"`
	PerIssue []IssueCost `json:"perIssue"`
}

// RunCost says what a run cost.
func (e *Engine) RunCost(ctx context.Context, runID string) (RunCost, error) {
	all, err := e.runs()
	if err != nil {
		return RunCost{}, err
	}
	i := slices.IndexFunc(all.Runs, func(r config.AFKRun) bool { return r.ID == runID })
	if i < 0 {
		return RunCost{}, errors.New("no such run")
	}
	return e.runCost(ctx, all.Runs[i]), nil
}

func (e *Engine) runCost(ctx context.Context, r config.AFKRun) RunCost {
	prices := config.EffectivePrices(e.Config())
	out := RunCost{RunID: r.ID, PerIssue: []IssueCost{}}
	if r.PlanningSession != "" {
		out.Planning = e.sessionCost(ctx, prices, r.StartedAt, r.PlanningSession)
	}
	var issues, reviews []string
	for _, s := range r.Steps {
		issues, reviews = append(issues, s.SessionID), append(reviews, s.ReviewID)
		out.PerIssue = append(out.PerIssue, IssueCost{CostPart: e.sessionCost(ctx, prices, time.Time{}, s.SessionID, s.ReviewID), Issue: s.Issue, Commit: s.Commit})
	}
	out.Issues = e.sessionCost(ctx, prices, time.Time{}, issues...)
	out.Reviews = e.sessionCost(ctx, prices, time.Time{}, reviews...)
	out.Total = out.Planning.Cost + out.Issues.Cost + out.Reviews.Cost
	out.Unpriced = out.Planning.Unpriced || out.Issues.Unpriced || out.Reviews.Unpriced
	return out
}

// issueCost is the cost of a run's step for an issue.
func (e *Engine) issueCost(ctx context.Context, s config.AFKStep) IssueCost {
	return IssueCost{CostPart: e.sessionCost(ctx, config.EffectivePrices(e.Config()), time.Time{}, s.SessionID, s.ReviewID), Issue: s.Issue, Commit: s.Commit}
}

// sessionCost prices what the sessions' model calls used, before a moment when one
// is given, by model.
func (e *Engine) sessionCost(ctx context.Context, prices []config.Price, before time.Time, sids ...string) CostPart {
	type key struct{ provider, model string }
	type tokens struct{ in, out, cr, cw, cw1h int64 }
	used := map[key]*tokens{}
	var order []key
	part := CostPart{Models: []CostByModel{}}
	for _, sid := range sids {
		if sid == "" {
			continue
		}
		ses, err := e.store.GetSession(ctx, sid)
		if err != nil {
			continue
		}
		evs, err := e.store.Events(ctx, sid, 0)
		if err != nil {
			continue
		}
		for _, ev := range evs {
			if ev.Type != event.UsageUpdate || (!before.IsZero() && !ev.TS.Before(before)) {
				continue
			}
			var d event.UsageData
			if json.Unmarshal(ev.Data, &d) != nil {
				continue
			}
			if d.Measured == event.MeasuredNone {
				part.Unpriced = true
				continue
			}
			k := key{cmp.Or(d.ProviderID, ses.ProviderID), cmp.Or(d.Model, ses.Model)}
			if !d.Adjustment {
				k.model = config.PricedModel(k.model, d.InputTokens+d.CacheReadTokens+d.CacheWriteTokens)
			}
			t := used[k]
			if t == nil {
				t = &tokens{}
				used[k] = t
				order = append(order, k)
			}
			t.in, t.out, t.cr, t.cw, t.cw1h = t.in+d.InputTokens, t.out+d.OutputTokens, t.cr+d.CacheReadTokens, t.cw+d.CacheWriteTokens, t.cw1h+d.CacheWrite1hTokens
		}
	}
	for _, k := range order {
		t := used[k]
		m := CostByModel{ProviderID: k.provider, Model: k.model}
		if p, ok := config.FindPrice(prices, k.provider, k.model); ok {
			m.Cost, m.Priced = p.Cost(t.in, t.out, t.cr, t.cw, t.cw1h), true
			part.Cost += m.Cost
		} else {
			part.Unpriced = true
		}
		part.Models = append(part.Models, m)
	}
	return part
}
