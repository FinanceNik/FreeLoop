package engine

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/store"
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

// UsageRow is a store row with its estimated cost.
type UsageRow struct {
	store.UsageRow
	Cost   float64 `json:"cost"`
	Priced bool    `json:"priced"`
}

// UsageReport is the usage the analytics view works from (PRV-008): one row
// per day, session and model from Since to Until (local dates, inclusive),
// newest first, costed with the current price table.
type UsageReport struct {
	Since string     `json:"since"`
	Until string     `json:"until"`
	Rows  []UsageRow `json:"rows"`
}

// MaxUsageDays bounds a report.
const MaxUsageDays = 731

// UsageReport returns the last days days of usage, today included.
func (e *Engine) UsageReport(ctx context.Context, days int) (UsageReport, error) {
	if days <= 0 || days > MaxUsageDays {
		days = MaxUsageDays
	}
	now := time.Now()
	rep := UsageReport{Since: now.AddDate(0, 0, 1-days).Format(time.DateOnly), Until: now.Format(time.DateOnly)}
	rows, err := e.store.UsageRowsSince(ctx, rep.Since)
	if err != nil {
		return UsageReport{}, err
	}
	prices := e.Config().Prices
	rep.Rows = make([]UsageRow, 0, len(rows))
	for _, u := range rows {
		r := UsageRow{UsageRow: u}
		if p, ok := config.FindPrice(prices, u.ProviderID, u.Model); ok {
			r.Cost, r.Priced = p.Cost(u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens), true
		}
		rep.Rows = append(rep.Rows, r)
	}
	return rep, nil
}

// ExportUsage writes the last days days of usage as CSV to path, one line
// per day, session and model, for a spreadsheet.
func (e *Engine) ExportUsage(ctx context.Context, days int, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("the export needs an absolute path")
	}
	rep, err := e.UsageReport(ctx, days)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Write([]string{"day", "session", "title", "project", "agent", "provider", "model", "input", "output", "cache_read", "cache_write", "calls", "cost_usd"})
	n := func(v int64) string { return strconv.FormatInt(v, 10) }
	for _, r := range rep.Rows {
		cost := ""
		if r.Priced {
			cost = strconv.FormatFloat(r.Cost, 'f', 6, 64)
		}
		w.Write([]string{r.Day, r.SessionID, cell(r.Title), cell(r.ProjectRoot), r.HarnessID, cell(r.ProviderID), cell(r.Model),
			n(r.InputTokens), n(r.OutputTokens), n(r.CacheReadTokens), n(r.CacheWriteTokens), n(r.Calls), cost})
	}
	w.Flush()
	env, err := e.localEnv(filepath.Dir(path), nil)
	if err != nil {
		return err
	}
	return env.FS().WriteFile(path, b.Bytes())
}

// cell keeps a spreadsheet from running text that starts like a formula.
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
