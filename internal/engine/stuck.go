package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// A session that is not getting on (HRN-020): its account limit reached, its turn
// limit reached, the same tool failing again and again, rewinds piling up, or turns
// ending in errors. Each says so once a turn, with a profile it could continue on: the
// session's own profile's onLimit for a limit, else the user's fallback for the agent.
// Continuing is HandOff, which also ends what the old session was waiting on and
// carries its last request over.

const (
	stuckLimit        = "limit"
	stuckMaxTurns     = "max_turns"
	stuckToolFailures = "tool_failures"
	stuckRewinds      = "rewinds"
	stuckErrors       = "errors"

	// How many of each make a session stuck.
	failuresInARow = 3
	rewindsInATurn = 2
	errorTurns     = 2
)

type stuckState struct {
	tools    map[string]string // tool call ID to its tool
	denied   map[string]bool   // calls the user or a rule refused: no failure of the tool's
	failures map[string]int    // failures in a row, per tool
	rewinds  int
	errored  int
	said     map[string]bool // reasons already said this turn
	// onLimit is the profile to continue on when this turn ends.
	onLimit string
	// now is what the session is stuck on, for the attention view.
	now *event.StuckData
}

type stuckWatch struct {
	mu       sync.Mutex
	sessions map[string]*stuckState
}

func (w *stuckWatch) state(sid string) *stuckState {
	if w.sessions == nil {
		w.sessions = map[string]*stuckState{}
	}
	s := w.sessions[sid]
	if s == nil {
		s = &stuckState{tools: map[string]string{}, denied: map[string]bool{}, failures: map[string]int{}, said: map[string]bool{}}
		w.sessions[sid] = s
	}
	return s
}

// observe follows every event saved. It runs inside the store's append, so anything it
// emits goes out on its own goroutine.
func (e *Engine) observe(ev event.Event) {
	// A session settling may be an AFK run's issue or review ending (WFL-005).
	if ev.Type == event.SessionStatus {
		e.KickRuns()
	}
	var data map[string]any
	json.Unmarshal(ev.Data, &data)
	str := func(k string) string { v, _ := data[k].(string); return v }
	w := &e.stuck
	w.mu.Lock()
	s := w.state(ev.SID)
	var reason, detail, auto string
	say := func(r, d string) {
		if !s.said[r] {
			s.said[r] = true
			reason, detail = r, d
		}
	}
	switch ev.Type {
	case event.TurnStarted:
		s.rewinds, s.said, s.now = 0, map[string]bool{}, nil
	case event.ToolCall:
		s.tools[str("toolCallId")] = cmpOr(str("tool"), str("kind"), str("title"))
	case event.PermissionDecided:
		if str("outcome") == "denied" || str("outcome") == "cancelled" {
			s.denied[str("toolCallId")] = true
		}
	case event.ToolCallUpdate:
		id := str("toolCallId")
		tool := s.tools[id]
		switch str("status") {
		case "failed":
			if tool != "" && !s.denied[id] {
				s.failures[tool]++
				if s.failures[tool] == failuresInARow {
					say(stuckToolFailures, fmt.Sprintf("%s failed %d times in a row", tool, failuresInARow))
				}
			}
		case "completed":
			s.failures[tool] = 0
		}
	case event.Rewind:
		s.rewinds++
		if s.rewinds == rewindsInATurn {
			say(stuckRewinds, fmt.Sprintf("%d rewinds in one turn", rewindsInATurn))
		}
	case event.LimitUpdate:
		var d event.LimitData
		json.Unmarshal(ev.Data, &d)
		for _, win := range d.Windows {
			if win.Status == "rejected" {
				say(stuckLimit, fmt.Sprintf("%s reached; it resets %s", win.Label, time.Unix(win.ResetsAt, 0).Local().Format("Mon 15:04")))
				break
			}
		}
	case event.TurnEnded:
		stop := str("stopReason")
		if stop == "max_turns" {
			say(stuckMaxTurns, "the turn limit was reached")
		}
		if stop == "error" {
			s.errored++
			if s.errored == errorTurns {
				say(stuckErrors, fmt.Sprintf("%d turns in a row ended in an error", errorTurns))
			}
		} else {
			s.errored = 0
		}
		auto, s.onLimit = s.onLimit, ""
	case event.HandedOff:
		s.now, s.onLimit = nil, ""
	}
	w.mu.Unlock()
	if reason != "" {
		e.bg.Go(func() { e.sayStuck(ev.SID, reason, detail) })
	}
	if auto != "" {
		e.bg.Go(func() {
			if _, err := e.HandOff(context.Background(), ev.SID, auto, stuckLimit, true); err != nil {
				log.Printf("engine: continue %s on %s: %v", ev.SID, auto, err)
				e.emit(ev.SID, event.Notice, event.NoticeData{Message: "Could not continue on the profile set for a reached limit: " + err.Error()})
			}
		})
	}
}

func cmpOr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// sayStuck records why a session is stuck and the profile it could continue on.
func (e *Engine) sayStuck(sid, reason, detail string) {
	ctx := context.Background()
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return
	}
	d := event.StuckData{Reason: reason, Detail: detail}
	project := e.sessionProject(ctx, ses)
	if own, err := e.profileFor(project, cmpOr(ses.ProfileID, config.DefaultProfileID)); err == nil && reason == stuckLimit && own.OnLimit != "" {
		if p, err := e.profileFor(project, own.OnLimit); err == nil {
			d.ProfileID, d.ProfileName, d.Auto = p.ID, p.Name, true
		}
	}
	if d.ProfileID == "" {
		if id := e.Config().Fallbacks[cmpOr(ses.HarnessID, "builtin")]; id != "" && id != ses.ProfileID {
			if p, err := e.profileFor(project, id); err == nil {
				d.ProfileID, d.ProfileName = p.ID, p.Name
			}
		}
	}
	e.stuck.mu.Lock()
	s := e.stuck.state(sid)
	s.now = &d
	if d.Auto {
		s.onLimit = d.ProfileID
	}
	e.stuck.mu.Unlock()
	e.emit(sid, event.Stuck, d)
}

// stuckOn is what a session is stuck on now, or nil.
func (e *Engine) stuckOn(sid string) *event.StuckData {
	e.stuck.mu.Lock()
	defer e.stuck.mu.Unlock()
	if s := e.stuck.sessions[sid]; s != nil {
		return s.now
	}
	return nil
}

// HandOffResult is the session a handoff continued in, and the message that carries
// the old one's last request over.
type HandOffResult struct {
	Session  session.Session `json:"session"`
	Continue string          `json:"continue"`
}

// HandOff continues a session on another profile in one step (HRN-020): it stops the
// old session's turn and ends what it was waiting on, which nobody will answer there
// now, continues with HRN-012's handoff, links the two, and gives the message that
// carries the last request over. auto sends that message too.
func (e *Engine) HandOff(ctx context.Context, sid, profileID, reason string, auto bool) (HandOffResult, error) {
	e.stopTurn(sid, "Stopped, to continue in another session.")
	e.broker.CancelSession(sid, "the session continued in another")
	ses, err := e.SwitchProfile(ctx, sid, profileID)
	if err != nil {
		return HandOffResult{}, err
	}
	name := profileID
	if src, err := e.store.GetSession(ctx, sid); err == nil {
		if p, err := e.profileFor(e.sessionProject(ctx, src), profileID); err == nil {
			name = p.Name
		}
	}
	e.emit(sid, event.HandedOff, event.HandedOffData{ToSessionID: ses.ID, ToTitle: ses.Title, ProfileName: name, Reason: reason})
	res := HandOffResult{Session: ses}
	if events, err := e.store.Events(ctx, sid, 0); err == nil {
		if last := lastRequest(events); last != "" {
			res.Continue = "Continue the work handed over above. The last request in the previous session was:\n\n" + last
		}
	}
	if auto && res.Continue != "" {
		if _, err := e.Prompt(ctx, ses.ID, res.Continue); err != nil {
			return res, err
		}
	}
	return res, nil
}

// lastRequest is the text of the last message the user sent.
func lastRequest(events []event.Event) string {
	var cur, last strings.Builder
	keep := func() {
		if strings.TrimSpace(cur.String()) != "" {
			last.Reset()
			last.WriteString(cur.String())
		}
		cur.Reset()
	}
	for _, ev := range events {
		switch ev.Type {
		case event.TurnStarted:
			keep()
		case event.UserMessageChunk:
			var d event.MessageChunk
			if json.Unmarshal(ev.Data, &d) == nil && d.Content.Type == "text" {
				cur.WriteString(d.Content.Text)
			}
		}
	}
	keep()
	return strings.TrimSpace(last.String())
}
