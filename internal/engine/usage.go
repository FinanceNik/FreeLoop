package engine

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
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

// UsageRow is a store row with its estimated cost, in all and per kind of
// token, so the dashboard can split cost the way it splits tokens.
type UsageRow struct {
	store.UsageRow
	Cost           float64 `json:"cost"`
	CostInput      float64 `json:"costInput"`
	CostOutput     float64 `json:"costOutput"`
	CostCacheRead  float64 `json:"costCacheRead"`
	CostCacheWrite float64 `json:"costCacheWrite"`
	// SavedByCache is what the cache reads would have cost as plain input
	// tokens, less what they did.
	SavedByCache float64 `json:"savedByCache"`
	Priced       bool    `json:"priced"`
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
			r.CostInput = p.Cost(u.InputTokens, 0, 0, 0)
			r.CostOutput = p.Cost(0, u.OutputTokens, 0, 0)
			r.CostCacheRead = p.Cost(0, 0, u.CacheReadTokens, 0)
			r.CostCacheWrite = p.Cost(0, 0, 0, u.CacheWriteTokens)
			r.SavedByCache = max(0, p.Cost(u.CacheReadTokens, 0, 0, 0)-r.CostCacheRead)
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

// RecentBucket is one step of a graph of recent usage (GUI-023).
type RecentBucket struct {
	// Start is when it begins, in milliseconds since 1970.
	Start      int64   `json:"start"`
	Input      int64   `json:"input"`
	Output     int64   `json:"output"`
	CacheRead  int64   `json:"cacheRead"`
	CacheWrite int64   `json:"cacheWrite"`
	Calls      int64   `json:"calls"`
	Cost       float64 `json:"cost"`
	// Unpriced counts model calls with no price, which the cost leaves out.
	Unpriced int64 `json:"unpriced"`
}

// RecentUsageReport is the last hours of usage, with every step present.
type RecentUsageReport struct {
	Hours       int            `json:"hours"`
	StepMinutes int            `json:"stepMinutes"`
	Buckets     []RecentBucket `json:"buckets"`
}

// RecentUsage returns the last hours of usage, up to two days: every 15 minutes
// for up to 12 hours, else every 30, which keeps the bars readable in a pane.
func (e *Engine) RecentUsage(ctx context.Context, hours int) (RecentUsageReport, error) {
	if hours < 1 || hours > 48 {
		return RecentUsageReport{}, fmt.Errorf("recent usage covers 1 to 48 hours, not %d", hours)
	}
	step := 15 * time.Minute
	if hours > 12 {
		step = 30 * time.Minute
	}
	n := int(time.Duration(hours) * time.Hour / step)
	since := time.Now().Truncate(step).Add(-time.Duration(n-1) * step)
	rows, err := e.store.RecentUsage(ctx, since)
	if err != nil {
		return RecentUsageReport{}, err
	}
	out := RecentUsageReport{Hours: hours, StepMinutes: int(step / time.Minute), Buckets: make([]RecentBucket, n)}
	for i := range out.Buckets {
		out.Buckets[i].Start = since.Add(time.Duration(i) * step).UnixMilli()
	}
	prices := e.Config().Prices
	for _, u := range rows {
		i := int(u.Start.Sub(since) / step)
		if i < 0 || i >= n {
			continue
		}
		b := &out.Buckets[i]
		b.Input += u.Input
		b.Output += u.Output
		b.CacheRead += u.CacheRead
		b.CacheWrite += u.CacheWrite
		b.Calls += u.Calls
		if p, ok := config.FindPrice(prices, u.ProviderID, u.Model); ok {
			b.Cost += p.Cost(u.Input, u.Output, u.CacheRead, u.CacheWrite)
		} else {
			b.Unpriced += u.Calls
		}
	}
	return out, nil
}
