package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Recent usage (GUI-023): tokens per five minutes, for the graphs of the last
// hours. The usage table counts per day, which is too coarse to watch. Like the
// activity table this is folded from the transcripts when asked, only for what
// moved on, and old rows go: the day counts keep the long view.

const (
	// RecentBucket is the finest a graph of recent usage can show.
	RecentBucket = 5 * time.Minute
	// recentKeep is how far back the buckets go.
	recentKeep = 72 * time.Hour
)

// RecentUsage is one model's use from one project and agent in one bucket.
type RecentUsage struct {
	Start      time.Time
	ProviderID string
	Model      string
	// ProjectID, ProjectRoot and HarnessID say where it was used, for the usage
	// pane's filters; empty for a session that has since been deleted.
	ProjectID   string
	ProjectRoot string
	HarnessID   string
	Input       int64
	Output      int64
	CacheRead   int64
	CacheWrite  int64
	// CacheWrite1h is the part of CacheWrite that lasts an hour.
	CacheWrite1h int64
	Calls        int64
}

// RecentUsage returns the buckets from since on, after folding what happened.
func (s *Store) RecentUsage(ctx context.Context, since time.Time) ([]RecentUsage, error) {
	if err := s.foldRecent(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT u.bucket, u.provider_id, u.model, COALESCE(p.project_id, ''), COALESCE(p.project_root, ''), COALESCE(p.harness_id, ''), SUM(u.input), SUM(u.output), SUM(u.cache_read), SUM(u.cache_write), SUM(u.cache_write_1h), SUM(u.calls)
		FROM usage_recent u LEFT JOIN sessions p ON p.id = u.session_id WHERE u.bucket >= ?
		GROUP BY u.bucket, u.provider_id, u.model, 4, 5, 6 ORDER BY u.bucket`, since.Unix())
	if err != nil {
		return nil, fmt.Errorf("store: recent usage: %w", err)
	}
	defer rows.Close()
	var out []RecentUsage
	for rows.Next() {
		var u RecentUsage
		var bucket int64
		if err := rows.Scan(&bucket, &u.ProviderID, &u.Model, &u.ProjectID, &u.ProjectRoot, &u.HarnessID, &u.Input, &u.Output, &u.CacheRead, &u.CacheWrite, &u.CacheWrite1h, &u.Calls); err != nil {
			return nil, fmt.Errorf("store: recent usage: %w", err)
		}
		u.Start = time.Unix(bucket, 0)
		out = append(out, u)
	}
	return out, rows.Err()
}

// foldRecent counts the usage of sessions whose transcripts have moved on. A
// session idle for longer than the buckets go back has nothing to add.
func (s *Store) foldRecent(ctx context.Context) error {
	s.recentMu.Lock()
	defer s.recentMu.Unlock()
	cutoff := s.now().Add(-recentKeep)
	rows, err := s.db.QueryContext(ctx, `SELECT id, recent_seq, copied_seq, provider_id, model FROM sessions WHERE recent_seq != last_seq AND updated_at >= ?`, cutoff.UnixMilli())
	if err != nil {
		return fmt.Errorf("store: recent usage: %w", err)
	}
	type stale struct {
		id                string
		from, copied      int64
		providerID, model string
	}
	var list []stale
	for rows.Next() {
		var st stale
		if err := rows.Scan(&st.id, &st.from, &st.copied, &st.providerID, &st.model); err != nil {
			rows.Close()
			return fmt.Errorf("store: recent usage: %w", err)
		}
		list = append(list, st)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: recent usage: %w", err)
	}
	for _, st := range list {
		events, err := s.Events(ctx, st.id, st.from)
		if err != nil {
			continue // deleted meanwhile
		}
		if len(events) == 0 {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("store: recent usage: %w", err)
		}
		for _, ev := range events {
			if ev.Type != event.UsageUpdate || ev.Seq <= st.copied || ev.TS.Before(cutoff) {
				continue
			}
			var d event.UsageData
			json.Unmarshal(ev.Data, &d)
			if d.InputTokens+d.OutputTokens+d.CacheReadTokens+d.CacheWriteTokens == 0 {
				continue
			}
			bucket := ev.TS.Truncate(RecentBucket).Unix()
			provider, model := d.ProviderID, d.Model
			if provider == "" {
				provider = st.providerID
			}
			if model == "" {
				model = st.model
			}
			model = pricedModel(model, d)
			if _, err := tx.ExecContext(ctx, `INSERT INTO usage_recent (bucket, session_id, provider_id, model, input, output, cache_read, cache_write, cache_write_1h, calls)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (bucket, session_id, provider_id, model) DO UPDATE SET input = input + excluded.input, output = output + excluded.output,
				cache_read = cache_read + excluded.cache_read, cache_write = cache_write + excluded.cache_write, cache_write_1h = cache_write_1h + excluded.cache_write_1h, calls = calls + excluded.calls`,
				bucket, st.id, provider, model, d.InputTokens, d.OutputTokens, d.CacheReadTokens, d.CacheWriteTokens, d.CacheWrite1hTokens, callCount(d)); err != nil {
				tx.Rollback()
				return fmt.Errorf("store: recent usage: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET recent_seq = ? WHERE id = ?`, events[len(events)-1].Seq, st.id); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: recent usage: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: recent usage: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM usage_recent WHERE bucket < ?`, cutoff.Unix()); err != nil {
		return fmt.Errorf("store: recent usage: %w", err)
	}
	return nil
}
