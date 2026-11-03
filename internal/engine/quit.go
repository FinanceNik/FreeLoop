package engine

import (
	"context"

	"github.com/FinanceNik/Freeloop/internal/session"
)

// QuitCheck is what quitting the app now would interrupt (GUI-027).
type QuitCheck struct {
	// Working is sessions with a running harness in the middle of something.
	Working int `json:"working"`
	// Waiting is approvals nobody has answered.
	Waiting int `json:"waiting"`
}

// Empty says nothing would be interrupted.
func (c QuitCheck) Empty() bool { return c.Working == 0 && c.Waiting == 0 }

// CheckQuit reports what quitting would interrupt. A status alone is not enough:
// one left behind by a crash has nothing running behind it.
func (e *Engine) CheckQuit(ctx context.Context) QuitCheck {
	c := QuitCheck{Waiting: len(e.broker.Pending())}
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		return c
	}
	for _, s := range list {
		switch s.Status {
		case session.StatusRunning, session.StatusQueued, session.StatusWaitingPermission:
			e.mu.Lock()
			_, started := e.runtimes[s.ID]
			e.mu.Unlock()
			if started {
				c.Working++
			}
		}
	}
	return c
}
