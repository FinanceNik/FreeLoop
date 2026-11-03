package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// transcript is one session's open events.jsonl.
type transcript struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	lastSeq int64
	// size is how much of the file holds whole lines.
	size   int64
	dirty  bool
	search indexer // guarded by mu
}

// ErrWrite marks an event that could not be written to its transcript, as on a
// full disk, unlike one that is refused or for a session that is gone.
var ErrWrite = errors.New("could not write the transcript")

// openTranscript opens path for appending. A torn last line, left by a crash
// in the middle of a write, is truncated away; lastSeq is read from the file.
// A damaged line elsewhere is repaired (see repairTranscript), so one bad
// line does not make the whole session unreadable.
func openTranscript(path string) (*transcript, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("store: open transcript: %w", err)
	}
	lastSeq, validLen, err := scanTranscript(f)
	var damaged *damage
	if errors.As(err, &damaged) {
		f.Close()
		if err := repairTranscript(path); err != nil {
			return nil, fmt.Errorf("store: repair %s: %w", path, err)
		}
		if f, err = os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0o600); err != nil {
			return nil, fmt.Errorf("store: open transcript: %w", err)
		}
		lastSeq, validLen, err = scanTranscript(f)
	}
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
	return &transcript{path: path, f: f, lastSeq: lastSeq, size: validLen}, nil
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
			return 0, 0, &damage{fmt.Sprintf("line %d: %v", line, err)}
		}
		if head.Seq != lastSeq+1 {
			return 0, 0, &damage{fmt.Sprintf("line %d: seq %d after %d", line, head.Seq, lastSeq)}
		}
		lastSeq = head.Seq
		offset += int64(len(b))
	}
}

// damage is a complete line that is not the next event: unreadable, or out
// of sequence.
type damage struct{ what string }

func (d *damage) Error() string { return d.what }

// maxGap bounds the seq gap a repair fills: a line claiming a seq far ahead
// is damaged itself, not the end of a run of lost lines.
const maxGap = 10000

// repairTranscript rewrites a damaged transcript. The original is kept
// beside it as events.jsonl.damaged-<time>. Every readable event stays,
// in order, with its seq; unreadable lines and events out of order are
// left out, and each seq they leave empty holds a notice saying a damaged
// line was set aside, so seqs stay contiguous and checkpoints, forks and
// search hits that point at a seq still point at the same event. A torn
// last line is dropped as on any open.
func repairTranscript(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	keep := bytes.LastIndexByte(b, '\n') + 1 // a torn tail goes
	backup := fmt.Sprintf("%s.damaged-%d", path, time.Now().Unix())
	if err := os.WriteFile(backup, b, 0o600); err != nil {
		return err
	}
	var out bytes.Buffer
	// Before the first readable event, the folder names the session.
	last := event.Event{SID: filepath.Base(filepath.Dir(path)), TS: time.Now().UTC()}
	dropped, tail := 0, 0
	placeholder := func(seq int64) {
		data, _ := json.Marshal(event.NoticeData{Message: fmt.Sprintf("A damaged line of this transcript was set aside here; the original file is %s.", filepath.Base(backup))})
		ev := event.Event{V: event.Version, Seq: seq, TS: last.TS, SID: last.SID, Type: event.Notice, Data: data}
		line, _ := json.Marshal(ev)
		out.Write(append(line, '\n'))
	}
	for _, line := range bytes.Split(b[:keep], []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev event.Event
		if json.Unmarshal(line, &ev) != nil || ev.Seq <= last.Seq || ev.Seq-last.Seq > maxGap || !ev.Type.Valid() {
			dropped++
			tail++
			continue
		}
		tail = 0
		for seq := last.Seq + 1; seq < ev.Seq; seq++ {
			placeholder(seq)
		}
		out.Write(append(bytes.TrimSpace(line), '\n'))
		last = ev
	}
	// Lines lost after the last readable event leave no gap to mark; say
	// so after it.
	if tail > 0 {
		placeholder(last.Seq + 1)
	}
	tmp := path + ".repair"
	if err := os.WriteFile(tmp, out.Bytes(), 0o600); err != nil {
		return err
	}
	log.Printf("store: repaired %s; %d damaged lines, original kept as %s", path, dropped, backup)
	return os.Rename(tmp, path)
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
	n, err := t.f.Write(append(b, '\n'))
	if err != nil {
		// A part of a line would swallow the next event when it is appended.
		if n > 0 {
			if terr := t.f.Truncate(t.size); terr != nil {
				err = errors.Join(err, terr)
			}
		}
		return fmt.Errorf("store: append event: %w: %w", ErrWrite, err)
	}
	t.size += int64(n)
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
			// A damaged line is repaired when the transcript opens; a
			// reader that comes first skips it.
			log.Printf("store: %s: skipped a damaged line: %v", path, err)
			continue
		}
		if ev.Seq > afterSeq {
			out = append(out, ev)
		}
	}
}
