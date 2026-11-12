package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// A transcript is read from where it is asked (NFR-PERF-002, issue 041): a sparse
// table of seq to byte offset, a read outside the append lock, and the same events
// as a parse of the whole file.

func notice(seq int, text string) string {
	b, _ := json.Marshal(event.Event{V: event.Version, Seq: int64(seq), TS: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), SID: "ses_x", Type: event.Notice, Data: json.RawMessage(fmt.Sprintf(`{"message":%q}`, text))})
	return string(b)
}

// checkReads compares every kind of read of an open transcript with the parse of the
// whole file, which is what a read was before the table.
func checkReads(t *testing.T, tr *transcript, path string) {
	t.Helper()
	whole, err := readEvents(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(whole)) != tr.lastSeq {
		t.Fatalf("the file holds %d events, lastSeq is %d", len(whole), tr.lastSeq)
	}
	n := int64(len(whole))
	afters := []int64{-5, 0, 1, offsetStride - 1, offsetStride, offsetStride + 1, 2*offsetStride - 1, 2 * offsetStride, n - 100, n - 1, n, n + 10}
	for _, after := range afters {
		want, _ := readEvents(path, after)
		for _, limit := range []int{0, 1, 7, offsetStride, 100000} {
			got, err := tr.read(after, limit)
			if err != nil {
				t.Fatal(err)
			}
			w := want
			if limit > 0 && len(w) > limit {
				w = w[:limit]
			}
			if len(got) != len(w) || (len(w) > 0 && !reflect.DeepEqual(got, w)) {
				t.Errorf("read(after %d, limit %d) gave %d events, want %d", after, limit, len(got), len(w))
			}
		}
	}
	for _, k := range []int{1, 5, 100, int(n), int(n) + 50} {
		got, err := tr.tail(k)
		if err != nil {
			t.Fatal(err)
		}
		start := max(0, len(whole)-k)
		if len(got) != len(whole)-start || (len(got) > 0 && !reflect.DeepEqual(got, whole[start:])) {
			t.Errorf("tail(%d) gave %d events, want %d", k, len(got), len(whole)-start)
		}
	}
	// Each entry of the table points at the line of its seq.
	for k, off := range tr.offsets {
		line := readLineAt(t, path, off)
		var ev event.Event
		if err := json.Unmarshal(line, &ev); err != nil || ev.Seq != int64(k)*offsetStride+1 {
			t.Errorf("table entry %d points at seq %d (%v)", k, ev.Seq, err)
		}
	}
	if want := (n + offsetStride - 1) / offsetStride; int64(len(tr.offsets)) != want {
		t.Errorf("table has %d entries for %d events, want %d", len(tr.offsets), n, want)
	}
}

func readLineAt(t *testing.T, path string, off int64) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = b[off:]
	return b[:bytes.IndexByte(b, '\n')]
}

func transcriptOf(t *testing.T, n int, damage func(lines []string) []string, tail string) (*transcript, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ses_x")
	os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, "events.jsonl")
	var lines []string
	for i := 1; i <= n; i++ {
		lines = append(lines, notice(i, strings.Repeat("w", i%50)))
	}
	if damage != nil {
		lines = damage(lines)
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"+tail), 0o600)
	tr, err := openTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.close() })
	return tr, path
}

func TestReadsGiveWhatAFullParseGives(t *testing.T) {
	tr, path := transcriptOf(t, 3*offsetStride+17, nil, "")
	checkReads(t, tr, path)
	// The table is kept as events are appended.
	for i := 0; i < 2*offsetStride; i++ {
		ev := event.Event{V: 1, Seq: tr.lastSeq + 1, SID: "ses_x", Type: event.Notice, Data: []byte(`{"message":"more"}`)}
		tr.mu.Lock()
		err := tr.write(ev)
		tr.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	checkReads(t, tr, path)
}

func TestTableOfAnEmptyAndAShortTranscript(t *testing.T) {
	for _, n := range []int{0, 1, offsetStride, offsetStride + 1} {
		tr, path := transcriptOf(t, n, nil, "")
		if n == 0 {
			if evs, err := tr.read(0, 0); err != nil || len(evs) != 0 {
				t.Errorf("empty: %d events, %v", len(evs), err)
			}
			continue
		}
		checkReads(t, tr, path)
	}
}

func TestTableIsRebuiltAfterRepairAndTruncation(t *testing.T) {
	// A damaged line in the middle, a torn line at the end.
	tr, path := transcriptOf(t, 2*offsetStride+40, func(l []string) []string {
		l[offsetStride+5] = l[offsetStride+5][:20]
		l[10] = "not json"
		return l
	}, `{"v":1,"seq":999,"torn`)
	checkReads(t, tr, path)
	// A transcript of lines whose seqs are out of order is repaired the same way.
	tr2, path2 := transcriptOf(t, offsetStride+30, func(l []string) []string {
		l[offsetStride-3], l[offsetStride-2] = l[offsetStride-2], l[offsetStride-3]
		return l
	}, "")
	checkReads(t, tr2, path2)
}

func TestReadDoesNotHoldTheAppendLock(t *testing.T) {
	tr, path := transcriptOf(t, 2*offsetStride, nil, "")
	appended := make(chan error, 1)
	readHook = func() {
		go func() {
			tr.mu.Lock()
			defer tr.mu.Unlock()
			appended <- tr.write(event.Event{V: 1, Seq: tr.lastSeq + 1, SID: "ses_x", Type: event.Notice, Data: []byte(`{"message":"during"}`)})
		}()
		select {
		case err := <-appended:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("an append waited for a read")
		}
	}
	defer func() { readHook = nil }()
	got, err := tr.read(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	readHook = nil
	// What it returns is what the file held when it began.
	if len(got) != 2*offsetStride {
		t.Errorf("the read returned %d events, want %d", len(got), 2*offsetStride)
	}
	checkReads(t, tr, path)
}

// The numbers of the PRD: the last 100 events of 80,000 take under 5 ms, and what it
// allocates does not depend on the size of the file.
func TestTailOfALongTranscriptIsCheapAndSmall(t *testing.T) {
	if testing.Short() {
		t.Skip("writes 100 MB")
	}
	ctx := context.Background()
	for _, shape := range []fixtureShape{shapeText, shapeTools} {
		f := newLongFixture(t, t.TempDir(), shape, longEvents)
		whole, err := f.s.Events(ctx, f.sid, 0)
		if err != nil || len(whole) != f.n {
			t.Fatalf("%d events, %v", len(whole), err)
		}
		got, err := f.s.TailEvents(ctx, f.sid, 100)
		if err != nil || !reflect.DeepEqual(got, whole[f.n-100:]) {
			t.Fatalf("%s: the last 100 events differ from the full read's (%d, %v)", shape, len(got), err)
		}
		lim, err := f.s.EventsLimit(ctx, f.sid, int64(f.n/2), 50)
		if err != nil || !reflect.DeepEqual(lim, whole[f.n/2:f.n/2+50]) {
			t.Fatalf("%s: a limited read from the middle differs (%d, %v)", shape, len(lim), err)
		}
		best, bytesPer := time.Hour, uint64(0)
		for i := 0; i < 20; i++ {
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			start := time.Now()
			evs, err := f.s.TailEvents(ctx, f.sid, 100)
			took := time.Since(start)
			runtime.ReadMemStats(&after)
			if err != nil || len(evs) != 100 {
				t.Fatalf("%d events, %v", len(evs), err)
			}
			if took < best {
				best = took
			}
			bytesPer = after.TotalAlloc - before.TotalAlloc
		}
		t.Logf("%s: last 100 of %d events (%.0f MB) in %v, %.2f MB allocated", shape, f.n, float64(f.size(t))/1e6, best, float64(bytesPer)/1e6)
		if !raceOn && best > 5*time.Millisecond {
			t.Errorf("%s: the last 100 events took %v, want under 5 ms", shape, best)
		}
		// A few hundred lines at most, whatever the size of the file.
		if bytesPer > 8<<20 || bytesPer > uint64(f.size(t))/20 {
			t.Errorf("%s: the last 100 events allocated %d bytes of a %d-byte file", shape, bytesPer, f.size(t))
		}
	}
}
