package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"

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
	day, providerID, model                      string
	input, output, cacheRead, cacheWrite, calls int64
}

// batch is what events add to the index.
type batch struct {
	rows  []row
	usage []usage
}

func (b *batch) add(o batch) {
	b.rows = append(b.rows, o.rows...)
	b.usage = append(b.usage, o.usage...)
}

func (b batch) empty() bool { return len(b.rows) == 0 && len(b.usage) == 0 }

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
	case event.ModelChanged:
		var d event.ModelChangedData
		json.Unmarshal(ev.Data, &d)
		if d.Model != "" {
			x.providerID, x.model = cmp.Or(d.ProviderID, x.providerID), d.Model
		}
	case event.UsageUpdate:
		var d event.UsageData
		json.Unmarshal(ev.Data, &d)
		if d.InputTokens+d.OutputTokens+d.CacheReadTokens+d.CacheWriteTokens > 0 {
			b.usage = append(b.usage, usage{day: ev.TS.In(time.Local).Format(time.DateOnly),
				providerID: cmp.Or(d.ProviderID, x.providerID), model: cmp.Or(d.Model, x.model),
				input: d.InputTokens, output: d.OutputTokens, cacheRead: d.CacheReadTokens, cacheWrite: d.CacheWriteTokens, calls: 1})
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
	if body, id := eventText(ev); strings.TrimSpace(body) != "" || id != "" {
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
	s := strings.Join(parts, "\n")
	if len(s) > maxIndexed {
		s = s[:maxIndexed]
	}
	return s, d.ToolCallID
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
		log.Printf("store: search index %s: %v", sid, err)
	}
}

func (s *Store) writeIndex(ctx context.Context, sid string, b batch, done int64, sets string, args []any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, u := range b.usage {
		if _, err := tx.ExecContext(ctx, `INSERT INTO usage (day, session_id, provider_id, model, input, output, cache_read, cache_write, calls)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (day, session_id, provider_id, model) DO UPDATE SET
			input = input + excluded.input, output = output + excluded.output, cache_read = cache_read + excluded.cache_read,
			cache_write = cache_write + excluded.cache_write, calls = calls + excluded.calls`,
			u.day, sid, u.providerID, u.model, u.input, u.output, u.cacheRead, u.cacheWrite, u.calls); err != nil {
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
	var from int64
	x := indexer{loaded: true}
	if err := s.db.QueryRowContext(ctx, `SELECT indexed_seq, provider_id, model FROM sessions WHERE id = ?`, sid).Scan(&from, &x.providerID, &x.model); err != nil {
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
		if err := s.writeIndex(ctx, sid, b, done, "", nil); err != nil {
			return err
		}
	}
	t.search = x
	return nil
}

// IndexAll catches up every session whose transcript is ahead of the index,
// for transcripts written before the index existed. It runs once after
// Open, in the background.
func (s *Store) IndexAll(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sessions WHERE indexed_seq < last_seq`)
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
		t, err := s.transcript(ctx, id)
		if err != nil {
			continue
		}
		t.mu.Lock()
		if !t.search.loaded {
			err = s.catchUp(ctx, t, id)
		}
		t.mu.Unlock()
		if err != nil {
			log.Printf("store: search index %s: %v", id, err)
		}
	}
	return nil
}

// Search finds sessions' text matching query, best first. Words match
// anywhere and in any order, the last one also as a prefix; a "quoted
// phrase" matches as a phrase.
func (s *Store) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	match := matchExpr(query)
	if match == "" {
		return []Hit{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, seq, kind, snippet(search, 0, ?, ?, '…', 16), bm25(search)
		 FROM search WHERE search MATCH ? ORDER BY bm25(search) LIMIT ?`,
		SnippetOpen, SnippetClose, match, limit)
	if err != nil {
		return nil, fmt.Errorf("store: search: %w", err)
	}
	defer rows.Close()
	hits := []Hit{}
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.SessionID, &h.Seq, &h.Kind, &h.Snippet, &h.Rank); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

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
	Calls            int64  `json:"calls"`
}

// UsageSince returns the tokens per day and model from day since
// (YYYY-MM-DD, local time) on, newest day first. A deleted session's usage
// stays: the tokens were spent.
func (s *Store) UsageSince(ctx context.Context, since string) ([]Usage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT day, provider_id, model, sum(input), sum(output), sum(cache_read), sum(cache_write), sum(calls)
		 FROM usage WHERE day >= ? GROUP BY day, provider_id, model ORDER BY day DESC, provider_id, model`, since)
	if err != nil {
		return nil, fmt.Errorf("store: usage: %w", err)
	}
	defer rows.Close()
	out := []Usage{}
	for rows.Next() {
		var u Usage
		if err := rows.Scan(&u.Day, &u.ProviderID, &u.Model, &u.InputTokens, &u.OutputTokens, &u.CacheReadTokens, &u.CacheWriteTokens, &u.Calls); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
