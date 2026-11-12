package remote

import (
	"log"
	"sync"

	"github.com/FinanceNik/Freeloop/internal/engine"
)

// Telling the phone (GUI-052, issue 029). The engine's notices already follow the
// notification switches and the muted projects, so the phone gets exactly what the
// desktop would show; here they go to every phone that turned notifications on and
// has no stream open (one that does is being looked at).

// pushKind is the kind the service worker has words for, from a notice's kind.
func pushKind(noticeKind string) string {
	switch noticeKind {
	case "approval":
		return "approval"
	case "finished":
		return "done"
	case "error":
		return "failed"
	case "budget":
		return "budget"
	}
	return "other"
}

// Notify pushes a notice to the phones that should hear of it and returns when every
// push has been answered. Failures are logged, never raised: a phone that cannot be
// reached must not hold up the others.
func (s *Server) Notify(n engine.Notice) {
	if s.pushable() != nil {
		return
	}
	s.mu.Lock()
	var ids []string
	for _, d := range s.devices {
		if d.Push != nil && len(s.streams[d.ID]) == 0 {
			ids = append(ids, d.ID)
		}
	}
	s.mu.Unlock()
	if len(ids) == 0 {
		return
	}
	kind := pushKind(n.Kind)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Push(id, kind, n.SessionID); err != nil {
				log.Printf("freeloop: phone push: %v", err)
			}
		}()
	}
	wg.Wait()
}
