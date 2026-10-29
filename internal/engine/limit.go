package engine

import (
	"context"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// limited is a session's view of a provider with the provider's cap on
// simultaneous model calls (OQ-013): a call that finds every slot taken
// marks the session queued until one frees, so ten sessions on one key
// wait their turn visibly instead of all hitting the rate limit.
type limited struct {
	provider.Provider
	slots chan struct{}
	queue func(queued bool)
}

// limit wraps cp for session sid.
func (e *Engine) limit(cp *configuredProvider, sid string) provider.Provider {
	if cp.slots == nil {
		return cp.p
	}
	return limited{Provider: cp.p, slots: cp.slots, queue: func(queued bool) {
		status := session.StatusRunning
		if queued {
			status = session.StatusQueued
		}
		e.emit(sid, event.SessionStatus, event.SessionStatusData{Status: string(status)})
	}}
}

func (l limited) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	select {
	case l.slots <- struct{}{}:
	default:
		l.queue(true)
		select {
		case l.slots <- struct{}{}:
			l.queue(false)
		case <-ctx.Done():
			l.queue(false)
			return nil, ctx.Err()
		}
	}
	ch, err := l.Provider.Stream(ctx, req)
	if err != nil {
		<-l.slots
		return nil, err
	}
	out := make(chan provider.Event)
	go func() {
		defer func() { <-l.slots }()
		defer close(out)
		for ev := range ch {
			select {
			case out <- ev:
			case <-ctx.Done():
				// The caller stopped reading; let the provider finish
				// before the slot frees.
				for range ch {
				}
				return
			}
		}
	}()
	return out, nil
}
