package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// NFR-PERF-003: 50 archived and 10 active sessions, each with a transcript of a few
// hundred events, are back within 3 seconds of opening the data folder: the engine
// open, the session list, and the newest session's events, as the window needs them.
func TestRestoringSixtySessionsIsQuick(t *testing.T) {
	if testing.Short() {
		t.Skip("writes 18,000 events")
	}
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	var newest string
	for i := 0; i < 60; i++ {
		ses, _ := demoSession(t, e, fmt.Sprintf("session %d", i), "ask")
		for j := 0; j < 300; j++ {
			e.emit(ses.ID, event.AgentMessageChunk, event.MessageChunk{Content: event.ContentBlock{Type: "text", Text: fmt.Sprintf("line %d of a long reply, with enough words to look like one.", j)}})
		}
		if i < 50 {
			if err := e.ArchiveSession(ctx, ses.ID, true); err != nil {
				t.Fatal(err)
			}
		}
		newest = ses.ID
	}
	e.Close()

	start := time.Now()
	e2, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	list, err := e2.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := e2.Events(ctx, newest, 0)
	if err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	t.Logf("restored %d sessions and %d events of the newest in %v", len(list), len(evs), took)
	if len(list) != 60 || len(evs) < 300 {
		t.Errorf("restored %d sessions, %d events", len(list), len(evs))
	}
	if took > 3*time.Second {
		t.Errorf("restoring took %v; NFR-PERF-003 allows 3 s", took)
	}
}
