package engine

import (
	"context"
	"time"
)

// limitHistory is how far back the usage page looks for a limit's past runs.
const limitHistory = 90 * 24 * time.Hour

// LimitPeriod is one run of an account's limit, as its agent reported it (GUI-028).
type LimitPeriod struct {
	Source     string    `json:"source"`
	Window     string    `json:"window"`
	Label      string    `json:"label"`
	ResetsAt   time.Time `json:"resetsAt"`
	Used       float64   `json:"used"`
	Peak       float64   `json:"peak"`
	LastReport time.Time `json:"lastReport"`
	Warned     int       `json:"warned"`
	Refused    int       `json:"refused"`
}

// Limits returns the runs of each limit agents have reported, newest first. It asks no
// agent or vendor anything: it is what the sessions' model calls already brought back.
func (e *Engine) Limits(ctx context.Context) ([]LimitPeriod, error) {
	rows, err := e.store.LimitPeriods(ctx, time.Now().Add(-limitHistory))
	if err != nil {
		return nil, err
	}
	out := make([]LimitPeriod, 0, len(rows))
	for _, r := range rows {
		out = append(out, LimitPeriod{Source: r.Source, Window: r.Window, Label: r.Label, ResetsAt: r.ResetsAt, Used: r.Used, Peak: r.Peak, LastReport: r.LastReport, Warned: r.Warned, Refused: r.Refused})
	}
	return out, nil
}
