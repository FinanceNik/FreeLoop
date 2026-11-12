package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// Full-text search across sessions (SES-007) and token usage per day
// (PRV-008). Every session's messages, thoughts and tool calls go into one
// SQLite FTS5 table as they are appended: streamed chunks are joined into
// whole messages first, so a phrase split across chunks is found. Model
// calls' tokens are added up per day, session and model in the usage table.
// sessions.indexed_seq records how far a session's transcript is in both;
// transcripts written before they existed, or whose last appends missed
// them, are caught up from the JSONL file, which stays the source of truth.

// maxIndexed caps the text one row indexes; a longer message or output is
// indexed from its start.
const maxIndexed = 32 << 10

// Hit kinds.
const (
	HitUser    = "user"
	HitAgent   = "agent"
	HitThought = "thought"
	HitTool    = "tool"
	HitSummary = "summary"
)

// Hit is one search result: the event where the matching text starts, with
// a snippet whose matches are wrapped in SnippetOpen and SnippetClose.
type Hit struct {
	SessionID string  `json:"sessionId"`
	Seq       int64   `json:"seq"`
	Kind      string  `json:"kind"`
	Snippet   string  `json:"snippet"`
	Rank      float64 `json:"rank"`
}

// Snippet markers: control characters that cannot come from the query, so
// the UI can mark matches without parsing markup.
const (
	SnippetOpen  = "\x02"
	SnippetClose = "\x03"
)

// row is one indexed text.
type row struct {
	seq  int64
	kind string
	body string
}

// usage is one model call's tokens, on the day of the call.
type usage struct {
	day, providerID, model                                    string
	input, output, cacheRead, cacheWrite, cacheWrite1h, calls int64
	turnTotals, unreported                                    int64
}

// batch is what events add to the index.
type batch struct {
	rows   []row
	usage  []usage
	limits []limitRow
}

func (b *batch) add(o batch) {
	b.rows = append(b.rows, o.rows...)
	b.usage = append(b.usage, o.usage...)
	b.limits = append(b.limits, o.limits...)
}

func (b batch) empty() bool { return len(b.rows) == 0 && len(b.usage) == 0 && len(b.limits) == 0 }

// indexer turns a session's events into rows. Chunks of one kind collect
// until an event of another type ends the message.
type indexer struct {
	loaded bool
	seen   int64 // last seq fed
	kind   string
	start  int64 // seq of the pending message's first chunk
	buf    strings.Builder
	// calls maps tool call IDs to the seq of their tool_call, where a
	// result is reported, so a hit points at the call in the conversation.
	calls map[string]int64
	// The session's model, for usage events that do not name theirs.
	providerID, model string
	// copied is the last seq copied from another session, whose usage
	// was counted there.
	copied int64
}

func chunkKind(t event.Type) string {
	switch t {
	case event.UserMessageChunk:
		return HitUser
	case event.AgentMessageChunk:
		return HitAgent
	case event.AgentThoughtChunk:
		return HitThought
	case event.CompactionSummaryChunk:
		return HitSummary
	}
	return ""
}

// feed takes the next event and returns what it completes, and the seq up
// to which every event is in that or earlier batches.
func (x *indexer) feed(ev event.Event) (batch, int64) {
	x.seen = ev.Seq
	var b batch
	switch ev.Type {
	case event.LimitUpdate:
		if ev.Seq > x.copied {
			b.limits = limitRows(ev)
		}
	case event.ModelChanged:
		var d event.ModelChangedData
		json.Unmarshal(ev.Data, &d)
		if d.Model != "" {
			x.providerID, x.model = cmp.Or(d.ProviderID, x.providerID), d.Model
		}
	case event.UsageUpdate:
		var d event.UsageData
		json.Unmarshal(ev.Data, &d)
		day := ev.TS.In(time.Local).Format(time.DateOnly)
		switch {
		case ev.Seq <= x.copied:
		case d.Measured == event.MeasuredNone:
			// A turn the agent reported nothing for, so a total can say it is missing.
			b.usage = append(b.usage, usage{day: day, providerID: x.providerID, unreported: 1})
		case d.InputTokens+d.OutputTokens+d.CacheReadTokens+d.CacheWriteTokens > 0:
			u := usage{day: day,
				providerID: cmp.Or(d.ProviderID, x.providerID), model: pricedModel(cmp.Or(d.Model, x.model), d),
				input: d.InputTokens, output: d.OutputTokens, cacheRead: d.CacheReadTokens, cacheWrite: d.CacheWriteTokens, cacheWrite1h: d.CacheWrite1hTokens, calls: callCount(d)}
			if d.Measured == event.MeasuredTurn {
				u.turnTotals = 1
			}
			b.usage = append(b.usage, u)
		}
	}
	rows := b.rows
	kind := chunkKind(ev.Type)
	if x.kind != "" && kind != x.kind {
		rows = append(rows, row{seq: x.start, kind: x.kind, body: x.buf.String()})
		x.kind = ""
		x.buf.Reset()
	}
	if kind != "" {
		var d event.MessageChunk
		json.Unmarshal(ev.Data, &d)
		if x.kind == "" {
			x.kind, x.start = kind, ev.Seq
		}
		if x.buf.Len() < maxIndexed {
			x.buf.WriteString(d.Content.Text)
		}
		b.rows = rows
		return b, x.start - 1
	}
	if ev.Type == event.CompactionUpdate {
		var d event.CompactionData
		json.Unmarshal(ev.Data, &d)
		if d.Status == "completed" && strings.TrimSpace(d.Summary) != "" {
			rows = append(rows, row{seq: ev.Seq, kind: HitSummary, body: capIndexed(d.Summary)})
		}
	} else if body, id := eventText(ev); strings.TrimSpace(body) != "" || id != "" {
		seq := ev.Seq
		if ev.Type == event.ToolCall {
			if x.calls == nil {
				x.calls = map[string]int64{}
			}
			x.calls[id] = ev.Seq
		} else if at, ok := x.calls[id]; ok {
			seq = at
		}
		if strings.TrimSpace(body) != "" {
			rows = append(rows, row{seq: seq, kind: HitTool, body: body})
		}
	}
	b.rows = rows
	return b, ev.Seq
}

// capIndexed cuts s to maxIndexed bytes at a character boundary, so a
// snippet never ends in half a character.
func capIndexed(s string) string {
	if len(s) <= maxIndexed {
		return s
	}
	n := maxIndexed
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// eventText is the searchable text of a tool call (its title, command and
// paths) or a tool call update (its output and diffs), and the call's ID.
func eventText(ev event.Event) (string, string) {
	var d struct {
		ToolCallID string `json:"toolCallId"`
		Title      string `json:"title"`
		RawInput   struct {
			Command string `json:"command"`
			Path    string `json:"path"`
			Pattern string `json:"pattern"`
			Query   string `json:"query"`
		} `json:"rawInput"`
		Content []struct {
			Type    string             `json:"type"`
			Content event.ContentBlock `json:"content"`
			Path    string             `json:"path"`
			NewText string             `json:"newText"`
		} `json:"content"`
	}
	var parts []string
	switch ev.Type {
	case event.ToolCall:
		json.Unmarshal(ev.Data, &d)
		in := d.RawInput
		parts = append(parts, d.Title, in.Command, in.Path, in.Pattern, in.Query)
	case event.ToolCallUpdate:
		json.Unmarshal(ev.Data, &d)
		for _, c := range d.Content {
			switch c.Type {
			case "content":
				parts = append(parts, c.Content.Text)
			case "diff":
				parts = append(parts, c.Path, c.NewText)
			}
		}
	default:
		return "", ""
	}
	return capIndexed(strings.Join(parts, "\n")), d.ToolCallID
}

// index feeds an appended event to the session's indexer and writes the
// rows it completes, with the session's new search_seq, in one transaction.
// sets and args update the session row in the same transaction (they may
// be empty). Callers hold t.mu.
func (s *Store) index(ctx context.Context, t *transcript, sid string, ev event.Event, sets string, args []any) {
	ctx = context.WithoutCancel(ctx)
	if !t.search.loaded {
		if err := s.catchUp(ctx, t, sid); err != nil {
			log.Printf("store: search index %s: %v", sid, err)
		}
	}
	var b batch
	done := int64(-1)
	if t.search.loaded && t.search.seen == ev.Seq-1 {
		b, done = t.search.feed(ev)
	}
	if b.empty() && (done < 0 || chunk(ev.Type)) && sets == "" {
		return
	}
	if err := s.writeIndex(ctx, sid, b, done, sets, args); err != nil {
		// The indexer moved past events whose rows were not written; the
		// next append catches up from indexed_seq, which did not move.
		t.search.loaded = false
		log.Printf("store: search index %s: %v", sid, err)
	}
}

// addUsage adds one call's tokens to the day's row for its session and model.
func addUsage(ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, sid string, u usage) error {
	_, err := db.ExecContext(ctx, `INSERT INTO usage (day, session_id, provider_id, model, input, output, cache_read, cache_write, cache_write_1h, calls, turn_totals, unreported)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (day, session_id, provider_id, model) DO UPDATE SET
		input = input + excluded.input, output = output + excluded.output, cache_read = cache_read + excluded.cache_read,
		cache_write = cache_write + excluded.cache_write, cache_write_1h = cache_write_1h + excluded.cache_write_1h, calls = calls + excluded.calls,
		turn_totals = turn_totals + excluded.turn_totals, unreported = unreported + excluded.unreported`,
		u.day, sid, u.providerID, u.model, u.input, u.output, u.cacheRead, u.cacheWrite, u.cacheWrite1h, u.calls, u.turnTotals, u.unreported)
	return err
}

// AddSessionlessUsage books a model call that belongs to no session (a skill's test)
// to the day it was made, under no session, so the day's totals include it (PRV-008).
func (s *Store) AddSessionlessUsage(ctx context.Context, at time.Time, d event.UsageData) error {
	if d.InputTokens+d.OutputTokens+d.CacheReadTokens+d.CacheWriteTokens <= 0 {
		return nil
	}
	return addUsage(ctx, s.db, "", usage{day: at.In(time.Local).Format(time.DateOnly),
		providerID: d.ProviderID, model: pricedModel(d.Model, d),
		input: d.InputTokens, output: d.OutputTokens, cacheRead: d.CacheReadTokens, cacheWrite: d.CacheWriteTokens,
		cacheWrite1h: d.CacheWrite1hTokens, calls: callCount(d)})
}

func (s *Store) writeIndex(ctx context.Context, sid string, b batch, done int64, sets string, args []any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, u := range b.usage {
		if err := addUsage(ctx, tx, sid, u); err != nil {
			return err
		}
	}
	for _, l := range b.limits {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO limit_reports (session_id, seq, ts, source, window_id, label, used, resets_at, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, sid, l.seq, l.ts, l.source, l.window, l.label, l.used, l.resetsAt, l.status); err != nil {
			return err
		}
	}
	for _, r := range b.rows {
		if strings.TrimSpace(r.body) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO search (body, session_id, seq, kind) VALUES (?, ?, ?, ?)`, r.body, sid, r.seq, r.kind); err != nil {
			return err
		}
	}
	if done >= 0 {
		if sets != "" {
			sets += ", "
		}
		sets += "indexed_seq = ?"
		args = append(args, done)
	}
	if sets != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET `+sets+` WHERE id = ?`, append(args, sid)...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// catchUp indexes the part of a session's transcript the index is missing
// and leaves the indexer ready for the next event. Callers hold t.mu.
func (s *Store) catchUp(ctx context.Context, t *transcript, sid string) error {
	// The indexer is built in place: it holds a strings.Builder, which
	// must not be copied once written to.
	t.search = indexer{}
	x := &t.search
	var from int64
	if err := s.db.QueryRowContext(ctx, `SELECT indexed_seq, provider_id, model, copied_seq FROM sessions WHERE id = ?`, sid).Scan(&from, &x.providerID, &x.model, &x.copied); err != nil {
		return err
	}
	x.seen = from
	if from < t.lastSeq {
		events, err := readEvents(t.path, from)
		if err != nil {
			return err
		}
		var b batch
		done := from
		for _, ev := range events {
			r, d := x.feed(ev)
			b.add(r)
			done = d
		}
		// An update that failed left indexed_seq behind what it was to record.
		sets, args := replaySets(events)
		if err := s.writeIndex(ctx, sid, b, done, sets, args); err != nil {
			return err
		}
	}
	x.loaded = true
	return nil
}

// IndexAll catches up every session whose transcript is ahead of the index
// (written before the index existed, or by a run that quit in the middle of
// a turn), most recently used first. It runs once after Open, in the
// background. Sessions that are not open are caught up straight from their
// files, without opening them in the store or holding its lock, so live
// sessions go on streaming meanwhile.
func (s *Store) IndexAll(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sessions
		WHERE indexed_seq < last_seq OR status IN ('running', 'queued', 'waiting_permission', 'waiting_input')
		ORDER BY updated_at DESC`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.mu.Lock()
		// Being opened, or read by an earlier pass: that finishes first.
		for s.indexing[id] != nil {
			ch := s.indexing[id]
			s.mu.Unlock()
			select {
			case <-ch:
			case <-ctx.Done():
				return ctx.Err()
			}
			s.mu.Lock()
		}
		if t, open := s.files[id]; open {
			s.mu.Unlock()
			t.mu.Lock()
			if !t.search.loaded {
				err = s.catchUp(ctx, t, id)
			}
			t.mu.Unlock()
		} else {
			ch := make(chan struct{})
			s.indexing[id] = ch
			s.mu.Unlock()
			err = s.catchUpFile(ctx, id)
			s.mu.Lock()
			delete(s.indexing, id)
			close(ch)
			s.mu.Unlock()
		}
		if err != nil {
			log.Printf("store: search index %s: %v", id, err)
		}
	}
	return nil
}

// catchUpFile indexes the rest of a session's transcript that is not open.
// Nothing streams into it, so a message it ends in the middle of (the app
// quit mid-reply) is complete and indexed too.
func (s *Store) catchUpFile(ctx context.Context, sid string) error {
	var x indexer
	var from int64
	if err := s.db.QueryRowContext(ctx, `SELECT indexed_seq, provider_id, model, copied_seq FROM sessions WHERE id = ?`, sid).Scan(&from, &x.providerID, &x.model, &x.copied); err != nil {
		return err
	}
	events, err := readEvents(filepath.Join(s.sessionDir(sid), "events.jsonl"), from)
	if err != nil || len(events) == 0 {
		return err
	}
	x.seen = from
	var b batch
	done := from
	for _, ev := range events {
		r, d := x.feed(ev)
		b.add(r)
		done = d
	}
	if x.kind != "" {
		b.rows = append(b.rows, row{seq: x.start, kind: x.kind, body: x.buf.String()})
		done = x.seen
	}
	sets, args := replaySets(events)
	return s.writeIndex(ctx, sid, b, done, sets, args)
}

// Search finds sessions' text matching query, best first, at most
// perSession hits of each session (0 for no cap), so one long session does
// not crowd out the others. Words match anywhere and in any order, the last
// one also as a prefix; a "quoted phrase" matches as a phrase.
func (s *Store) Search(ctx context.Context, query string, limit, perSession int) ([]Hit, error) {
	match := matchExpr(query)
	if match == "" {
		return []Hit{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if perSession <= 0 {
		perSession = limit
	}
	// One pass over the best rows, snippets built in the same scan (much
	// cheaper than looking rows up again), keeping perSession of each.
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, seq, kind, snippet(search, 0, ?, ?, '…', 16), rank
		 FROM search WHERE search MATCH ? ORDER BY rank LIMIT ?`,
		SnippetOpen, SnippetClose, match, searchScan)
	if err != nil {
		return nil, fmt.Errorf("store: search: %w", err)
	}
	defer rows.Close()
	hits := []Hit{}
	per := map[string]int{}
	for rows.Next() && len(hits) < limit {
		var h Hit
		if err := rows.Scan(&h.SessionID, &h.Seq, &h.Kind, &h.Snippet, &h.Rank); err != nil {
			return nil, err
		}
		if per[h.SessionID] < perSession {
			per[h.SessionID]++
			hits = append(hits, h)
		}
	}
	return hits, rows.Err()
}

// searchScan bounds how many ranked rows a search looks through to fill its
// per-session caps.
const searchScan = 1000

// matchExpr turns what the user typed into an FTS5 query: every word and
// quoted phrase becomes a quoted string, so FTS5's operators and syntax
// characters in the input are plain text.
func matchExpr(q string) string {
	var terms []string
	var phrase bool
	for i, part := range strings.Split(q, `"`) {
		phrase = i%2 == 1
		if phrase {
			if t := tokens(part); t != "" {
				terms = append(terms, `"`+t+`"`)
			}
			continue
		}
		for _, w := range strings.Fields(part) {
			if t := tokens(w); t != "" {
				terms = append(terms, `"`+t+`"`)
			}
		}
	}
	if len(terms) == 0 {
		return ""
	}
	// The last word, still being typed, matches as a prefix.
	if last := strings.TrimRightFunc(q, unicode.IsSpace); !phrase && !strings.HasSuffix(last, `"`) && last == q {
		terms[len(terms)-1] += "*"
	}
	return strings.Join(terms, " ")
}

// tokens keeps the letters and digits of s, the characters the index's
// tokenizer keeps, separated by spaces.
func tokens(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }), " ")
}

// deleteIndex removes a session's rows.
func deleteIndex(ctx context.Context, db *sql.DB, sid string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM search WHERE session_id = ?`, sid)
	return err
}

// Usage is the tokens one model used on one day, across sessions.
type Usage struct {
	Day              string `json:"day"`
	ProviderID       string `json:"providerId"`
	Model            string `json:"model"`
	InputTokens      int64  `json:"inputTokens"`
	OutputTokens     int64  `json:"outputTokens"`
	CacheReadTokens  int64  `json:"cacheReadTokens"`
	CacheWriteTokens int64  `json:"cacheWriteTokens"`
	// CacheWrite1hTokens is the part of CacheWriteTokens that lasts an hour.
	CacheWrite1hTokens int64 `json:"cacheWrite1hTokens"`
	Calls              int64 `json:"calls"`
	// TurnTotals are turns an external agent counted by their total, and Unreported
	// turns it gave no tokens for, which no cost includes (PRV-008).
	TurnTotals int64 `json:"turnTotals"`
	Unreported int64 `json:"unreported"`
}

// pricedModel is the model a usage event counts under. An adjustment or a turn's
// total is a sum of several calls, so its size says nothing about one prompt.
func pricedModel(model string, d event.UsageData) string {
	if !d.OnePrompt() {
		return model
	}
	return config.PricedModel(model, d.InputTokens+d.CacheReadTokens+d.CacheWriteTokens)
}

// callCount is how many model calls a usage event stands for.
func callCount(d event.UsageData) int64 {
	if d.Adjustment {
		return 0
	}
	return 1
}

// UsageSince returns the tokens per day and model from day since
// (YYYY-MM-DD, local time) on, newest day first. A deleted session's usage
// stays: the tokens were spent.
func (s *Store) UsageSince(ctx context.Context, since string) ([]Usage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT day, provider_id, model, sum(input), sum(output), sum(cache_read), sum(cache_write), sum(cache_write_1h), sum(calls), sum(turn_totals), sum(unreported)
		 FROM usage WHERE day >= ? GROUP BY day, provider_id, model ORDER BY day DESC, provider_id, model`, since)
	if err != nil {
		return nil, fmt.Errorf("store: usage: %w", err)
	}
	defer rows.Close()
	out := []Usage{}
	for rows.Next() {
		var u Usage
		if err := rows.Scan(&u.Day, &u.ProviderID, &u.Model, &u.InputTokens, &u.OutputTokens, &u.CacheReadTokens, &u.CacheWriteTokens, &u.CacheWrite1hTokens, &u.Calls, &u.TurnTotals, &u.Unreported); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UsageRow is the tokens one model used in one session on one day, with
// what the analytics view groups by (PRV-008). A deleted session's rows
// keep their tokens and lose the rest.
type UsageRow struct {
	Usage
	SessionID   string   `json:"sessionId"`
	Title       string   `json:"title"`
	ProjectID   string   `json:"projectId"`
	ProjectRoot string   `json:"projectRoot"`
	HarnessID   string   `json:"harnessId"`
	ProfileID   string   `json:"profileId"`
	ParentID    string   `json:"parentId"`
	Tags        []string `json:"tags"`
	Deleted     bool     `json:"deleted"`
}

// UsageRowsSince returns the usage per day, session and model from day
// since (YYYY-MM-DD, local time) on, newest day first.
func (s *Store) UsageRowsSince(ctx context.Context, since string) ([]UsageRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.day, u.provider_id, u.model, sum(u.input), sum(u.output), sum(u.cache_read), sum(u.cache_write), sum(u.cache_write_1h), sum(u.calls),
		        sum(u.turn_totals), sum(u.unreported),
		        u.session_id, coalesce(s.title, ''), coalesce(s.project_id, ''), coalesce(s.project_root, ''),
		        coalesce(s.harness_id, ''), coalesce(s.profile_id, ''), coalesce(s.parent_id, ''), coalesce(s.tags, ''), (s.id IS NULL AND u.session_id <> '')
		 FROM usage u LEFT JOIN sessions s ON s.id = u.session_id
		 WHERE u.day >= ?
		 GROUP BY u.day, u.session_id, u.provider_id, u.model
		 ORDER BY u.day DESC, u.session_id, u.provider_id, u.model`, since)
	if err != nil {
		return nil, fmt.Errorf("store: usage rows: %w", err)
	}
	defer rows.Close()
	out := []UsageRow{}
	for rows.Next() {
		var r UsageRow
		var tags string
		if err := rows.Scan(&r.Day, &r.ProviderID, &r.Model, &r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens, &r.CacheWrite1hTokens, &r.Calls,
			&r.TurnTotals, &r.Unreported,
			&r.SessionID, &r.Title, &r.ProjectID, &r.ProjectRoot, &r.HarnessID, &r.ProfileID, &r.ParentID, &tags, &r.Deleted); err != nil {
			return nil, err
		}
		r.Tags = strings.Fields(tags)
		out = append(out, r)
	}
	return out, rows.Err()
}
