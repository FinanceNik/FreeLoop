package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// scanWindow is how many bytes ScanBack reads first; each further read is four times
// the one before, so a lookup that must go far back costs about a third more than one
// read of what it covers.
const scanWindow = 64 << 10

// ScanBack calls visit with a session's events, the newest first, until visit
// returns true or the events run out. It is for the question "what was the last
// event of this kind" and reads only as far back as the answer is. It reads the file
// by its end and does not open the transcript, so asking about a session that was
// released (OQ-053) does not hold its file open again.
func (s *Store) ScanBack(ctx context.Context, sid string, visit func(event.Event) bool) error {
	if _, err := s.GetSession(ctx, sid); err != nil {
		return err
	}
	path := filepath.Join(s.sessionDir(sid), "events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("store: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	// end is where the lines not yet visited end, always just after a newline.
	end, win := st.Size(), int64(scanWindow)
	for end > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		start := max(end-win, 0)
		buf := make([]byte, end-start)
		if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
			return fmt.Errorf("store: %w", err)
		}
		if end == st.Size() {
			// A line being written, or torn by a crash, has no newline yet.
			if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
				buf = buf[:i+1]
				end = start + int64(len(buf))
			} else if start > 0 {
				win *= 4
				continue
			} else {
				return nil
			}
		}
		lines := buf
		if start > 0 {
			i := bytes.IndexByte(buf, '\n')
			if i < 0 {
				win *= 4 // a line longer than the window
				continue
			}
			lines = buf[i+1:]
			end = start + int64(i) + 1
		} else {
			end = 0
		}
		lines = bytes.TrimSuffix(lines, []byte("\n"))
		for len(lines) > 0 {
			i := bytes.LastIndexByte(lines, '\n')
			line := lines[i+1:]
			lines = lines[:max(i, 0)]
			var ev event.Event
			if err := json.Unmarshal(line, &ev); err != nil {
				log.Printf("store: %s: skipped a damaged line: %v", path, err)
			} else if visit(ev) {
				return nil
			}
			if i < 0 {
				break
			}
		}
		win *= 4
	}
	if end == 0 {
		s.noteRead(sid, 0, 0)
	}
	return nil
}

// ObserveReads sets a function called for each read of a session's events (with its
// afterSeq and limit) and for each ScanBack that went back to the first line (as a
// read from 0 without a limit). A test uses it to see who reads whole transcripts.
// nil stops it.
func (s *Store) ObserveReads(fn func(sid string, afterSeq int64, limit int)) {
	s.observer.Store(&fn)
}

func (s *Store) noteRead(sid string, after int64, limit int) {
	if p := s.observer.Load(); p != nil && *p != nil {
		(*p)(sid, after, limit)
	}
}

// LastError returns a session's last error event. The row holds its seq, so that is
// one event read; a session whose row does not say (one from before the column) is
// looked through from its end.
func (s *Store) LastError(ctx context.Context, sid string) (event.Event, bool, error) {
	var seq int64
	if err := s.db.QueryRowContext(ctx, `SELECT last_error_seq FROM sessions WHERE id = ?`, sid).Scan(&seq); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return event.Event{}, false, ErrNotFound
		}
		return event.Event{}, false, err
	}
	if seq > 0 {
		evs, err := s.EventsLimit(ctx, sid, seq-1, 1)
		if err != nil {
			return event.Event{}, false, err
		}
		if len(evs) == 1 && evs[0].Seq == seq && evs[0].Type == event.Error {
			return evs[0], true, nil
		}
	}
	var found event.Event
	ok := false
	err := s.ScanBack(ctx, sid, func(ev event.Event) bool {
		ok = ev.Type == event.Error
		if ok {
			found = ev
		}
		return ok
	})
	return found, ok, err
}
