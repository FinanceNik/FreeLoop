package engine

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// NFR-PERF-002 (issue 042): the engine's small questions are answered from the end of
// a transcript, or from the seq they already know, and never read a whole long one.

// readWatch notes the reads of whole transcripts, which is what a read from seq 0
// without a limit is, once a session's indexes are current.
type readWatch struct {
	mu    sync.Mutex
	whole []string
}

func watchReads(e *Engine) *readWatch {
	w := &readWatch{}
	e.store.ObserveReads(func(sid string, after int64, limit int) {
		if after <= 0 && limit <= 0 {
			w.mu.Lock()
			w.whole = append(w.whole, sid)
			w.mu.Unlock()
		}
	})
	return w
}

func (w *readWatch) check(t *testing.T, what string) {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.whole) > 0 {
		t.Fatalf("%s read a whole transcript: %v", what, w.whole)
	}
}

// padChunks writes n agent text chunks of about 300 bytes, a transcript long enough
// that a read of its end is a small part of it.
func padChunks(e *Engine, sid string, n int) {
	text := strings.Repeat("word ", 60)
	for range n {
		e.write(sid, event.AgentMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: text}})
	}
}

// Folding a session in two steps, from the start of the turn the first step ended in,
// counts what folding it at once counts, wherever the first step ends.
func TestFoldActivityInStepsCountsWhatOneFoldCounts(t *testing.T) {
	t0 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.Local)
	at := func(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }
	events := []event.Event{
		evAt(1, at(0), event.TurnStarted, struct{}{}),
		evAt(2, at(1), event.UserMessageChunk, event.MessageChunk{}),
		evAt(3, at(2), event.ToolCall, map[string]any{"toolCallId": "a", "tool": "shell", "kind": "execute"}),
		evAt(4, at(3), event.PermissionRequested, map[string]any{"requestId": "r1", "toolCallId": "a", "tool": "shell", "command": "npm test"}),
		evAt(5, at(33), event.PermissionDecided, permission.Decision{RequestID: "r1", ToolCallID: "a", Outcome: permission.Allowed, Actor: permission.ActorUser}),
		evAt(6, at(34), event.ToolCallUpdate, map[string]any{"toolCallId": "a", "status": "in_progress"}),
		evAt(7, at(39), event.ToolCallUpdate, map[string]any{"toolCallId": "a", "status": "completed"}),
		evAt(8, at(40), event.Error, event.ErrorData{Message: "x"}),
		evAt(9, at(41), event.TurnEnded, event.TurnEndedData{StopReason: "end_turn"}),
		evAt(10, at(100), event.TurnStarted, struct{}{}),
		evAt(11, at(101), event.ToolCall, map[string]any{"toolCallId": "b", "tool": "write_file", "kind": "edit"}),
		evAt(12, at(105), event.ToolCallUpdate, map[string]any{"toolCallId": "b", "status": "completed"}),
		evAt(13, at(106), event.PullRequest, event.PullRequestData{}),
		evAt(14, at(110), event.TurnEnded, event.TurnEndedData{StopReason: "cancelled"}),
	}
	sum := func(parts ...[]store.ActivityCount) map[activityKey]store.ActivityCount {
		out := map[activityKey]store.ActivityCount{}
		for _, part := range parts {
			for _, c := range part {
				k := activityKey{c.Day, c.Hour, c.Kind, c.Name}
				o := out[k]
				o.Count, o.Failures, o.Millis = o.Count+c.Count, o.Failures+c.Failures, o.Millis+c.Millis
				out[k] = o
			}
		}
		return out
	}
	want := sum(foldActivity(events, 0))
	for cut := int64(1); cut < int64(len(events)); cut++ {
		first := foldActivity(events[:cut], 0)
		from := 0
		for i, ev := range events[:cut] {
			if ev.Type == event.TurnStarted {
				from = i
			}
		}
		got := sum(first, foldActivityFrom(events[from:], 0, cut))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("cut after seq %d: %v, want %v", cut, got, want)
		}
	}
}

// The answers read from the end are the ones a read of everything gave.
func TestLookingBackAnswersAsReadingEverythingDid(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "x", "ask")
	chunk := func(text string) {
		e.write(ses.ID, event.AgentMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: text}})
	}
	chunk("old reply")
	e.write(ses.ID, event.ToolCall, map[string]any{"toolCallId": "a", "tool": "shell", "kind": "execute"})
	chunk("Hello ")
	e.write(ses.ID, event.UsageUpdate, event.UsageData{InputTokens: 1})
	chunk("world ")
	if got := e.fullReply(ctx, ses.ID); got != "Hello world" {
		t.Errorf("reply = %q", got)
	}

	e.write(ses.ID, event.UserMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: "go"}})
	e.write(ses.ID, event.Error, event.ErrorData{Message: "first"})
	e.write(ses.ID, event.Error, event.ErrorData{Message: "rate limited."})
	e.write(ses.ID, event.TurnEnded, event.TurnEndedData{StopReason: "error"})
	if got := e.endedBecause(ctx, ses.ID); got != "failed before it marked the issue done (rate limited)" {
		t.Errorf("ended because = %q", got)
	}
	// An error before the user's last message is not this turn's.
	e.write(ses.ID, event.UserMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: "again"}})
	e.write(ses.ID, event.TurnEnded, event.TurnEndedData{StopReason: "max_tokens"})
	if got := e.endedBecause(ctx, ses.ID); got != "ran out of output room before it marked the issue done" {
		t.Errorf("ended because = %q", got)
	}
}

func TestSmallQuestionsDoNotReadWholeTranscripts(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()

	// A session that failed early in a long transcript, and one whose turn ended at
	// the end of one.
	failed, _ := demoSession(t, e, "failed", "ask")
	e.emit(failed.ID, event.TurnStarted, struct{}{})
	e.emit(failed.ID, event.Error, event.ErrorData{Message: "the key was refused"})
	e.emit(failed.ID, event.SessionStatus, event.SessionStatusData{Status: "error"})
	padChunks(e, failed.ID, 3000)
	done, _ := demoSession(t, e, "done", "ask")
	e.emit(done.ID, event.TurnStarted, struct{}{})
	e.emit(done.ID, event.UserMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: "go"}})
	padChunks(e, done.ID, 3000)
	e.emit(done.ID, event.PullRequest, event.PullRequestData{URL: "https://example.test/pr/1"})
	e.emit(done.ID, event.TurnEnded, event.TurnEndedData{StopReason: "cancelled"})
	e.emit(done.ID, event.SessionStatus, event.SessionStatusData{Status: "idle"})

	// The first fold of a session's activity and usage reads what is there; the
	// polls after it are what must stay small.
	turns := func() int64 {
		rep, err := e.ActivityReport(ctx, 7)
		if err != nil || rep.Pending != 0 {
			t.Fatalf("report: %+v, %v", rep, err)
		}
		var n int64
		for _, r := range rep.Rows {
			if r.Kind == activityTurn && r.SessionID == done.ID {
				n += r.Count
			}
		}
		return n
	}
	if n := turns(); n != 1 {
		t.Fatalf("turns before = %d", n)
	}
	if _, err := e.RecentUsage(ctx, 1); err != nil {
		t.Fatal(err)
	}

	w := watchReads(e)

	e.emit(done.ID, event.TurnStarted, struct{}{})
	e.emit(done.ID, event.UsageUpdate, event.UsageData{InputTokens: 10, OutputTokens: 5})
	e.emit(done.ID, event.TurnEnded, event.TurnEndedData{StopReason: "end_turn"})

	items, err := e.Attention(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, it := range items {
		if it.Session.ID == failed.ID {
			seen = true
			if it.Error != "the key was refused" {
				t.Errorf("error = %q", it.Error)
			}
		}
	}
	if !seen {
		t.Fatalf("attention: %+v", items)
	}
	w.check(t, "attention")

	if n := turns(); n != 2 {
		t.Errorf("turns after = %d, want the new one counted once", n)
	}
	w.check(t, "activity")
	if n := turns(); n != 2 {
		t.Errorf("turns asked again = %d", n)
	}

	rep, err := e.RecentUsage(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var calls int64
	for _, b := range rep.Buckets {
		calls += b.Calls
	}
	if calls != 1 {
		t.Errorf("calls = %d", calls)
	}
	w.check(t, "live usage")

	if got := e.taskState(ctx, done.ID); got != "done" {
		t.Errorf("task state = %q", got)
	}
	w.check(t, "the queue")

	if got := e.endedBecause(ctx, done.ID); got != "ended without marking it done" {
		t.Errorf("ended because = %q", got)
	}
	w.check(t, "the AFK tick")

	if pr := e.lastPullRequest(ctx, done.ID); pr == nil || pr.URL != "https://example.test/pr/1" {
		t.Errorf("pull request = %+v", pr)
	}
	w.check(t, "the pull request")
	_ = permission.Allowed
}
