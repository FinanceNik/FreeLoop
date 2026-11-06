package engine

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// AttentionItem is one session that needs the user (GUI-002): it waits for
// approval or input, or its last turn failed.
type AttentionItem struct {
	Session     session.Session `json:"session"`
	ProjectName string          `json:"projectName"`
	// Requests are the session's pending permission requests, oldest first.
	Requests []permission.Request `json:"requests"`
	// Error is the last error message when the session is in error.
	Error string `json:"error,omitempty"`
	// Stuck says why the session is not getting on and where it could continue (HRN-020).
	Stuck *event.StuckData `json:"stuck,omitempty"`
}

// Attention lists every session that needs the user, across projects,
// oldest request first; sessions in error come after those waiting.
func (e *Engine) Attention(ctx context.Context) ([]AttentionItem, error) {
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	projects, err := e.store.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, p := range projects {
		names[p.ID] = p.Name
	}
	pending := map[string][]permission.Request{}
	for _, r := range e.broker.Pending() {
		// The transcript shows these redacted; so does this.
		r.Title, r.Command, r.Input = e.redactText(r.Title), e.redactText(r.Command), e.redact(r.Input)
		pending[r.SessionID] = append(pending[r.SessionID], r)
	}
	out := []AttentionItem{}
	for _, ses := range list {
		reqs := pending[ses.ID]
		stuck := e.stuckOn(ses.ID)
		if len(reqs) == 0 && stuck == nil && ses.Status != session.StatusError && ses.Status != session.StatusWaitingInput {
			continue
		}
		it := AttentionItem{Session: ses, ProjectName: names[ses.ProjectID], Requests: reqs, Stuck: stuck}
		if it.Requests == nil {
			it.Requests = []permission.Request{}
		}
		if ses.Status == session.StatusError {
			it.Error = e.lastError(ctx, ses.ID)
		}
		out = append(out, it)
	}
	slices.SortStableFunc(out, func(a, b AttentionItem) int {
		switch {
		case len(a.Requests) > 0 && len(b.Requests) > 0:
			return a.Requests[0].Created.Compare(b.Requests[0].Created)
		case len(a.Requests) > 0:
			return -1
		case len(b.Requests) > 0:
			return 1
		}
		return b.Session.UpdatedAt.Compare(a.Session.UpdatedAt)
	})
	return out, nil
}

// lastError returns the message of a session's latest error event.
func (e *Engine) lastError(ctx context.Context, sid string) string {
	events, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return err.Error()
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == event.Error {
			var d event.ErrorData
			json.Unmarshal(events[i].Data, &d)
			return d.Message
		}
	}
	return ""
}

// DismissError takes a session out of the error status; the error stays in
// its transcript.
func (e *Engine) DismissError(ctx context.Context, sid string) error {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	if ses.Status != session.StatusError {
		return errors.New("the session is not in error")
	}
	e.emit(sid, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusIdle)})
	return nil
}
