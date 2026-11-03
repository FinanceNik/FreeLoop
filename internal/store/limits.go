package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// An account's limits as agents reported them (GUI-028), one row per change. The
// table is no migration, for the reason addedColumns gives: an older build can still
// open a database that has it, and simply does not look.
const limitsTable = `CREATE TABLE IF NOT EXISTS limit_reports (
	session_id TEXT NOT NULL,
	seq        INTEGER NOT NULL,
	ts         INTEGER NOT NULL,
	source     TEXT NOT NULL,
	window_id  TEXT NOT NULL,
	label      TEXT NOT NULL,
	used       REAL NOT NULL,
	resets_at  INTEGER NOT NULL,
	status     TEXT NOT NULL,
	PRIMARY KEY (session_id, seq, window_id)
)`

func addLimitsTable(db *sql.DB) error {
	if _, err := db.Exec(limitsTable); err != nil {
		return fmt.Errorf("store: limits table: %w", err)
	}
	return nil
}

// limitRow is one window of one report.
type limitRow struct {
	seq, ts, resetsAt             int64
	source, window, label, status string
	used                          float64
}

// limitRows reads a limit_update. A copy in a fork is the same report, so it is not
// counted again, though parallel sessions' own reports are, under their own keys.
func limitRows(ev event.Event) []limitRow {
	var d event.LimitData
	if json.Unmarshal(ev.Data, &d) != nil {
		return nil
	}
	out := make([]limitRow, 0, len(d.Windows))
	for _, w := range d.Windows {
		out = append(out, limitRow{seq: ev.Seq, ts: ev.TS.UnixMilli(), resetsAt: w.ResetsAt, source: d.Source, window: w.ID, label: w.Label, status: w.Status, used: w.UsedPercent})
	}
	return out
}

// LimitPeriod is one run of a limit, up to the reset that ends it.
type LimitPeriod struct {
	Source   string
	Window   string
	Label    string
	ResetsAt time.Time
	// Used is the latest report, and Peak the most reported: parallel sessions report
	// in any order, so the latest can read lower.
	Used float64
	Peak float64
	// LastReport is when the agent last said.
	LastReport time.Time
	// Warned and Refused count reports of being close to the limit and over it.
	Warned  int
	Refused int
}

// LimitPeriods returns the runs of each limit that end from since on, newest first.
func (s *Store) LimitPeriods(ctx context.Context, since time.Time) ([]LimitPeriod, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source, window_id, MAX(label), resets_at, MAX(used), MAX(ts),
			SUM(status = 'warning'), SUM(status = 'rejected'),
			(SELECT r2.used FROM limit_reports r2 WHERE r2.source = r.source AND r2.window_id = r.window_id AND r2.resets_at = r.resets_at ORDER BY r2.ts DESC, r2.used DESC LIMIT 1)
		FROM limit_reports r WHERE resets_at >= ? GROUP BY source, window_id, resets_at ORDER BY resets_at DESC, window_id`, since.Unix())
	if err != nil {
		return nil, fmt.Errorf("store: limits: %w", err)
	}
	defer rows.Close()
	out := []LimitPeriod{}
	for rows.Next() {
		var p LimitPeriod
		var resets, last int64
		if err := rows.Scan(&p.Source, &p.Window, &p.Label, &resets, &p.Peak, &last, &p.Warned, &p.Refused, &p.Used); err != nil {
			return nil, fmt.Errorf("store: limits: %w", err)
		}
		p.ResetsAt, p.LastReport = time.Unix(resets, 0), time.UnixMilli(last)
		out = append(out, p)
	}
	return out, rows.Err()
}
