package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// limited is a session's view of a provider with the provider's cap on
// simultaneous model calls (OQ-013): a call that finds every slot taken
// marks the session queued until one frees, so ten sessions on one key
// wait their turn visibly instead of all hitting the rate limit.
//
// It is also where every model call for a session passes, so it records what each
// kind of call carries besides the conversation (NFR-PRIV-003).
type limited struct {
	provider.Provider
	slots  chan struct{}
	queue  func(queued bool)
	record func(ctx context.Context, req provider.ChatRequest)
}

// limit wraps cp for session sid; with no session, nothing is recorded.
func (e *Engine) limit(cp *configuredProvider, sid string) provider.Provider {
	l := limited{Provider: cp.p, slots: cp.slots, queue: func(queued bool) {
		status := session.StatusRunning
		if queued {
			status = session.StatusQueued
		}
		e.emit(sid, event.SessionStatus, event.SessionStatusData{Status: string(status)})
	}}
	if sid != "" {
		l.record = func(ctx context.Context, req provider.ChatRequest) { e.recordModelContext(ctx, sid, cp.cfg.ID, req) }
	}
	return l
}

func (l limited) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	if l.record != nil {
		l.record(ctx, req)
	}
	if l.slots == nil {
		return l.Provider.Stream(ctx, req)
	}
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

// recordModelContext writes what req carries besides the conversation to the session's
// transcript, when it differs from what was last written for that kind of call
// (NFR-PRIV-003). The store redacts it as it does every event.
func (e *Engine) recordModelContext(ctx context.Context, sid, providerID string, req provider.ChatRequest) {
	purpose, sent := provider.PurposeOf(ctx)
	d := event.ModelContextData{Purpose: purpose, ProviderID: providerID, Model: req.Model, Effort: req.Effort, System: req.System, Tools: []event.ToolDef{}, Sent: sent}
	for _, t := range req.Tools {
		schema, _ := json.Marshal(t.InputSchema)
		d.Tools = append(d.Tools, event.ToolDef{Name: t.Name, Description: t.Description, InputSchema: schema})
	}
	// Sent carries sizes that change with every call; what it says does not change
	// what the model is told, so it is left out of the comparison.
	key, _ := json.Marshal(struct {
		P, M, E, S string
		T          []event.ToolDef
	}{purpose, req.Model, req.Effort, req.System, d.Tools})
	sum := sha256.Sum256(append([]byte(providerID+"\x00"), key...))
	e.contextMu.Lock()
	if e.contextSeen == nil {
		e.contextSeen = map[string][32]byte{}
	}
	same := e.contextSeen[sid+"\x00"+purpose] == sum
	e.contextSeen[sid+"\x00"+purpose] = sum
	e.contextMu.Unlock()
	if !same {
		e.emit(sid, event.ModelContext, d)
	}
}

// sizeText is a size of text in words: "12 KB".
func sizeText(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d bytes", n)
	}
	return fmt.Sprintf("%d KB", (n+512)/1024)
}
