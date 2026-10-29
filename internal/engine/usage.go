package engine

import (
	"context"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// ModelCost is one model's tokens on a day and their estimated cost
// (PRV-008); Priced is false when the price table has no entry for it.
type ModelCost struct {
	ProviderID       string  `json:"providerId"`
	Model            string  `json:"model"`
	InputTokens      int64   `json:"inputTokens"`
	OutputTokens     int64   `json:"outputTokens"`
	CacheReadTokens  int64   `json:"cacheReadTokens"`
	CacheWriteTokens int64   `json:"cacheWriteTokens"`
	Calls            int64   `json:"calls"`
	Cost             float64 `json:"cost"`
	Priced           bool    `json:"priced"`
}

// DayCost is a day's usage across sessions. Cost adds up the priced models.
type DayCost struct {
	Day    string      `json:"day"`
	Cost   float64     `json:"cost"`
	Models []ModelCost `json:"models"`
}

// Usage returns the estimated cost per day for the last days days, today
// included, newest first; days without model calls are left out. Costs come
// from the current price table, so a price added later applies to past
// usage too.
func (e *Engine) Usage(ctx context.Context, days int) ([]DayCost, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, 1-days).Format(time.DateOnly)
	rows, err := e.store.UsageSince(ctx, since)
	if err != nil {
		return nil, err
	}
	prices := e.Config().Prices
	out := []DayCost{}
	for _, u := range rows {
		if len(out) == 0 || out[len(out)-1].Day != u.Day {
			out = append(out, DayCost{Day: u.Day})
		}
		d := &out[len(out)-1]
		m := ModelCost{ProviderID: u.ProviderID, Model: u.Model, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
			CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens, Calls: u.Calls}
		if p, ok := config.FindPrice(prices, u.ProviderID, u.Model); ok {
			m.Cost, m.Priced = p.Cost(u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens), true
			d.Cost += m.Cost
		}
		d.Models = append(d.Models, m)
	}
	return out, nil
}
