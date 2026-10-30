package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func toolOutput(id, out string) json.RawMessage {
	return raw(map[string]any{"toolCallId": id, "status": "completed", "content": []any{
		map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": out}},
	}})
}

func searchFor(t *testing.T, s *Store, q string) []Hit {
	t.Helper()
	hits, err := s.Search(context.Background(), q, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// SES-007: messages streamed in chunks, tool calls and their output are
// found, by phrase too, with the event where the message or tool call
// starts and a marked snippet; a deleted session's text is gone.
func TestSearchIndexesAsItAppends(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin"})
	b, _ := s.CreateSession(ctx, session.Session{Title: "b", HarnessID: "builtin"})

	s.Append(ctx, a.ID, event.UserMessageChunk, text("Why does the flaky test fail?"))
	s.Append(ctx, a.ID, event.TurnStarted, nil)
	first, _ := s.Append(ctx, a.ID, event.AgentMessageChunk, text("The race is in the conn"))
	s.Append(ctx, a.ID, event.AgentMessageChunk, text("ection pool's reaper."))
	call, _ := s.Append(ctx, a.ID, event.ToolCall, raw(map[string]any{"toolCallId": "c1", "title": "Run go test", "rawInput": map[string]any{"command": "go test ./pool/..."}}))
	s.Append(ctx, a.ID, event.ToolCallUpdate, toolOutput("c1", "panic: send on closed channel in reaper_loop"))
	s.Append(ctx, b.ID, event.AgentMessageChunk, text("Nothing about pools here, only a closed door."))
	s.Append(ctx, b.ID, event.TurnEnded, nil)
	// The last message is still being streamed: it is not searchable yet.
	s.Append(ctx, a.ID, event.AgentMessageChunk, text("pending words zanzibar"))

	if h := searchFor(t, s, "connection pool"); len(h) != 1 || h[0].SessionID != a.ID || h[0].Seq != first.Seq || h[0].Kind != HitAgent {
		t.Fatalf("connection pool = %+v", h)
	}
	if h := searchFor(t, s, `"send on closed channel"`); len(h) != 1 || h[0].Seq != call.Seq || h[0].Kind != HitTool ||
		!strings.Contains(h[0].Snippet, SnippetOpen+"send on closed channel"+SnippetClose) {
		t.Fatalf("phrase = %+v", h)
	}
	if h := searchFor(t, s, `"closed send"`); len(h) != 0 {
		t.Errorf("the words of a phrase matched out of order: %+v", h)
	}
	if h := searchFor(t, s, "closed"); len(h) != 2 {
		t.Errorf("closed = %+v", h)
	}
	if h := searchFor(t, s, "reaper_loop"); len(h) != 1 {
		t.Errorf("an identifier with an underscore = %+v", h)
	}
	if h := searchFor(t, s, "flak"); len(h) != 1 || h[0].Kind != HitUser {
		t.Errorf("prefix of the word being typed = %+v", h)
	}
	if h := searchFor(t, s, "zanzibar"); len(h) != 0 {
		t.Errorf("a message still streaming was indexed: %+v", h)
	}
	s.Append(ctx, a.ID, event.TurnEnded, nil)
	if h := searchFor(t, s, "zanzibar"); len(h) != 1 {
		t.Errorf("a finished message was not indexed: %+v", h)
	}
	sum, _ := s.Append(ctx, a.ID, event.CompactionUpdate, raw(event.CompactionData{Status: "completed", Reason: "auto", Summary: "We traced the leak to the quokka cache."}))
	if h := searchFor(t, s, "quokka"); len(h) != 1 || h[0].Kind != HitSummary || h[0].Seq != sum.Seq {
		t.Errorf("a compaction summary = %+v", h)
	}
	for _, q := range []string{"", `"`, "AND OR NOT", "NEAR(", "*", "col:umn", `a"b`} {
		if _, err := s.Search(ctx, q, 0, 0); err != nil {
			t.Errorf("Search(%q) = %v", q, err)
		}
	}

	if err := s.DeleteSession(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if h := searchFor(t, s, "reaper"); len(h) != 0 {
		t.Errorf("a deleted session's text is still found: %+v", h)
	}
}

// Transcripts written before the index existed are indexed on open, and a
// session appended to before that catch-up ran is not indexed twice.
func TestSearchCatchesUpOldTranscripts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir, nil)
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin"})
	b, _ := s.CreateSession(ctx, session.Session{Title: "b", HarnessID: "builtin"})
	s.Append(ctx, a.ID, event.AgentMessageChunk, text("an old answer about kerberos"))
	s.Append(ctx, a.ID, event.TurnEnded, nil)
	s.Append(ctx, b.ID, event.ToolCallUpdate, toolOutput("c", "old output mentioning kerberos"))
	// Forget the index, as a database from before it would be.
	s.db.Exec(`DELETE FROM search`)
	s.db.Exec(`UPDATE sessions SET indexed_seq = 0`)
	s.Close()

	s = openTest(t, dir, nil)
	defer s.Close()
	s.Append(ctx, a.ID, event.UserMessageChunk, text("and kerberos again"))
	s.Append(ctx, a.ID, event.TurnStarted, nil)
	s.indexed.Wait()
	if h := searchFor(t, s, "kerberos"); len(h) != 3 {
		t.Fatalf("kerberos = %+v", h)
	}
}

// SES-007's acceptance: a phrase from a tool result weeks old is found in
// under a second, among months of sessions.
func TestSearchIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	words := strings.Fields("build test deploy error panic config parser token cache request handler session module schema index query router worker retry timeout")
	var needle string
	const sessions, rows = 200, 120
	tx, _ := s.db.Begin()
	for i := range sessions {
		sid := fmt.Sprintf("s%03d", i)
		for j := range rows {
			var sb strings.Builder
			for k := range 60 {
				sb.WriteString(words[(i*7+j*3+k*k)%len(words)])
				sb.WriteByte(' ')
			}
			if i == 120 && j == 77 {
				needle = "unexpected EOF while decoding manifest.json"
				sb.WriteString(needle)
			}
			tx.Exec(`INSERT INTO search (body, session_id, seq, kind) VALUES (?, ?, ?, ?)`, sb.String(), sid, j+1, HitTool)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	hits, err := s.Search(ctx, `"`+needle+`"`, 0, 0)
	took := time.Since(start)
	if err != nil || len(hits) != 1 || hits[0].SessionID != "s120" || hits[0].Seq != 78 {
		t.Fatalf("hits = %+v, %v", hits, err)
	}
	if took > time.Second {
		t.Errorf("the search took %v among %d rows", took, sessions*rows)
	}
	t.Logf("a phrase among %d rows: %v", sessions*rows, took)
	// A common word is still quick, ranked and capped.
	start = time.Now()
	hits, _ = s.Search(ctx, "timeout", 0, 0)
	if took := time.Since(start); len(hits) != 50 || took > time.Second {
		t.Errorf("a common word: %d hits in %v", len(hits), took)
	}
}

func TestMatchExpr(t *testing.T) {
	for in, want := range map[string]string{
		"conn pool":           `"conn" "pool"*`,
		"conn pool ":          `"conn" "pool"`,
		`"send on closed" ch`: `"send on closed" "ch"*`,
		`"send on closed"`:    `"send on closed"`,
		"reaper_loop":         `"reaper loop"*`,
		"NEAR( x":             `"NEAR" "x"*`,
		"*":                   ``,
		`a"b`:                 `"a" "b"`,
	} {
		if got := matchExpr(in); got != want {
			t.Errorf("matchExpr(%q) = %s, want %s", in, got, want)
		}
	}
}

// A failed index write is caught up on the next append instead of being
// skipped for good.
func TestSearchRecoversFromAFailedWrite(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin"})
	s.Append(ctx, a.ID, event.TurnStarted, nil)
	// Make the next index write fail.
	s.db.Exec(`CREATE TRIGGER fail BEFORE INSERT ON usage BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
	s.Append(ctx, a.ID, event.UsageUpdate, raw(event.UsageData{InputTokens: 5, Model: "m"}))
	s.Append(ctx, a.ID, event.AgentMessageChunk, text("the lost ostrich"))
	s.db.Exec(`DROP TRIGGER fail`)
	s.Append(ctx, a.ID, event.TurnEnded, nil)
	if h := searchFor(t, s, "ostrich"); len(h) != 1 {
		t.Errorf("hits = %+v", h)
	}
	if u, _ := s.UsageSince(ctx, "2000-01-01"); len(u) != 1 || u[0].InputTokens != 5 {
		t.Errorf("usage = %+v", u)
	}
}

// One session with many matches does not crowd out another.
func TestSearchCapsHitsPerSession(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	for i := range 40 {
		s.db.Exec(`INSERT INTO search (body, session_id, seq, kind) VALUES (?, 'long', ?, 'tool')`, "kiwi kiwi kiwi kiwi", i+1)
	}
	s.db.Exec(`INSERT INTO search (body, session_id, seq, kind) VALUES ('a kiwi and much else besides, said once', 'short', 1, 'agent')`)
	hits, err := s.Search(ctx, "kiwi", 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	per := map[string]int{}
	for _, h := range hits {
		per[h.SessionID]++
	}
	if per["long"] != 3 || per["short"] != 1 || !strings.Contains(hits[0].Snippet, SnippetOpen) {
		t.Errorf("hits per session = %v", per)
	}
}

// A catch-up that ends in the middle of a streamed message keeps
// streaming into it: the indexer is not copied with text in its buffer.
func TestCatchUpMidMessage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir, nil)
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin"})
	s.Append(ctx, a.ID, event.TurnStarted, nil)
	s.Append(ctx, a.ID, event.AgentMessageChunk, text("the wombat "))
	s.db.Exec(`UPDATE sessions SET indexed_seq = 0`)
	s.db.Exec(`DELETE FROM search`)
	s.Close()
	s = openTest(t, dir, nil)
	defer s.Close()
	s.indexed.Wait()
	s.Append(ctx, a.ID, event.AgentMessageChunk, text("burrows deep"))
	s.Append(ctx, a.ID, event.TurnEnded, nil)
	if h := searchFor(t, s, `"wombat burrows"`); len(h) != 1 {
		t.Errorf("hits = %+v", h)
	}
}
