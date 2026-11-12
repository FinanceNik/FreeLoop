package remote

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// The phone follows sessions over server-sent events. A list's stream sends the
// whole summary each time something it shows changes, so a phone that lost its
// signal is right again at once; a session's stream sends its events in order,
// from where the phone left off.

const (
	keepAlive = 25 * time.Second
	settle    = 300 * time.Millisecond
)

// streamTo starts an event stream, or says the response cannot be one.
func streamTo(w http.ResponseWriter) (func(), bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return nil, false
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return f.Flush, true
}

// changesSummary are the events after which the lists may read differently.
func changesSummary(t event.Type) bool {
	switch t {
	case event.SessionStatus, event.SessionInfoUpdate, event.PermissionRequested, event.PermissionDecided,
		event.TurnEnded, event.Error, event.Stuck:
		return true
	}
	return false
}

func (s *Server) summaryStream(w http.ResponseWriter, r *http.Request, _ config.Device) {
	flush, ok := streamTo(w)
	if !ok {
		return
	}
	ch, cancel := s.eng.Subscribe()
	defer cancel()
	send := func() bool {
		sum, err := s.eng.PhoneSummary(r.Context())
		if err != nil {
			return false
		}
		b, _ := json.Marshal(sum)
		if _, err := fmt.Fprintf(w, "event: summary\ndata: %s\n\n", b); err != nil {
			return false
		}
		flush()
		return true
	}
	if !send() {
		return
	}
	ping := time.NewTicker(keepAlive)
	defer ping.Stop()
	var due <-chan time.Time
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if changesSummary(ev.Type) && due == nil {
				due = time.After(settle)
			}
		case <-due:
			due = nil
			if !send() {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flush()
		}
	}
}

func (s *Server) sessionStream(w http.ResponseWriter, r *http.Request, _ config.Device) {
	id, ok := s.readable(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if last, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); err == nil && last > after {
		after = last
	}
	// Subscribed before reading what came before, so nothing falls between.
	ch, cancel := s.eng.Subscribe()
	defer cancel()
	flush, ok := streamTo(w)
	if !ok {
		return
	}
	last := after
	write := func(evs []event.Event) bool {
		for _, ev := range evs {
			if ev.Seq <= last {
				continue
			}
			b, _ := json.Marshal(ev)
			if _, err := fmt.Fprintf(w, "id: %d\nevent: event\ndata: %s\n\n", ev.Seq, b); err != nil {
				return false
			}
			last = ev.Seq
		}
		flush()
		return true
	}
	// catchUp reads what the stream has not sent, as when the bus dropped some.
	catchUp := func() bool {
		for {
			evs, err := s.eng.Events(r.Context(), id, last, engine.MaxEventsPage)
			if err != nil || !write(evs) {
				return false
			}
			if len(evs) < engine.MaxEventsPage {
				return true
			}
		}
	}
	if !catchUp() {
		return
	}
	ping := time.NewTicker(keepAlive)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if ev.SID != id || ev.Seq <= last {
				continue
			}
			if ev.Seq > last+1 {
				if !catchUp() {
					return
				}
				continue
			}
			if !write([]event.Event{ev}) {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flush()
		}
	}
}
