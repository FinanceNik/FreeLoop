package store

import (
	"context"
	"fmt"
	"strings"
)

// Activity is what the analytics view knows beyond tokens (GUI-021): tools,
// decisions and turns per hour and session, folded from the transcripts by the
// engine. Like usage, a deleted session's counts stay: the work happened. Its
// "asked" rows go with it, since they hold the text of its commands.

// ActivityCount is one thing counted in one hour of one session.
type ActivityCount struct {
	Day       string `json:"day"`
	Hour      int    `json:"hour"`
	SessionID string `json:"sessionId"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Count     int64  `json:"count"`
	Failures  int64  `json:"failures"`
	Millis    int64  `json:"millis"`
}

// ActivityRow is a count with what the analytics view filters by.
type ActivityRow struct {
	ActivityCount
	Title       string   `json:"title"`
	ProjectID   string   `json:"projectId"`
	ProjectRoot string   `json:"projectRoot"`
	HarnessID   string   `json:"harnessId"`
	ProviderID  string   `json:"providerId"`
	Tags        []string `json:"tags"`
	Deleted     bool     `json:"deleted"`
}

// StaleSession is a session whose transcript has moved on since it was folded.
type StaleSession struct {
	ID        string
	LastSeq   int64
	CopiedSeq int64
	// ActivitySeq is how far the session's counts go; 0 when it was never folded.
	ActivitySeq int64
}

// ActivityStale lists the sessions to fold again, latest first.
func (s *Store) ActivityStale(ctx context.Context) ([]StaleSession, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, last_seq, copied_seq, activity_seq FROM sessions WHERE activity_seq != last_seq ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: stale activity: %w", err)
	}
	defer rows.Close()
	var out []StaleSession
	for rows.Next() {
		var st StaleSession
		if err := rows.Scan(&st.ID, &st.LastSeq, &st.CopiedSeq, &st.ActivitySeq); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ReplaceActivity sets a session's counts, folded up to seq.
func (s *Store) ReplaceActivity(ctx context.Context, sid string, seq int64, counts []ActivityCount) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM activity WHERE session_id = ?`, sid); err != nil {
		return err
	}
	for _, c := range counts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO activity (day, hour, session_id, kind, name, count, failures, millis) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			c.Day, c.Hour, sid, c.Kind, c.Name, c.Count, c.Failures, c.Millis); err != nil {
			return fmt.Errorf("store: activity: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET activity_seq = ? WHERE id = ?`, seq, sid); err != nil {
		return err
	}
	return tx.Commit()
}

// AddActivity adds counts to a session's, which were folded up to seq now.
func (s *Store) AddActivity(ctx context.Context, sid string, seq int64, counts []ActivityCount) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range counts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO activity (day, hour, session_id, kind, name, count, failures, millis) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (day, hour, session_id, kind, name) DO UPDATE SET count = count + excluded.count, failures = failures + excluded.failures, millis = millis + excluded.millis`,
			c.Day, c.Hour, sid, c.Kind, c.Name, c.Count, c.Failures, c.Millis); err != nil {
			return fmt.Errorf("store: activity: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET activity_seq = ? WHERE id = ?`, seq, sid); err != nil {
		return err
	}
	return tx.Commit()
}

// ActivityRowsSince returns the counts from day since on.
func (s *Store) ActivityRowsSince(ctx context.Context, since string) ([]ActivityRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.day, a.hour, a.session_id, a.kind, a.name, a.count, a.failures, a.millis,
		        coalesce(s.title, ''), coalesce(s.project_id, ''), coalesce(s.project_root, ''), coalesce(s.harness_id, ''), coalesce(s.provider_id, ''), coalesce(s.tags, ''), s.id IS NULL
		 FROM activity a LEFT JOIN sessions s ON s.id = a.session_id
		 WHERE a.day >= ? ORDER BY a.day DESC, a.hour DESC, a.session_id, a.kind, a.name`, since)
	if err != nil {
		return nil, fmt.Errorf("store: activity rows: %w", err)
	}
	defer rows.Close()
	out := []ActivityRow{}
	for rows.Next() {
		var r ActivityRow
		var tags string
		if err := rows.Scan(&r.Day, &r.Hour, &r.SessionID, &r.Kind, &r.Name, &r.Count, &r.Failures, &r.Millis,
			&r.Title, &r.ProjectID, &r.ProjectRoot, &r.HarnessID, &r.ProviderID, &tags, &r.Deleted); err != nil {
			return nil, err
		}
		r.Tags = strings.Fields(tags)
		out = append(out, r)
	}
	return out, rows.Err()
}
