package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Review finding: one damaged line in the middle made the session
// unreadable and unwritable. It is now repaired on open: readable events
// keep their seq, each lost one becomes a notice, and the original stays
// beside the transcript.
func TestDamagedTranscriptIsRepaired(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ses_x")
	os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, "events.jsonl")
	lines := []string{
		`{"v":1,"seq":1,"ts":"2026-10-31T10:00:00Z","sid":"ses_x","type":"session_status","data":{"status":"idle"}}`,
		`{"v":1,"seq":2,"ts":"2026-10-31T10:00:01Z","sid":"ses_x","type":"turn_star`,
		`{"v":1,"seq":3,"ts":"2026-10-31T10:00:02Z","sid":"ses_x","type":"notice","data":{"message":"kept"}}`,
		`{"v":1,"seq":4,"ts":"2026-10-31T10:00:03Z","sid":"ses_x","type":"session_status","data":{"status":"running"}}`,
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"+`{"v":1,"seq":5,"torn`), 0o600)

	tr, err := openTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if tr.lastSeq != 4 {
		t.Errorf("lastSeq = %d", tr.lastSeq)
	}
	if err := tr.write(event.Event{V: 1, Seq: 5, SID: "ses_x", Type: event.Notice, Data: []byte(`{"message":"new"}`)}); err != nil {
		t.Fatal(err)
	}
	tr.close()
	evs, err := readEvents(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 5 {
		t.Fatalf("events = %d", len(evs))
	}
	for i, ev := range evs {
		if ev.Seq != int64(i+1) {
			t.Errorf("event %d has seq %d", i, ev.Seq)
		}
	}
	if evs[1].Type != event.Notice || !strings.Contains(string(evs[1].Data), "damaged line") || !strings.Contains(string(evs[2].Data), "kept") {
		t.Errorf("events = %+v", evs[1:3])
	}
	backups, _ := filepath.Glob(path + ".damaged-*")
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	if b, _ := os.ReadFile(backups[0]); !strings.Contains(string(b), "turn_star") {
		t.Error("the backup lost the damaged line")
	}
}
