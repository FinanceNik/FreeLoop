package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// What a unit of work costs (GUI-051): the commits sessions made, the usage between
// them, and usage per five minutes kept for good, for comparing over months. Like
// the activity table it is folded from the transcripts when asked, a session again
// whole whenever it moved on, so the rows always follow from the transcripts.
//
// These tables are made outside the migrations, as limit_reports is: an older build
// ignores them and still opens the database, and none prunes them.

const workTables = `
CREATE TABLE IF NOT EXISTS work_commits (
	sha          TEXT PRIMARY KEY,
	root_id      TEXT NOT NULL,
	session_id   TEXT NOT NULL,
	project_id   TEXT NOT NULL,
	unit_seq     INTEGER NOT NULL,
	unit_ts      INTEGER NOT NULL,
	committed_at INTEGER NOT NULL,
	added        INTEGER NOT NULL,
	deleted      INTEGER NOT NULL,
	files        INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS work_commits_unit ON work_commits (unit_ts);
CREATE INDEX IF NOT EXISTS work_commits_root ON work_commits (root_id, unit_ts);
CREATE TABLE IF NOT EXISTS work_usage (
	session_id     TEXT NOT NULL,
	root_id        TEXT NOT NULL,
	project_id     TEXT NOT NULL,
	harness_id     TEXT NOT NULL,
	end_seq        INTEGER NOT NULL,
	ts             INTEGER NOT NULL,
	provider_id    TEXT NOT NULL,
	model          TEXT NOT NULL,
	input          INTEGER NOT NULL,
	output         INTEGER NOT NULL,
	cache_read     INTEGER NOT NULL,
	cache_write    INTEGER NOT NULL,
	cache_write_1h INTEGER NOT NULL,
	calls          INTEGER NOT NULL,
	PRIMARY KEY (session_id, end_seq, provider_id, model)
);
CREATE INDEX IF NOT EXISTS work_usage_root ON work_usage (root_id, ts);
CREATE INDEX IF NOT EXISTS work_usage_ts ON work_usage (ts);
CREATE TABLE IF NOT EXISTS usage_buckets (
	bucket         INTEGER NOT NULL,
	session_id     TEXT NOT NULL,
	harness_id     TEXT NOT NULL,
	provider_id    TEXT NOT NULL,
	model          TEXT NOT NULL,
	input          INTEGER NOT NULL,
	output         INTEGER NOT NULL,
	cache_read     INTEGER NOT NULL,
	cache_write    INTEGER NOT NULL,
	cache_write_1h INTEGER NOT NULL,
	calls          INTEGER NOT NULL,
	PRIMARY KEY (bucket, session_id, provider_id, model)
);
CREATE TABLE IF NOT EXISTS work_resolved (
	session_id TEXT NOT NULL,
	seq        INTEGER NOT NULL,
	commits    TEXT NOT NULL,
	PRIMARY KEY (session_id, seq)
);
CREATE TABLE IF NOT EXISTS work_meta (
	key   TEXT PRIMARY KEY,
	value INTEGER NOT NULL
);`

func addWorkTables(db *sql.DB) error {
	if _, err := db.Exec(workTables); err != nil {
		return fmt.Errorf("store: work tables: %w", err)
	}
	return nil
}

// markWorkSince records when this build first opened the database: transcripts
// written before list no commits in their git states, so those are found with git
// when folded.
func (s *Store) markWorkSince() error {
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO work_meta (key, value) VALUES ('since', ?)`, s.now().UnixMilli()); err != nil {
		return fmt.Errorf("store: work tables: %w", err)
	}
	return nil
}

// CommitResolver finds the commits a checkout's HEAD moved forward over in a
// transcript from before commits were listed. ok is false when git could not say,
// as for a repository since removed; that move is not counted, and asked again.
type CommitResolver func(ctx context.Context, sessionID, old, now string) (commits []event.CommitInfo, ok bool)

// commitCommand is a shell command that makes commits.
var commitCommand = regexp.MustCompile(`\bgit\b[^\n|;&]*?\s(commit|merge|cherry-pick|revert|am)\b`)

type workSession struct {
	id, root, project, harness, provider, model string
	last, copied                                int64
}

type workUsageRow struct {
	endSeq, ts                                                int64
	provider, model                                           string
	input, output, cacheRead, cacheWrite, cacheWrite1h, calls int64
}

type workCommitRow struct {
	info            event.CommitInfo
	attributed      bool
	unitSeq, unitTS int64
}

type bucketKey struct {
	bucket          int64
	provider, model string
}

// FoldWork folds the sessions that moved on until deadline, and returns how many it
// left for the next call.
func (s *Store) FoldWork(ctx context.Context, deadline time.Time, resolve CommitResolver) (int, error) {
	s.workMu.Lock()
	defer s.workMu.Unlock()
	var since int64
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM work_meta WHERE key = 'since'`).Scan(&since); err != nil {
		return 0, fmt.Errorf("store: work: %w", err)
	}
	parents := map[string]string{}
	rows, err := s.db.QueryContext(ctx, `SELECT id, parent_id FROM sessions`)
	if err != nil {
		return 0, fmt.Errorf("store: work: %w", err)
	}
	for rows.Next() {
		var id, parent string
		if err := rows.Scan(&id, &parent); err != nil {
			rows.Close()
			return 0, fmt.Errorf("store: work: %w", err)
		}
		parents[id] = parent
	}
	rows.Close()
	root := func(id string) string {
		for i := 0; i < 32 && parents[id] != ""; i++ {
			id = parents[id]
		}
		return id
	}
	rows, err = s.db.QueryContext(ctx, `SELECT id, project_id, harness_id, provider_id, model, last_seq, copied_seq FROM sessions WHERE work_seq != last_seq ORDER BY updated_at DESC`)
	if err != nil {
		return 0, fmt.Errorf("store: work: %w", err)
	}
	var stale []workSession
	for rows.Next() {
		var w workSession
		if err := rows.Scan(&w.id, &w.project, &w.harness, &w.provider, &w.model, &w.last, &w.copied); err != nil {
			rows.Close()
			return 0, fmt.Errorf("store: work: %w", err)
		}
		w.root = root(w.id)
		stale = append(stale, w)
	}
	rows.Close()
	for i, w := range stale {
		if !time.Now().Before(deadline) {
			return len(stale) - i, nil
		}
		if err := s.foldWorkSession(ctx, w, since, resolve); err != nil {
			return 0, err
		}
	}
	return 0, nil
}

func (s *Store) foldWorkSession(ctx context.Context, w workSession, since int64, resolve CommitResolver) error {
	events, err := s.Events(ctx, w.id, 0)
	if err != nil {
		return nil // deleted meanwhile
	}
	resolved := map[int64][]event.CommitInfo{}
	rows, err := s.db.QueryContext(ctx, `SELECT seq, commits FROM work_resolved WHERE session_id = ?`, w.id)
	if err != nil {
		return fmt.Errorf("store: work: %w", err)
	}
	for rows.Next() {
		var seq int64
		var text string
		if rows.Scan(&seq, &text) == nil {
			var c []event.CommitInfo
			if json.Unmarshal([]byte(text), &c) == nil {
				resolved[seq] = c
			}
		}
	}
	rows.Close()

	var (
		usage    []workUsageRow
		commits  []workCommitRow
		buckets  = map[bucketKey]*workUsageRow{}
		turn     = map[[2]string]*workUsageRow{}
		pending  []event.CommitInfo
		lastHead string
		asked    bool
		fresh    = map[int64][]event.CommitInfo{}
	)
	flushTurn := func(seq, ts int64) {
		keys := make([][2]string, 0, len(turn))
		for k := range turn {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i][0]+keys[i][1] < keys[j][0]+keys[j][1] })
		for _, k := range keys {
			r := turn[k]
			r.endSeq, r.ts = seq, ts
			usage = append(usage, *r)
		}
		turn = map[[2]string]*workUsageRow{}
	}
	unit := func(seq, ts int64) {
		for _, c := range pending {
			commits = append(commits, workCommitRow{info: c, attributed: true, unitSeq: seq, unitTS: ts})
		}
		pending = nil
	}
	// The model in force: the session's now, unless it changed. Before its first
	// change that is not recorded, so it is the one the first call named, and
	// otherwise none, which prices nothing rather than at the wrong model.
	inForce, inForceProvider := w.model, w.provider
	for _, ev := range events {
		if ev.Type == event.ModelChanged {
			inForce, inForceProvider = "", ""
			for _, e2 := range events {
				if e2.Type == event.ModelChanged {
					break
				}
				if e2.Type == event.UsageUpdate {
					var d event.UsageData
					if json.Unmarshal(e2.Data, &d) == nil && d.Model != "" {
						inForce, inForceProvider = d.Model, d.ProviderID
						break
					}
				}
			}
			break
		}
	}
	for _, ev := range events {
		if ev.Type == event.ModelChanged {
			var d event.ModelChangedData
			json.Unmarshal(ev.Data, &d)
			if d.Model != "" {
				inForce, inForceProvider = d.Model, cmp.Or(d.ProviderID, inForceProvider)
			}
		}
		if ev.Seq <= w.copied {
			continue
		}
		switch ev.Type {
		case event.ToolCall, event.ToolCallUpdate:
			var d struct {
				RawInput struct {
					Command string `json:"command"`
				} `json:"rawInput"`
			}
			if json.Unmarshal(ev.Data, &d) == nil && commitCommand.MatchString(d.RawInput.Command) {
				asked = true
			}
		case event.UsageUpdate:
			var d event.UsageData
			json.Unmarshal(ev.Data, &d)
			if d.InputTokens+d.OutputTokens+d.CacheReadTokens+d.CacheWriteTokens == 0 {
				continue
			}
			provider, model := d.ProviderID, d.Model
			if provider == "" {
				provider = inForceProvider
			}
			if model == "" {
				model = inForce
			}
			model = pricedModel(model, d)
			add := func(r *workUsageRow) {
				r.input += d.InputTokens
				r.output += d.OutputTokens
				r.cacheRead += d.CacheReadTokens
				r.cacheWrite += d.CacheWriteTokens
				r.cacheWrite1h += d.CacheWrite1hTokens
				r.calls += callCount(d)
			}
			k := [2]string{provider, model}
			if turn[k] == nil {
				turn[k] = &workUsageRow{provider: provider, model: model}
			}
			add(turn[k])
			bk := bucketKey{ev.TS.Truncate(RecentBucket).Unix(), provider, model}
			if buckets[bk] == nil {
				buckets[bk] = &workUsageRow{provider: provider, model: model}
			}
			add(buckets[bk])
		case event.GitState:
			var d event.GitStateData
			if json.Unmarshal(ev.Data, &d) != nil || !d.Repo {
				continue
			}
			found := d.Commits
			if len(found) == 0 && d.Cause == "" && ev.TS.UnixMilli() < since && lastHead != "" && d.Head != "" && d.Head != lastHead {
				if c, ok := resolved[ev.Seq]; ok {
					found = c
				} else if resolve != nil {
					if c, ok := resolve(ctx, w.id, lastHead, d.Head); ok {
						found = c
						fresh[ev.Seq] = c
					}
				}
			}
			moved := d.Head != "" && lastHead != "" && d.Head != lastHead
			if d.Head != "" {
				lastHead = d.Head
			}
			if len(found) > 0 {
				switch {
				case d.Cause == event.CauseCommit:
					// The app committed between turns: a unit of its own, now.
					pending = append(pending, found...)
					unit(ev.Seq, ev.TS.UnixMilli())
				case asked:
					pending = append(pending, found...)
				default:
					// Another session's commit, or one made outside the app.
					for _, c := range found {
						commits = append(commits, workCommitRow{info: c})
					}
				}
			}
			// A state that only counted changed files, as another tool call
			// finishing first, leaves a commit under way its command.
			if moved {
				asked = false
			}
		case event.TurnEnded:
			flushTurn(ev.Seq, ev.TS.UnixMilli())
			unit(ev.Seq, ev.TS.UnixMilli())
		}
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		flushTurn(last.Seq, last.TS.UnixMilli())
		unit(last.Seq, last.TS.UnixMilli())
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: work: %w", err)
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM work_usage WHERE session_id = ?`, `DELETE FROM usage_buckets WHERE session_id = ?`, `DELETE FROM work_commits WHERE session_id = ?`} {
		if _, err := tx.ExecContext(ctx, q, w.id); err != nil {
			return fmt.Errorf("store: work: %w", err)
		}
	}
	for _, r := range usage {
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_usage (session_id, root_id, project_id, harness_id, end_seq, ts, provider_id, model, input, output, cache_read, cache_write, cache_write_1h, calls)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			w.id, w.root, w.project, w.harness, r.endSeq, r.ts, r.provider, r.model, r.input, r.output, r.cacheRead, r.cacheWrite, r.cacheWrite1h, r.calls); err != nil {
			return fmt.Errorf("store: work: %w", err)
		}
	}
	for k, r := range buckets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO usage_buckets (bucket, session_id, harness_id, provider_id, model, input, output, cache_read, cache_write, cache_write_1h, calls)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			k.bucket, w.id, w.harness, k.provider, k.model, r.input, r.output, r.cacheRead, r.cacheWrite, r.cacheWrite1h, r.calls); err != nil {
			return fmt.Errorf("store: work: %w", err)
		}
	}
	for _, c := range commits {
		rootID, unitSeq, unitTS := "", int64(0), int64(0)
		if c.attributed {
			rootID, unitSeq, unitTS = w.root, c.unitSeq, c.unitTS
		}
		// A commit counts once: the session that made it takes it from one that
		// only saw it.
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_commits (sha, root_id, session_id, project_id, unit_seq, unit_ts, committed_at, added, deleted, files)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (sha) DO UPDATE SET root_id = excluded.root_id, session_id = excluded.session_id, project_id = excluded.project_id,
				unit_seq = excluded.unit_seq, unit_ts = excluded.unit_ts, added = excluded.added, deleted = excluded.deleted, files = excluded.files
			WHERE work_commits.root_id = '' AND excluded.root_id != ''`,
			c.info.SHA, rootID, w.id, w.project, unitSeq, unitTS, c.info.At, c.info.Added, c.info.Deleted, c.info.Files); err != nil {
			return fmt.Errorf("store: work: %w", err)
		}
	}
	for seq, c := range fresh {
		b, _ := json.Marshal(c)
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO work_resolved (session_id, seq, commits) VALUES (?, ?, ?)`, w.id, seq, string(b)); err != nil {
			return fmt.Errorf("store: work: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET work_seq = ? WHERE id = ?`, w.last, w.id); err != nil {
		return fmt.Errorf("store: work: %w", err)
	}
	return tx.Commit()
}

// WorkCommit is a commit a session made.
type WorkCommit struct {
	SHA         string    `json:"sha"`
	RootID      string    `json:"rootId"`
	SessionID   string    `json:"sessionId"`
	ProjectID   string    `json:"projectId"`
	UnitSeq     int64     `json:"unitSeq"`
	UnitTS      time.Time `json:"unitTs"`
	CommittedAt time.Time `json:"committedAt"`
	Added       int       `json:"added"`
	Deleted     int       `json:"deleted"`
	Files       int       `json:"files"`
	// Before marks a commit from before the window, kept only to say where the
	// first one in it starts counting.
	Before bool `json:"-"`
}

// WorkUsage is one turn's usage of one model in a session, under the session its
// work counts for (its root, for a subagent's).
type WorkUsage struct {
	SessionID    string
	RootID       string
	ProjectID    string
	HarnessID    string
	EndSeq       int64
	TS           time.Time
	ProviderID   string
	Model        string
	Input        int64
	Output       int64
	CacheRead    int64
	CacheWrite   int64
	CacheWrite1h int64
	Calls        int64
}

// Queries of WorkSince; a test reads their plans, as each is to use an index.
const (
	workCommitsSince = `SELECT sha, root_id, session_id, project_id, unit_seq, unit_ts, committed_at, added, deleted, files FROM work_commits WHERE root_id != '' AND unit_ts >= ? ORDER BY unit_ts, unit_seq`
	workCommitBefore = `SELECT sha, root_id, session_id, project_id, unit_seq, unit_ts, committed_at, added, deleted, files FROM work_commits WHERE root_id = ? AND unit_ts < ? ORDER BY unit_ts DESC, unit_seq DESC LIMIT 1`
	workUnattributed = `SELECT count(*) FROM work_commits WHERE root_id = '' AND committed_at >= ?`
	workUsageSince   = `SELECT session_id, root_id, project_id, harness_id, end_seq, ts, provider_id, model, input, output, cache_read, cache_write, cache_write_1h, calls FROM work_usage WHERE ts >= ? ORDER BY ts, end_seq`
	workUsageOfRoot  = `SELECT session_id, root_id, project_id, harness_id, end_seq, ts, provider_id, model, input, output, cache_read, cache_write, cache_write_1h, calls FROM work_usage WHERE root_id = ? AND ts >= ? AND ts < ? ORDER BY ts, end_seq`
)

func scanWorkCommit(rows *sql.Rows) (WorkCommit, error) {
	var c WorkCommit
	var unit, at int64
	if err := rows.Scan(&c.SHA, &c.RootID, &c.SessionID, &c.ProjectID, &c.UnitSeq, &unit, &at, &c.Added, &c.Deleted, &c.Files); err != nil {
		return c, err
	}
	c.UnitTS, c.CommittedAt = time.UnixMilli(unit), time.Unix(at, 0)
	return c, nil
}

func scanWorkUsage(rows *sql.Rows) ([]WorkUsage, error) {
	var out []WorkUsage
	for rows.Next() {
		var u WorkUsage
		var ts int64
		if err := rows.Scan(&u.SessionID, &u.RootID, &u.ProjectID, &u.HarnessID, &u.EndSeq, &ts, &u.ProviderID, &u.Model, &u.Input, &u.Output, &u.CacheRead, &u.CacheWrite, &u.CacheWrite1h, &u.Calls); err != nil {
			return nil, err
		}
		u.TS = time.UnixMilli(ts)
		out = append(out, u)
	}
	return out, rows.Err()
}

// WorkSince returns the commits attributed from since on, and the usage that
// counts for them: all usage from since on, and for each session a commit in the
// window belongs to, the usage since its commit before the window. That commit is
// returned as well, with Before set, so the first commit in the window takes on
// only what was spent after it.
func (s *Store) WorkSince(ctx context.Context, since time.Time) ([]WorkCommit, []WorkUsage, int, error) {
	fail := func(err error) ([]WorkCommit, []WorkUsage, int, error) {
		return nil, nil, 0, fmt.Errorf("store: work: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, workCommitsSince, since.UnixMilli())
	if err != nil {
		return fail(err)
	}
	var commits []WorkCommit
	var roots []string
	seen := map[string]bool{}
	for rows.Next() {
		c, err := scanWorkCommit(rows)
		if err != nil {
			rows.Close()
			return fail(err)
		}
		commits = append(commits, c)
		if !seen[c.RootID] {
			seen[c.RootID] = true
			roots = append(roots, c.RootID)
		}
	}
	rows.Close()
	var unattributed int
	s.db.QueryRowContext(ctx, workUnattributed, since.Unix()).Scan(&unattributed)
	rows, err = s.db.QueryContext(ctx, workUsageSince, since.UnixMilli())
	if err != nil {
		return fail(err)
	}
	usage, err := scanWorkUsage(rows)
	rows.Close()
	if err != nil {
		return fail(err)
	}
	for _, root := range roots {
		// Usage from the commit before the window; with none, from the start.
		var from int64
		rows, err := s.db.QueryContext(ctx, workCommitBefore, root, since.UnixMilli())
		if err != nil {
			return fail(err)
		}
		if rows.Next() {
			c, err := scanWorkCommit(rows)
			if err != nil {
				rows.Close()
				return fail(err)
			}
			c.Before = true
			from = c.UnitTS.UnixMilli()
			commits = append(commits, c)
		}
		rows.Close()
		rows, err = s.db.QueryContext(ctx, workUsageOfRoot, root, from, since.UnixMilli())
		if err != nil {
			return fail(err)
		}
		more, err := scanWorkUsage(rows)
		rows.Close()
		if err != nil {
			return fail(err)
		}
		usage = append(usage, more...)
	}
	sort.SliceStable(usage, func(i, j int) bool {
		if !usage[i].TS.Equal(usage[j].TS) {
			return usage[i].TS.Before(usage[j].TS)
		}
		return usage[i].EndSeq < usage[j].EndSeq
	})
	sort.SliceStable(commits, func(i, j int) bool {
		if !commits[i].UnitTS.Equal(commits[j].UnitTS) {
			return commits[i].UnitTS.Before(commits[j].UnitTS)
		}
		return commits[i].UnitSeq < commits[j].UnitSeq
	})
	return commits, usage, unattributed, nil
}

// LimitSpan is one run of a limit as reports saw it: the first report, and the
// highest it reached and when.
type LimitSpan struct {
	Source   string
	Window   string
	Label    string
	ResetsAt time.Time
	First    time.Time
	FirstUse float64
	PeakAt   time.Time
	Peak     float64
	Reports  int
	// Harnesses are the agents of the sessions that reported it.
	Harnesses []string
}

// LimitSpans returns the runs of each limit that end from since on. Parallel
// sessions report in any order, so the peak is the running highest.
func (s *Store) LimitSpans(ctx context.Context, since time.Time) ([]LimitSpan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.source, r.window_id, r.label, r.resets_at, r.ts, r.used, COALESCE(p.harness_id, '')
		FROM limit_reports r LEFT JOIN sessions p ON p.id = r.session_id WHERE r.resets_at >= ? ORDER BY r.source, r.window_id, r.resets_at, r.ts`, since.Unix())
	if err != nil {
		return nil, fmt.Errorf("store: limit spans: %w", err)
	}
	defer rows.Close()
	var out []LimitSpan
	var cur *LimitSpan
	seen := map[string]bool{}
	for rows.Next() {
		var source, window, label, harness string
		var resets, ts int64
		var used float64
		if err := rows.Scan(&source, &window, &label, &resets, &ts, &used, &harness); err != nil {
			return nil, fmt.Errorf("store: limit spans: %w", err)
		}
		if cur == nil || cur.Source != source || cur.Window != window || cur.ResetsAt.Unix() != resets {
			out = append(out, LimitSpan{Source: source, Window: window, Label: label, ResetsAt: time.Unix(resets, 0), First: time.UnixMilli(ts), FirstUse: used, PeakAt: time.UnixMilli(ts), Peak: used})
			cur = &out[len(out)-1]
			seen = map[string]bool{}
		}
		cur.Reports++
		if used > cur.Peak {
			cur.Peak, cur.PeakAt = used, time.UnixMilli(ts)
		}
		if harness != "" && !seen[harness] {
			seen[harness] = true
			cur.Harnesses = append(cur.Harnesses, harness)
		}
	}
	return out, rows.Err()
}

// BucketUsage returns the usage per model of sessions of harnesses from from up to
// to, from the five-minute buckets kept for good.
func (s *Store) BucketUsage(ctx context.Context, from, to time.Time, harnesses []string) ([]WorkUsage, error) {
	if len(harnesses) == 0 {
		return nil, nil
	}
	q := `SELECT provider_id, model, SUM(input), SUM(output), SUM(cache_read), SUM(cache_write), SUM(cache_write_1h), SUM(calls) FROM usage_buckets WHERE bucket >= ? AND bucket <= ? AND harness_id IN (?` + repeatComma(len(harnesses)-1) + `) GROUP BY provider_id, model`
	args := []any{from.Truncate(RecentBucket).Unix(), to.Unix()}
	for _, h := range harnesses {
		args = append(args, h)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: bucket usage: %w", err)
	}
	defer rows.Close()
	var out []WorkUsage
	for rows.Next() {
		var u WorkUsage
		if err := rows.Scan(&u.ProviderID, &u.Model, &u.Input, &u.Output, &u.CacheRead, &u.CacheWrite, &u.CacheWrite1h, &u.Calls); err != nil {
			return nil, fmt.Errorf("store: bucket usage: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func repeatComma(n int) string {
	b := make([]byte, 0, 3*n)
	for i := 0; i < n; i++ {
		b = append(b, ", ?"...)
	}
	return string(b)
}
