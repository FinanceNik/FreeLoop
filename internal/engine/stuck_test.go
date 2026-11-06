package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// stuckSession is a built-in session on the scripted provider, with profiles "main"
// and "fallback" saved in the app.
func stuckSession(t *testing.T, mainYAML string) (*Engine, session.Session) {
	t.Helper()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	withScript(e, "s", scripted.Step{Text: "one"}, scripted.Step{Text: "two"}, scripted.Step{Text: "three"})
	if err := e.SaveProfileYAML("fallback", "name: Fallback\nprovider: s\nmodel: m\n"); err != nil {
		t.Fatal(err)
	}
	if err := e.SaveProfileYAML("main", "name: Main\nprovider: s\nmodel: m\n"+mainYAML); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(context.Background(), CreateSessionRequest{ProjectRoot: t.TempDir(), ProfileID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	return e, ses
}

// waitEvent returns the first event of typ in sid's transcript, waiting a little.
func waitEvent(t *testing.T, e *Engine, sid string, typ event.Type) *event.Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs, _ := e.Events(context.Background(), sid, 0)
		for i := range evs {
			if evs[i].Type == typ {
				return &evs[i]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

func stuckOf(t *testing.T, e *Engine, sid string) event.StuckData {
	t.Helper()
	ev := waitEvent(t, e, sid, event.Stuck)
	if ev == nil {
		t.Fatal("no stuck event")
	}
	var d event.StuckData
	json.Unmarshal(ev.Data, &d)
	return d
}

func failCall(e *Engine, sid, id, tool string) {
	e.emit(sid, event.ToolCall, map[string]any{"toolCallId": id, "tool": tool, "status": "pending"})
	e.emit(sid, event.ToolCallUpdate, map[string]any{"toolCallId": id, "status": "failed"})
}

// HRN-020: the same tool failing three times in a row makes a session stuck, and the
// user's fallback for the agent is offered; refused calls are not the tool failing.
func TestStuckOnRepeatedFailures(t *testing.T) {
	e, ses := stuckSession(t, "")
	cfg := e.Config()
	cfg.Fallbacks = map[string]string{"builtin": "fallback"}
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	e.emit(ses.ID, event.TurnStarted, struct{}{})
	// A refused call does not count.
	e.emit(ses.ID, event.ToolCall, map[string]any{"toolCallId": "r", "tool": "shell"})
	e.emit(ses.ID, event.PermissionDecided, map[string]any{"toolCallId": "r", "outcome": "denied"})
	e.emit(ses.ID, event.ToolCallUpdate, map[string]any{"toolCallId": "r", "status": "failed"})
	failCall(e, ses.ID, "a", "shell")
	failCall(e, ses.ID, "b", "shell")
	if ev := waitEvent(t, e, ses.ID, event.Stuck); ev != nil {
		t.Fatalf("stuck after two failures and a refusal: %s", ev.Data)
	}
	failCall(e, ses.ID, "c", "shell")
	d := stuckOf(t, e, ses.ID)
	if d.Reason != stuckToolFailures || d.ProfileID != "fallback" || d.ProfileName != "Fallback" || d.Auto {
		t.Errorf("%+v", d)
	}
	if got := e.stuckOn(ses.ID); got == nil || got.Reason != stuckToolFailures {
		t.Errorf("the attention view does not see it: %+v", got)
	}
	items, _ := e.Attention(context.Background())
	if len(items) != 1 || items[0].Stuck == nil {
		t.Errorf("attention: %+v", items)
	}
	// A new turn starts afresh.
	e.emit(ses.ID, event.TurnStarted, struct{}{})
	if got := e.stuckOn(ses.ID); got != nil {
		t.Errorf("still stuck after a new turn: %+v", got)
	}
}

// HRN-020: rewinds, the turn limit and turns ending in errors.
func TestStuckOnRewindsTurnLimitAndErrors(t *testing.T) {
	for _, c := range []struct {
		reason string
		events func(e *Engine, sid string)
	}{
		{stuckRewinds, func(e *Engine, sid string) {
			e.emit(sid, event.Rewind, map[string]any{"checkpointSeq": 1})
			e.emit(sid, event.Rewind, map[string]any{"checkpointSeq": 2})
		}},
		{stuckMaxTurns, func(e *Engine, sid string) {
			e.emit(sid, event.TurnEnded, map[string]any{"stopReason": "max_turns"})
		}},
		{stuckErrors, func(e *Engine, sid string) {
			e.emit(sid, event.TurnEnded, map[string]any{"stopReason": "error"})
			e.emit(sid, event.TurnStarted, struct{}{})
			e.emit(sid, event.TurnEnded, map[string]any{"stopReason": "error"})
		}},
	} {
		t.Run(c.reason, func(t *testing.T) {
			e, ses := stuckSession(t, "")
			e.emit(ses.ID, event.TurnStarted, struct{}{})
			c.events(e, ses.ID)
			if d := stuckOf(t, e, ses.ID); d.Reason != c.reason || d.ProfileID != "" {
				t.Errorf("%+v", d)
			}
		})
	}
}

// HRN-020: a profile's onLimit continues there by itself when the limit is reached,
// once the turn ends, and says so in both sessions.
func TestOnLimitContinuesByItself(t *testing.T) {
	ctx := context.Background()
	e, ses := stuckSession(t, "onLimit: fallback\n")
	ch, cancel := e.Subscribe()
	defer cancel()
	e.Prompt(ctx, ses.ID, "fix the login page")
	waitIdle(t, ch, ses.ID)
	e.emit(ses.ID, event.TurnStarted, struct{}{})
	e.emit(ses.ID, event.LimitUpdate, event.LimitData{Source: "Claude Code", Windows: []event.LimitWindow{{ID: "five_hour", Label: "Session limit (5 hours)", UsedPercent: 100, ResetsAt: time.Now().Add(time.Hour).Unix(), Status: "rejected"}}})
	if d := stuckOf(t, e, ses.ID); d.Reason != stuckLimit || !d.Auto || d.ProfileID != "fallback" {
		t.Fatalf("%+v", d)
	}
	e.emit(ses.ID, event.TurnEnded, map[string]any{"stopReason": "end_turn"})
	off := waitEvent(t, e, ses.ID, event.HandedOff)
	if off == nil {
		t.Fatal("it did not continue")
	}
	var d event.HandedOffData
	json.Unmarshal(off.Data, &d)
	next, err := e.store.GetSession(ctx, d.ToSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if next.ProfileID != "fallback" || d.Reason != stuckLimit {
		t.Errorf("continued on %+v, %+v", next, d)
	}
	if waitEvent(t, e, next.ID, event.Handoff) == nil {
		t.Error("the new session has no handoff")
	}
	// The last request goes on in the new session, by itself.
	deadline := time.Now().Add(5 * time.Second)
	for {
		evs, _ := e.Events(ctx, next.ID, 0)
		if strings.Contains(eventsText(evs), "fix the login page") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the last request was not carried over")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func eventsText(evs []event.Event) string {
	var b strings.Builder
	for _, ev := range evs {
		b.Write(ev.Data)
	}
	return b.String()
}

// HRN-020: continuing by hand ends what the old session waited on and gives the
// message that carries its last request over, without sending it.
func TestHandOffByHand(t *testing.T) {
	ctx := context.Background()
	e, ses := stuckSession(t, "")
	ch, cancel := e.Subscribe()
	defer cancel()
	e.Prompt(ctx, ses.ID, "add a test for the parser")
	waitIdle(t, ch, ses.ID)
	got := make(chan permission.Decision, 1)
	go func() {
		got <- e.broker.Request(ctx, permission.Context{Mode: permission.ModeAsk, Root: ses.ProjectRoot}, permission.Request{SessionID: ses.ID, Tool: "shell", Kind: permission.KindExecute, Command: "make test"})
	}()
	for len(e.broker.Pending()) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	res, err := e.HandOff(ctx, ses.ID, "fallback", stuckToolFailures, false)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case d := <-got:
		if d.Outcome != permission.Cancelled {
			t.Errorf("the old request ended %s", d.Outcome)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the old session's request still waits")
	}
	if !strings.Contains(res.Continue, "add a test for the parser") {
		t.Errorf("continue = %q", res.Continue)
	}
	if waitEvent(t, e, ses.ID, event.HandedOff) == nil {
		t.Error("the old session does not say where it continued")
	}
	evs, _ := e.Events(ctx, res.Session.ID, 0)
	if strings.Contains(eventsText(evs), "user_message_chunk") && strings.Contains(eventsText(evs), "add a test for the parser") {
		t.Error("by hand, the request was sent without the user")
	}
}
