// Package engine is the transport-agnostic API the GUI and, from v1, the
// JSON-RPC socket are thin adapters over. Requests and responses are plain
// types; live events arrive through Subscribe.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// ErrNotFound is returned for an unknown session.
var ErrNotFound = store.ErrNotFound

// Options configures Open.
type Options struct {
	DataDir string
	Secrets secrets.Store
}

// Engine is safe for concurrent use.
type Engine struct {
	store   *store.Store
	bus     *bus
	secrets secrets.Store
	cfg     config.Config
	issues  []config.Issue
}

// Open loads configuration and opens the store in opts.DataDir. Config
// problems do not fail Open; they are reported by ConfigIssues.
func Open(opts Options) (*Engine, error) {
	if opts.Secrets == nil {
		return nil, errors.New("engine: Secrets is required")
	}
	cfg, issues, err := config.Load(opts.DataDir)
	if err != nil {
		return nil, err
	}
	b := newBus()
	st, err := store.Open(store.Options{Dir: opts.DataDir, OnAppend: b.publish})
	if err != nil {
		return nil, err
	}
	return &Engine{store: st, bus: b, secrets: opts.Secrets, cfg: cfg, issues: issues}, nil
}

// Close closes every subscription and the store.
func (e *Engine) Close() error {
	e.bus.close()
	return e.store.Close()
}

// Config returns the loaded configuration.
func (e *Engine) Config() config.Config { return e.cfg }

// ConfigIssues returns the problems found while loading configuration.
func (e *Engine) ConfigIssues() []config.Issue {
	return append([]config.Issue{}, e.issues...)
}

// Secrets returns the secret store.
func (e *Engine) Secrets() secrets.Store { return e.secrets }

// ListSessions returns all sessions, most recently updated first.
func (e *Engine) ListSessions(ctx context.Context) ([]session.Session, error) {
	return e.store.ListSessions(ctx)
}

// CreateSessionRequest is the input of CreateSession.
type CreateSessionRequest struct {
	Title string `json:"title"`
}

// DefaultHarness is the harness every session uses until profiles (HRN-003).
const DefaultHarness = "builtin"

// CreateSession creates an empty session and records its initial status as
// the first transcript event.
func (e *Engine) CreateSession(ctx context.Context, req CreateSessionRequest) (session.Session, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "New session"
	}
	ses, err := e.store.CreateSession(ctx, title, DefaultHarness)
	if err != nil {
		return session.Session{}, err
	}
	data, _ := json.Marshal(event.SessionStatusData{Status: string(session.StatusIdle)})
	ev, err := e.store.Append(ctx, ses.ID, event.SessionStatus, data)
	if err != nil {
		return session.Session{}, err
	}
	ses.LastSeq, ses.UpdatedAt = ev.Seq, ev.TS
	return ses, nil
}

// AppendEventRequest is the input of AppendEvent.
type AppendEventRequest struct {
	SessionID string          `json:"sessionId"`
	Type      event.Type      `json:"type"`
	Data      json.RawMessage `json:"data"`
}

// AppendEvent persists an event to a session's transcript and then publishes
// it to subscribers. Harnesses are the normal producers from M1 on; in M0 the
// GUI uses it to prove the path from engine to transcript to UI.
func (e *Engine) AppendEvent(ctx context.Context, req AppendEventRequest) (event.Event, error) {
	if req.SessionID == "" {
		return event.Event{}, errors.New("engine: sessionId is required")
	}
	ev, err := e.store.Append(ctx, req.SessionID, req.Type, req.Data)
	if err != nil {
		return event.Event{}, fmt.Errorf("engine: append to %s: %w", req.SessionID, err)
	}
	return ev, nil
}

// Events returns a session's events with seq greater than afterSeq. The GUI
// uses it for the initial load and to fill a gap in the live stream.
func (e *Engine) Events(ctx context.Context, sessionID string, afterSeq int64) ([]event.Event, error) {
	return e.store.Events(ctx, sessionID, afterSeq)
}

// Subscribe returns a channel of every event persisted from now on, in seq
// order per session, and a function that ends the subscription. A subscriber
// that falls behind loses events rather than slowing the engine; it notices
// the seq gap and catches up with Events.
func (e *Engine) Subscribe() (<-chan event.Event, func()) {
	return e.bus.subscribe(1024)
}
