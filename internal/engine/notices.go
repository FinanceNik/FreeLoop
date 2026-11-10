package engine

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Notices (GUI-008) are what the app tells the user about while it is in
// the background: a session needs an approval, a turn failed, or a turn
// finished. The engine decides what is worth a notice; the desktop layer
// decides whether to show it (only while the window is not focused) and how.

// Notice is one thing to tell the user. Title and Body are in English; the desktop
// layer writes them in the interface's language from what the notice says happened
// (GUI-048), and shows these for a kind it does not know.
type Notice struct {
	SessionID string `json:"sessionId"`
	ProjectID string `json:"projectId"`
	// Kind is approval, error, finished or budget.
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body"`
	// Session is the session's title, and Detail what the approval asks for or the
	// error says, as given.
	Session string `json:"session,omitempty"`
	Detail  string `json:"detail,omitempty"`
	// StopReason is a finished turn's.
	StopReason string `json:"stopReason,omitempty"`
	// Budget is the budget a budget notice is about.
	Budget *BudgetState `json:"budget,omitempty"`
}

var finishedLabel = map[string]string{
	"end_turn":   "Finished",
	"max_tokens": "Stopped at the output limit",
	"max_turns":  "Stopped at the turn limit",
	"refusal":    "The model declined the request",
}

// noticeFor returns the notice ev calls for, if any. Notifications turned
// off in config.yaml and muted projects get none.
func (e *Engine) noticeFor(ctx context.Context, ev event.Event) (Notice, bool) {
	var kind, body, reason string
	switch ev.Type {
	case event.PermissionRequested:
		var d struct{ Title, Command string }
		json.Unmarshal(ev.Data, &d)
		kind, body = "approval", d.Title
		if d.Command != "" {
			body = d.Command
		}
	case event.Error:
		var d event.ErrorData
		json.Unmarshal(ev.Data, &d)
		kind, body = "error", d.Message
	case event.TurnEnded:
		var d event.TurnEndedData
		json.Unmarshal(ev.Data, &d)
		label, ok := finishedLabel[d.StopReason]
		if !ok {
			return Notice{}, false // cancelled by the user, or an error noticed already
		}
		kind, body, reason = "finished", label, d.StopReason
	default:
		return Notice{}, false
	}
	cfg := e.Config().Notifications
	if !cfg.Enabled || (kind == "finished" && !cfg.Finished) {
		return Notice{}, false
	}
	ses, err := e.store.GetSession(ctx, ev.SID)
	if err != nil {
		return Notice{}, false
	}
	if p, err := e.store.GetProject(ctx, ses.ProjectID); err == nil && p.Muted {
		return Notice{}, false
	}
	title := map[string]string{"approval": "Approval needed", "error": "Session failed", "finished": "Session finished"}[kind]
	detail := truncate(strings.TrimSpace(body), 200)
	if kind == "finished" {
		detail = ""
	}
	return Notice{
		SessionID: ses.ID, ProjectID: ses.ProjectID, Kind: kind,
		Title: title + ": " + ses.Title, Body: truncate(strings.TrimSpace(body), 200),
		Session: ses.Title, Detail: detail, StopReason: reason,
	}, true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}

// notify turns the events of every session into notices until the bus
// closes.
func (e *Engine) notify(events <-chan event.Event, send func(Notice)) {
	defer e.notifying.Done()
	for ev := range events {
		if n, ok := e.noticeFor(context.Background(), ev); ok {
			send(n)
		}
	}
}

// SetProjectMuted turns a project's notifications off or on.
func (e *Engine) SetProjectMuted(ctx context.Context, projectID string, muted bool) error {
	p, err := e.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	p.Muted = muted
	return e.store.UpdateProject(ctx, p)
}

// AttentionCount is the number the dock badge shows: each waiting request,
// and each session in error or waiting for a reply without one.
func (e *Engine) AttentionCount(ctx context.Context) int {
	items, err := e.Attention(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, it := range items {
		n += max(len(it.Requests), 1)
	}
	return n
}
