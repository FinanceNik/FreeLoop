package store

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// A long session to measure with (NFR-PERF-002, issue 040). The fixture is made up,
// the way the user's longest session is shaped: its shape was taken by counting the
// event types and the sizes of lines in one real transcript (types and byte counts
// only, no content), and the text here is words drawn from a list. It is written into
// a test's scratch folder and never kept.
//
// The real session held 81,325 events in 92 MB: 42% streamed message chunks of about
// 225 bytes a line, 25% tool call updates (median 600 bytes, 95th percentile 12 KB,
// the largest 1.3 MB, each carrying its output twice), 15% usage updates of about 260
// bytes, 7% thought chunks, 7% tool calls (median 250 bytes), and about 1% of git
// states, limit updates, checkpoints, turns and permission requests (up to 17 KB).
// The two shapes below are the same session leaning two ways.

// fixtureShape says what a long made-up transcript is mostly made of.
type fixtureShape string

const (
	// shapeText is mostly streamed text: an answer written in many small chunks.
	shapeText fixtureShape = "text"
	// shapeTools is heavy with tool calls and their output, the way an agent
	// working on a repository is.
	shapeTools fixtureShape = "tools"
)

// longEvents is the length of the fixture: the user's longest session, rounded up.
const longEvents = 80000

var fixtureWords = strings.Fields(`the a to of and in that it is was for on with as this be at by from or an are
file function return value error test session agent tool result line check update read write
build run path commit change model request store event index config option name type data
list item first next last before after because still only then when where which would should
could might need make find look see use try fix add remove move open close start stop wait`)

// fixtureText is n bytes of words, about.
func fixtureText(r *rand.Rand, n int) string {
	var b strings.Builder
	for b.Len() < n {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(fixtureWords[r.Intn(len(fixtureWords))])
	}
	return b.String()
}

// fixtureLines writes n events of the shape to w, one line each, seq 1 to n, and
// returns the seq of the last error event in it.
func fixtureLines(w *bufio.Writer, sid string, shape fixtureShape, n int) (lastError int64) {
	r := rand.New(rand.NewSource(7))
	ts := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	var seq int64
	emit := func(typ event.Type, data any) {
		if int(seq) >= n {
			return
		}
		seq++
		ts = ts.Add(180 * time.Millisecond)
		b, err := json.Marshal(data)
		if err != nil {
			panic(err)
		}
		line, err := json.Marshal(event.Event{V: event.Version, Seq: seq, TS: ts, SID: sid, Type: typ, Data: b})
		if err != nil {
			panic(err)
		}
		w.Write(line)
		w.WriteByte('\n')
		if typ == event.Error {
			lastError = seq
		}
	}
	text := func(typ event.Type, size int) {
		emit(typ, map[string]any{"messageId": "msg_made_up", "content": map[string]any{"type": "text", "text": fixtureText(r, size)}})
	}
	usage := func() {
		emit(event.UsageUpdate, map[string]any{"inputTokens": 3000 + r.Intn(60000), "outputTokens": 100 + r.Intn(900), "cacheReadTokens": r.Intn(90000), "cacheWriteTokens": r.Intn(3000)})
	}
	// An output's size: most small, some of a few KB, a few of tens of KB.
	outputSize := func() int {
		switch p := r.Intn(100); {
		case p < 60:
			return 300 + r.Intn(500)
		case p < 96:
			return 1500 + r.Intn(7500)
		default:
			return 15000 + r.Intn(30000)
		}
	}
	call := 0
	tool := func() {
		call++
		id := fmt.Sprintf("toolu_made_up_%06d", call)
		kind := []string{"read", "edit", "execute", "search"}[r.Intn(4)]
		emit(event.ToolCall, map[string]any{"toolCallId": id, "title": "Run " + fixtureText(r, 20), "kind": kind, "tool": "shell", "status": "pending",
			"rawInput": map[string]any{"command": fixtureText(r, 40+r.Intn(200)), "description": fixtureText(r, 40)}})
		if r.Intn(3) == 0 {
			emit(event.ToolCallUpdate, map[string]any{"toolCallId": id, "status": "in_progress"})
		}
		out := fixtureText(r, outputSize())
		emit(event.ToolCallUpdate, map[string]any{"toolCallId": id, "status": "completed",
			"content":   []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": out}}},
			"rawOutput": []any{map[string]any{"type": "text", "text": out}}})
	}
	chunks := func(k int) {
		for i := 0; i < k; i++ {
			text(event.AgentMessageChunk, 55+r.Intn(30))
		}
	}
	for int(seq) < n {
		emit(event.TurnStarted, map[string]any{})
		text(event.UserMessageChunk, 60+r.Intn(250))
		emit(event.Checkpoint, map[string]any{"commit": fmt.Sprintf("%040x", r.Int63()), "historyLen": int(seq) / 40})
		emit(event.SessionStatus, map[string]any{"status": "running"})
		length := 150 + r.Intn(500)
		start := seq
		for int(seq-start) < length && int(seq) < n {
			switch shape {
			case shapeText:
				// Streamed text with a thought now and then, a model call per
				// stretch and a tool call for every few stretches.
				if r.Intn(6) == 0 {
					for i := 0; i < 10+r.Intn(30); i++ {
						text(event.AgentThoughtChunk, 55+r.Intn(30))
					}
				}
				chunks(40 + r.Intn(120))
				usage()
				if r.Intn(4) == 0 {
					tool()
				}
			default:
				// Tool calls one after another, with a line of text between.
				chunks(2 + r.Intn(6))
				tool()
				if r.Intn(3) == 0 {
					tool()
				}
				usage()
			}
		}
		if r.Intn(12) == 0 {
			emit(event.PermissionRequested, map[string]any{"requestId": fmt.Sprintf("perm_%d", seq), "toolCallId": fmt.Sprintf("toolu_made_up_%06d", call), "tool": "shell", "kind": "execute",
				"title": "Run a command", "input": map[string]any{"command": fixtureText(r, 200)}, "command": fixtureText(r, 200),
				"options": []any{map[string]any{"optionId": "allow_once", "name": "Allow once", "kind": "allow_once"}}, "createdAt": ts.Format(time.RFC3339)})
			emit(event.PermissionDecided, map[string]any{"requestId": fmt.Sprintf("perm_%d", seq-1), "outcome": "allow_once", "decidedBy": "user"})
		}
		// A few errors, the last of them well before the end, so that finding it
		// means looking back and not at the last line.
		if int(seq) > n*6/10 && int(seq) < n*7/10 && lastError < int64(n)*6/10 {
			emit(event.Error, map[string]any{"message": "made-up error: " + fixtureText(r, 80)})
		} else if r.Intn(40) == 0 && int(seq) < n*6/10 {
			emit(event.Error, map[string]any{"message": "made-up error: " + fixtureText(r, 80)})
		}
		emit(event.GitState, map[string]any{"repo": true, "branch": "main", "head": fmt.Sprintf("%040x", r.Int63()), "dirty": r.Intn(9), "baseCommit": fmt.Sprintf("%040x", 1)})
		emit(event.TurnEnded, map[string]any{"stopReason": "end_turn"})
		emit(event.SessionStatus, map[string]any{"status": "idle"})
	}
	return lastError
}

// longFixture is a store holding a session of n made-up events.
type longFixture struct {
	s         *Store
	sid       string
	path      string
	n         int
	lastError int64
}

// newLongFixture writes a made-up transcript of n events of the shape for a new
// session into a store in dir, which a test gives as its scratch folder. The file is
// put there before the store first opens the session, as a file the app left is.
func newLongFixture(tb testing.TB, dir string, shape fixtureShape, n int) *longFixture {
	tb.Helper()
	s, err := Open(Options{Dir: dir, SyncInterval: time.Hour})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { s.Close() })
	ses, err := s.CreateSession(context.Background(), session.Session{Title: "made-up " + string(shape), HarnessID: "builtin"})
	if err != nil {
		tb.Fatal(err)
	}
	path := filepath.Join(s.sessionDir(ses.ID), "events.jsonl")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	lastError := fixtureLines(w, ses.ID, shape, n)
	if err := w.Flush(); err != nil {
		tb.Fatal(err)
	}
	if err := f.Close(); err != nil {
		tb.Fatal(err)
	}
	// Opening brings the session's row up to the file, and builds what a read uses.
	if _, err := s.Events(context.Background(), ses.ID, int64(n)); err != nil {
		tb.Fatal(err)
	}
	return &longFixture{s: s, sid: ses.ID, path: path, n: n, lastError: lastError}
}

// size is the transcript's size in bytes.
func (f *longFixture) size(tb testing.TB) int64 {
	st, err := os.Stat(f.path)
	if err != nil {
		tb.Fatal(err)
	}
	return st.Size()
}

// lastErrorOf is the seq of the last error among events, the way the attention view
// asks for it, or 0.
func lastErrorOf(events []event.Event) int64 {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == event.Error {
			return events[i].Seq
		}
	}
	return 0
}

// BenchmarkLong reports what a read of the 80,000-event fixture costs, for each shape:
// the whole transcript, the last 100 events, and the last error. The numbers measured
// before any fix are in backlog/prds/before-going-public.md.
func BenchmarkLong(b *testing.B) {
	ctx := context.Background()
	for _, shape := range []fixtureShape{shapeText, shapeTools} {
		f := newLongFixture(b, b.TempDir(), shape, longEvents)
		b.Logf("%s: %d events, %.1f MB, last error at seq %d", shape, f.n, float64(f.size(b))/1e6, f.lastError)
		b.Run(string(shape)+"/from-0", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				evs, err := f.s.Events(ctx, f.sid, 0)
				if err != nil || len(evs) != f.n {
					b.Fatalf("%d events, %v", len(evs), err)
				}
			}
		})
		b.Run(string(shape)+"/last-100", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				evs, err := f.s.Events(ctx, f.sid, int64(f.n-100))
				if err != nil || len(evs) != 100 {
					b.Fatalf("%d events, %v", len(evs), err)
				}
			}
		})
		// What the engine's attention view does since issue 042: look back from the end.
		b.Run(string(shape)+"/last-error-scan-back", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var got int64
				err := f.s.ScanBack(ctx, f.sid, func(ev event.Event) bool {
					if ev.Type == event.Error {
						got = ev.Seq
					}
					return got != 0
				})
				if err != nil || got != f.lastError {
					b.Fatalf("last error %d, want %d, %v", got, f.lastError, err)
				}
			}
		})
		b.Run(string(shape)+"/last-error-row", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ev, ok, err := f.s.LastError(ctx, f.sid)
				if err != nil || !ok || ev.Seq != f.lastError {
					b.Fatalf("last error %d, want %d, %v", ev.Seq, f.lastError, err)
				}
			}
		})
		// The recent usage poll, 60 seconds apart: about 50 new events since the last.
		b.Run(string(shape)+"/usage-poll", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if _, err := f.s.db.ExecContext(ctx, `UPDATE sessions SET recent_seq = ?, updated_at = ? WHERE id = ?`, f.n-50, time.Now().UnixMilli(), f.sid); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if err := f.s.foldRecent(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(string(shape)+"/last-error", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				evs, err := f.s.Events(ctx, f.sid, 0)
				if err != nil || lastErrorOf(evs) != f.lastError {
					b.Fatalf("last error %d, want %d, %v", lastErrorOf(evs), f.lastError, err)
				}
			}
		})
	}
}
