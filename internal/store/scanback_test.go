package store

import (
	"context"
	"os"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// ScanBack finds the last event of a kind by reading the file from its end
// (NFR-PERF-002, issue 042).

func TestScanBackFindsTheLastErrorWithoutReadingFromTheStart(t *testing.T) {
	ctx := context.Background()
	f := newLongFixture(t, t.TempDir(), shapeText, 20000)
	var reads []int64
	f.s.ObserveReads(func(sid string, after int64, limit int) { reads = append(reads, after) })
	var got int64
	visited := 0
	err := f.s.ScanBack(ctx, f.sid, func(ev event.Event) bool {
		visited++
		if ev.Type == event.Error {
			got = ev.Seq
			return true
		}
		return false
	})
	if err != nil || got != f.lastError {
		t.Fatalf("last error %d, want %d, %v", got, f.lastError, err)
	}
	if len(reads) != 0 {
		t.Fatalf("it read from the start: %v", reads)
	}
	if visited >= f.n/2 {
		t.Fatalf("it visited %d of %d events", visited, f.n)
	}
}

func TestScanBackVisitsEveryEventNewestFirstAndSkipsATornLine(t *testing.T) {
	ctx := context.Background()
	f := newLongFixture(t, t.TempDir(), shapeTools, 3000)
	// A line being written has no newline yet.
	fh, err := os.OpenFile(f.path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	fh.WriteString(`{"v":1,"seq":3001,"type":"no`)
	fh.Close()
	want, err := readEvents(f.path, 0)
	if err != nil {
		t.Fatal(err)
	}
	var seqs []int64
	f.s.ObserveReads(func(string, int64, int) { seqs = append(seqs, -1) })
	if err := f.s.ScanBack(ctx, f.sid, func(ev event.Event) bool { seqs = append(seqs, ev.Seq); return false }); err != nil {
		t.Fatal(err)
	}
	if seqs[len(seqs)-1] != -1 {
		t.Fatalf("reaching the first line was not noted")
	}
	seqs = seqs[:len(seqs)-1]
	if len(seqs) != len(want) {
		t.Fatalf("%d events, want %d", len(seqs), len(want))
	}
	for i, s := range seqs {
		if s != want[len(want)-1-i].Seq {
			t.Fatalf("event %d is seq %d, want %d", i, s, want[len(want)-1-i].Seq)
		}
	}
}

func TestScanBackDoesNotOpenAReleasedSession(t *testing.T) {
	ctx := context.Background()
	f := newLongFixture(t, t.TempDir(), shapeText, 5000)
	f.s.Release(f.sid)
	var first int64
	if err := f.s.ScanBack(ctx, f.sid, func(ev event.Event) bool { first = ev.Seq; return true }); err != nil {
		t.Fatal(err)
	}
	if first != 5000 {
		t.Fatalf("newest event is seq %d", first)
	}
	if f.s.IsOpen(f.sid) {
		t.Fatal("the read opened the transcript again")
	}
}

func TestLastErrorReadsOneEventAndFallsBackForARowThatDoesNotSay(t *testing.T) {
	ctx := context.Background()
	f := newLongFixture(t, t.TempDir(), shapeTools, 20000)
	type read struct {
		after int64
		limit int
	}
	var reads []read
	f.s.ObserveReads(func(sid string, after int64, limit int) { reads = append(reads, read{after, limit}) })
	ev, ok, err := f.s.LastError(ctx, f.sid)
	if err != nil || !ok || ev.Seq != f.lastError {
		t.Fatalf("last error %d (%v), want %d, %v", ev.Seq, ok, f.lastError, err)
	}
	if len(reads) != 1 || reads[0] != (read{f.lastError - 1, 1}) {
		t.Fatalf("reads %v, want one event at %d", reads, f.lastError)
	}
	// A row from before the column says 0: the answer is the same, found from the end.
	if _, err := f.s.db.ExecContext(ctx, `UPDATE sessions SET last_error_seq = 0 WHERE id = ?`, f.sid); err != nil {
		t.Fatal(err)
	}
	reads = nil
	ev, ok, err = f.s.LastError(ctx, f.sid)
	if err != nil || !ok || ev.Seq != f.lastError || len(reads) != 0 {
		t.Fatalf("fallback: last error %d (%v), reads %v, %v", ev.Seq, ok, reads, err)
	}
	// A new error moves it.
	n, err := f.s.Append(ctx, f.sid, event.Error, []byte(`{"message":"again"}`))
	if err != nil {
		t.Fatal(err)
	}
	ev, ok, err = f.s.LastError(ctx, f.sid)
	if err != nil || !ok || ev.Seq != n.Seq {
		t.Fatalf("after an error: %d (%v), want %d, %v", ev.Seq, ok, n.Seq, err)
	}
}
