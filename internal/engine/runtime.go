package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// runtime is a live session: its harness session and the prompt it is
// running, if any. Prompts sent while one runs wait in a queue and start at
// the next turn boundary (GUI-004).
type runtime struct {
	hs  harness.Session
	sid string
	env environment.Environment

	// Git state, refreshed after tool calls and turns (SES-006).
	gitMu   sync.Mutex
	base    string
	lastGit event.GitStateData

	mu     sync.Mutex
	busy   bool
	cancel context.CancelFunc
	queue  []string
	done   chan struct{} // closed when the current prompt and queue finish
}

func (r *runtime) stop() {
	r.mu.Lock()
	r.queue = nil
	if r.cancel != nil {
		r.cancel()
	}
	done := r.done
	r.mu.Unlock()
	if done != nil {
		<-done
	}
}

// runtimeFor returns the live session for sid, starting it on first use:
// environment, provider, restored history and the built-in harness.
func (e *Engine) runtimeFor(ctx context.Context, sid string) (*runtime, error) {
	e.mu.Lock()
	if r, ok := e.runtimes[sid]; ok {
		e.mu.Unlock()
		return r, nil
	}
	e.mu.Unlock()

	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	if ses.HarnessID != builtin.ID {
		return nil, fmt.Errorf("session %s uses harness %q, which this build cannot run", sid, ses.HarnessID)
	}
	p, err := e.provider(ses.ProviderID)
	if err != nil {
		return nil, err
	}
	if err := e.requireTools(ctx, ses.ProviderID, ses.Model); err != nil {
		return nil, err
	}
	envID := ses.EnvironmentID
	if envID == "" {
		envID = DefaultEnvironmentID
	}
	envCfg, err := e.environment(envID)
	if err != nil {
		return nil, err
	}
	vars, err := e.resolveEnv(envCfg)
	if err != nil {
		return nil, err
	}
	if !environment.IsDir(ses.ProjectRoot) {
		return nil, fmt.Errorf("the project folder %s is missing; relink the project", ses.ProjectRoot)
	}
	env, err := e.localEnv(ses.ProjectRoot, vars)
	if err != nil {
		return nil, err
	}
	events, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return nil, err
	}
	e.rules.restore(sid, events)
	r := &runtime{env: env, base: ses.BaseCommit, sid: sid}
	for _, ev := range events {
		if ev.Type == event.GitState {
			json.Unmarshal(ev.Data, &r.lastGit)
		}
	}
	hs, err := builtin.Harness{}.NewSession(ctx, harness.Params{
		SessionID:     sid,
		Env:           env,
		Broker:        e.broker,
		Provider:      p.p,
		Model:         ses.Model,
		Effort:        "high",
		Mode:          permission.Mode(ses.Mode),
		History:       builtin.Restore(events),
		DisabledTools: envCfg.DisabledTools,
		Emit: func(typ event.Type, data any) {
			e.emit(sid, typ, data)
			if changesFiles(typ, data) {
				e.bg.Go(func() { e.refreshGit(r) })
			}
		},
	})
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if existing, ok := e.runtimes[sid]; ok { // another caller won the race
		return existing, nil
	}
	r.hs = hs
	e.bg.Go(func() { e.refreshGit(r) })
	e.runtimes[sid] = r
	return r, nil
}

// changesFiles reports events after which the checkout may have changed: a
// finished tool call or a finished turn.
func changesFiles(typ event.Type, data any) bool {
	if typ == event.TurnEnded {
		return true
	}
	if typ != event.ToolCallUpdate {
		return false
	}
	m, ok := data.(map[string]any)
	return ok && m["status"] == "completed"
}

// refreshGit records the session's git state when it changed.
func (e *Engine) refreshGit(r *runtime) {
	r.gitMu.Lock()
	defer r.gitMu.Unlock()
	st := gitState(context.Background(), r.env)
	st.BaseCommit = r.base
	if st == r.lastGit || (!st.Repo && !r.lastGit.Repo) {
		return
	}
	r.lastGit = st
	e.emit(r.sid, event.GitState, st)
}

// PromptResult says whether the prompt started or waits for the current one.
type PromptResult struct {
	Queued bool `json:"queued"`
}

// Prompt sends a user message to a session. It returns at once; the turn
// runs in the background and reports through events.
func (e *Engine) Prompt(ctx context.Context, sid, text string) (PromptResult, error) {
	if text == "" {
		return PromptResult{}, errors.New("empty message")
	}
	r, err := e.runtimeFor(ctx, sid)
	if err != nil {
		e.emit(sid, event.Error, event.ErrorData{Message: err.Error()})
		return PromptResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy {
		r.queue = append(r.queue, text)
		return PromptResult{Queued: true}, nil
	}
	r.busy = true
	r.done = make(chan struct{})
	go e.run(r, text)
	return PromptResult{}, nil
}

func (e *Engine) run(r *runtime, text string) {
	for {
		ctx, cancel := context.WithCancel(context.Background())
		r.mu.Lock()
		r.cancel = cancel
		r.mu.Unlock()
		if _, err := r.hs.Prompt(ctx, text); err != nil {
			log.Printf("engine: prompt: %v", err)
		}
		cancel()

		r.mu.Lock()
		r.cancel = nil
		if len(r.queue) == 0 {
			r.busy = false
			close(r.done)
			r.mu.Unlock()
			return
		}
		text, r.queue = r.queue[0], r.queue[1:]
		r.mu.Unlock()
	}
}

// Cancel stops a session's running turn and drops its queued prompts.
func (e *Engine) Cancel(sid string) error {
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if !ok {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queue = nil
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}

// SetMode changes a session's permission mode; it applies from the next tool
// call. Bypass needs permissions.allowBypass in config.yaml (SEC-001).
func (e *Engine) SetMode(ctx context.Context, sid, mode string) error {
	m := permission.Mode(mode)
	if !m.Valid() {
		return fmt.Errorf("unknown mode %q", mode)
	}
	if m == permission.ModeBypass && !e.cfg.Permissions.AllowBypass {
		return errors.New("bypass mode is off; set permissions.allowBypass: true in config.yaml to allow it")
	}
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if ok {
		r.hs.SetMode(m)
		return nil
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	if ses.Mode != mode {
		e.emit(sid, event.CurrentModeUpdate, event.CurrentModeData{CurrentModeID: mode})
	}
	return nil
}

// DecideRequest answers a pending permission request.
type DecideRequest struct {
	RequestID string `json:"requestId"`
	OptionID  string `json:"optionId"`
	Reason    string `json:"reason"`
	// Scope for "Allow always": session (default) or global.
	Scope string `json:"scope"`
}

// Decide answers a pending permission request.
func (e *Engine) Decide(req DecideRequest) error {
	return e.broker.Decide(req.RequestID, req.OptionID, req.Reason, permission.Scope(req.Scope))
}

// PendingPermissions returns every request waiting for the user, across
// sessions, oldest first.
func (e *Engine) PendingPermissions() []permission.Request {
	return e.broker.Pending()
}

// recover closes out sessions the app left mid-turn: pending requests are
// recorded as cancelled, the open turn as ended, and the session as idle.
func (e *Engine) recover(ctx context.Context) {
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		log.Printf("engine: recover: %v", err)
		return
	}
	for _, ses := range list {
		if ses.Status != session.StatusRunning && ses.Status != session.StatusWaitingPermission {
			continue
		}
		events, err := e.store.Events(ctx, ses.ID, 0)
		if err != nil {
			continue
		}
		open := map[string]string{} // requestId -> toolCallId
		inTurn := false
		for _, ev := range events {
			var d struct {
				RequestID  string `json:"requestId"`
				ToolCallID string `json:"toolCallId"`
			}
			json.Unmarshal(ev.Data, &d)
			switch ev.Type {
			case event.PermissionRequested:
				open[d.RequestID] = d.ToolCallID
			case event.PermissionDecided:
				delete(open, d.RequestID)
			case event.TurnStarted:
				inTurn = true
			case event.TurnEnded:
				inTurn = false
			}
		}
		for id, call := range open {
			e.emit(ses.ID, event.PermissionDecided, permission.Decision{
				RequestID: id, ToolCallID: call, Outcome: permission.Cancelled, Actor: permission.ActorUser, Reason: "the app quit",
			})
		}
		if inTurn {
			e.emit(ses.ID, event.TurnEnded, event.TurnEndedData{StopReason: string(harness.StopCancelled)})
		}
		e.emit(ses.ID, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusIdle)})
	}
}
