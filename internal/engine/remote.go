package engine

import (
	"context"
	"slices"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Sessions from the phone (GUI-052): what the engine offers the phone server in
// internal/remote, which answers on 127.0.0.1 for the paired phones.

// RemoteState is whether the phone server should run, and where.
type RemoteState struct {
	On   bool   `json:"on"`
	Port int    `json:"port"`
	Host string `json:"host"`
	// Why says why it is off although turned on: waiting for approval, or
	// local-only mode.
	Why string `json:"why,omitempty"`
}

// RemoteState says whether the phone server should run. Turned on in a file, it
// waits for the user's approval; local-only mode turns it off, since transcripts
// would leave this computer over the network.
func (e *Engine) RemoteState() RemoteState {
	c := e.Config()
	st := RemoteState{Port: c.Remote.RemotePort(), Host: c.Remote.Host}
	switch {
	case !c.Remote.Enabled:
	case c.Network.LocalOnly || netguard.LocalOnly():
		st.Why = "local-only"
	case !e.commandApproved(config.RemoteFingerprint()):
		st.Why = "approval"
	default:
		st.On = true
	}
	return st
}

// PhoneSession is a session as the phone lists it.
type PhoneSession struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	ProjectName string    `json:"projectName"`
	Updated     time.Time `json:"updated"`
}

// PhoneRequest is a pending request as the phone shows it, with the options it may
// answer with from there: allow or deny once, for a plain request.
type PhoneRequest struct {
	permission.Request
	// OnLaptop says it can only be answered on this computer: a form, a plan.
	OnLaptop bool `json:"onLaptop"`
}

// PhoneAttention is a session that needs the user, as the phone shows it.
type PhoneAttention struct {
	SessionID string         `json:"sessionId"`
	Requests  []PhoneRequest `json:"requests"`
	Error     string         `json:"error,omitempty"`
	Stuck     string         `json:"stuck,omitempty"`
}

// PhoneSummary is what the phone's lists show.
type PhoneSummary struct {
	Sessions  []PhoneSession   `json:"sessions"`
	Attention []PhoneAttention `json:"attention"`
}

// PhoneSummary lists the sessions not archived, newest first, and what needs the
// user. QA of AFK runs stays on the computer.
func (e *Engine) PhoneSummary(ctx context.Context) (PhoneSummary, error) {
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		return PhoneSummary{}, err
	}
	names := map[string]string{}
	if projects, err := e.store.ListProjects(ctx); err == nil {
		for _, p := range projects {
			names[p.ID] = p.Name
		}
	}
	out := PhoneSummary{Sessions: []PhoneSession{}, Attention: []PhoneAttention{}}
	for _, s := range list {
		if s.Status == session.StatusArchived || !phoneVisible(s) {
			continue
		}
		out.Sessions = append(out.Sessions, PhoneSession{ID: s.ID, Title: s.Title, Status: string(s.Status), ProjectName: names[s.ProjectID], Updated: s.UpdatedAt})
	}
	slices.SortStableFunc(out.Sessions, func(a, b PhoneSession) int { return b.Updated.Compare(a.Updated) })
	items, err := e.Attention(ctx)
	if err != nil {
		return PhoneSummary{}, err
	}
	for _, it := range items {
		if len(it.Requests) == 0 && it.Error == "" && it.Stuck == nil {
			continue
		}
		a := PhoneAttention{SessionID: it.Session.ID, Error: it.Error, Requests: []PhoneRequest{}}
		if it.Stuck != nil {
			a.Stuck = it.Stuck.Detail
		}
		for _, r := range it.Requests {
			a.Requests = append(a.Requests, PhoneRequest{Request: r, OnLaptop: r.Form != nil || r.Kind == permission.KindSwitchMode})
		}
		out.Attention = append(out.Attention, a)
	}
	return out, nil
}

// phoneVisible reports a session the phone may read: one of this computer's, not
// a subagent's on its own (its parent shows its work).
func phoneVisible(s session.Session) bool { return s.ParentID == "" }
