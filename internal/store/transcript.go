package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// transcript is one session's open events.jsonl.
type transcript struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	lastSeq int64
	dirty   bool
}

// openTranscript opens path for appending. A torn last line, left by a crash
// in the middle of a write, is truncated away; lastSeq is read from the file.
func openTranscript(path string) (*transcript, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("store: open transcript: %w", err)
	}
	lastSeq, validLen, err := scanTranscript(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("store: %s: %w", path, err)
	}
	if err := f.Truncate(validLen); err != nil {
		f.Close()
		return nil, fmt.Errorf("store: truncate torn transcript tail: %w", err)
	}
	if _, err := f.Seek(validLen, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	return &transcript{path: path, f: f, lastSeq: lastSeq}, nil
}

// scanTranscript returns the last seq and the length of the file up to and
// including its last complete line.
func scanTranscript(r io.Reader) (lastSeq, validLen int64, err error) {
	br := bufio.NewReader(r)
	var offset int64
	for line := 1; ; line++ {
		b, err := br.ReadBytes('\n')
		if err == io.EOF {
			return lastSeq, offset, nil // b, if any, is a torn tail
		}
		if err != nil {
			return 0, 0, err
		}
		var head struct {
			Seq int64 `json:"seq"`
		}
		if err := json.Unmarshal(b, &head); err != nil {
			return 0, 0, fmt.Errorf("line %d: %w", line, err)
		}
		if head.Seq != lastSeq+1 {
			return 0, 0, fmt.Errorf("line %d: seq %d after %d", line, head.Seq, lastSeq)
		}
		lastSeq = head.Seq
		offset += int64(len(b))
	}
}

// write appends ev as one line with a single write call. Callers hold t.mu.
func (t *transcript) write(ev event.Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("store: marshal event: %w", err)
	}
	if bytes.IndexByte(b, '\n') >= 0 {
		return fmt.Errorf("store: marshalled event contains a newline")
	}
	if _, err := t.f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("store: append event: %w", err)
	}
	t.lastSeq = ev.Seq
	t.dirty = true
	return nil
}

func (t *transcript) syncIfDirty() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dirty {
		if t.f.Sync() == nil {
			t.dirty = false
		}
	}
}

func (t *transcript) close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	syncErr := t.f.Sync()
	if err := t.f.Close(); err != nil {
		return err
	}
	return syncErr
}

// readEvents reads the events with seq greater than afterSeq from path.
func readEvents(path string, afterSeq int64) ([]event.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	defer f.Close()
	out := []event.Event{}
	br := bufio.NewReader(f)
	for {
		b, err := br.ReadBytes('\n')
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var ev event.Event
		if err := json.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("store: %s: %w", path, err)
		}
		if ev.Seq > afterSeq {
			out = append(out, ev)
		}
	}
}
