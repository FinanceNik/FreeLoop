package engine

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/store"
)

func evAt(seq int64, at time.Time, typ event.Type, data any) event.Event {
	b, _ := json.Marshal(data)
	return event.Event{Seq: seq, TS: at, Type: typ, Data: b}
}

func find(counts []store.ActivityCount, kind, name string) store.ActivityCount {
	var total store.ActivityCount
	for _, c := range counts {
		if c.Kind == kind && c.Name == name {
			total.Kind, total.Name = kind, name
			total.Count += c.Count
			total.Failures += c.Failures
			total.Millis += c.Millis
			total.Hour = c.Hour
		}
	}
	return total
}

// GUI-021: a transcript's tools, decisions and turns. Waiting for an approval is
// not running, and a denied call is not a failure.
func TestFoldActivity(t *testing.T) {
	t0 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.Local)
	at := func(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }
	events := []event.Event{
		evAt(1, at(0), event.TurnStarted, struct{}{}),
		// Asked, approved after 30 s, ran for 5 s.
		evAt(2, at(1), event.ToolCall, map[string]any{"toolCallId": "a", "tool": "shell", "kind": "execute"}),
		evAt(3, at(1), event.PermissionRequested, map[string]any{"requestId": "r1", "toolCallId": "a", "tool": "shell", "command": "npm test"}),
		evAt(4, at(31), event.PermissionDecided, permission.Decision{RequestID: "r1", ToolCallID: "a", Outcome: permission.Allowed, Actor: permission.ActorUser}),
		evAt(5, at(31), event.ToolCallUpdate, map[string]any{"toolCallId": "a", "status": "in_progress"}),
		evAt(6, at(36), event.ToolCallUpdate, map[string]any{"toolCallId": "a", "status": "completed"}),
		// Denied by the user: not a failure of the tool.
		evAt(7, at(40), event.ToolCall, map[string]any{"toolCallId": "b", "tool": "shell", "kind": "execute"}),
		evAt(8, at(40), event.PermissionRequested, map[string]any{"requestId": "r2", "toolCallId": "b", "tool": "shell", "command": "rm x"}),
		evAt(9, at(50), event.PermissionDecided, permission.Decision{RequestID: "r2", ToolCallID: "b", Outcome: permission.Denied, Actor: permission.ActorUser}),
		evAt(10, at(50), event.ToolCallUpdate, map[string]any{"toolCallId": "b", "status": "failed"}),
		// Left waiting when the turn was stopped: nothing ran.
		evAt(101, at(52), event.ToolCall, map[string]any{"toolCallId": "e", "tool": "write_file", "kind": "edit"}),
		evAt(102, at(52), event.PermissionRequested, map[string]any{"requestId": "r3", "toolCallId": "e", "tool": "write_file", "paths": []string{"/a"}}),
		evAt(103, at(58), event.PermissionDecided, permission.Decision{RequestID: "r3", ToolCallID: "e", Outcome: permission.Cancelled, Actor: permission.ActorUser}),
		evAt(104, at(58), event.ToolCallUpdate, map[string]any{"toolCallId": "e", "status": "failed"}),
		// An edit, allowed by the mode, that really fails, and one that works.
		evAt(11, at(60), event.ToolCall, map[string]any{"toolCallId": "c", "tool": "edit_file", "kind": "edit"}),
		evAt(12, at(60), event.PermissionDecided, permission.Decision{ToolCallID: "c", Outcome: permission.Allowed, Actor: permission.ActorMode}),
		evAt(13, at(61), event.ToolCallUpdate, map[string]any{"toolCallId": "c", "status": "failed"}),
		evAt(14, at(62), event.ToolCall, map[string]any{"toolCallId": "d", "tool": "edit_file", "kind": "edit"}),
		evAt(15, at(63), event.ToolCallUpdate, map[string]any{"toolCallId": "d", "status": "completed"}),
		evAt(16, at(70), event.TurnEnded, event.TurnEndedData{StopReason: "end_turn"}),
		evAt(17, at(80), event.Rewind, event.RewindData{}),
		evAt(18, at(81), event.CompactionUpdate, event.CompactionData{Status: "started"}),
		evAt(19, at(82), event.CompactionUpdate, event.CompactionData{Status: "completed"}),
		evAt(20, at(83), event.PullRequest, event.PullRequestData{URL: "u"}),
		evAt(21, at(84), event.Error, event.ErrorData{Message: "x"}),
		// A turn still running is not counted yet.
		evAt(22, at(90), event.TurnStarted, struct{}{}),
	}
	for i := range events {
		events[i].Seq = int64(i + 1)
	}
	got := foldActivity(events, 0)
	if c := find(got, activityTool, "shell"); c.Count != 2 || c.Failures != 0 || c.Millis != 5000 {
		t.Errorf("shell = %+v", c)
	}
	if c := find(got, activityTool, "edit_file"); c.Count != 2 || c.Failures != 1 {
		t.Errorf("edit_file = %+v", c)
	}
	if c := find(got, activityTool, "write_file"); c.Count != 1 || c.Failures != 0 || c.Millis != 0 {
		t.Errorf("write_file = %+v", c)
	}
	if c := find(got, activityAsked, askedName("write_file", "", "/a")); c.Count != 0 || c.Failures != 0 {
		t.Errorf("a cancelled question counted as an answer: %+v", c)
	}
	if c := find(got, activityEvent, eventEdit); c.Count != 1 {
		t.Errorf("edits = %+v", c)
	}
	if c := find(got, activityTurn, "end_turn"); c.Count != 1 || c.Millis != 70000 || c.Hour != 9 {
		t.Errorf("turn = %+v", c)
	}
	if c := find(got, activityDecision, "user:allowed"); c.Count != 1 || c.Millis != 30000 {
		t.Errorf("user allowed = %+v", c)
	}
	if c := find(got, activityDecision, "user:denied"); c.Count != 1 || c.Millis != 10000 {
		t.Errorf("user denied = %+v", c)
	}
	if c := find(got, activityDecision, "mode:allowed"); c.Count != 1 {
		t.Errorf("mode = %+v", c)
	}
	if c := find(got, activityAsked, askedName("shell", "npm test", "")); c.Count != 1 || c.Failures != 0 {
		t.Errorf("asked = %+v", c)
	}
	if c := find(got, activityAsked, askedName("shell", "rm x", "")); c.Count != 0 || c.Failures != 1 {
		t.Errorf("denied ask = %+v", c)
	}
	for _, name := range []string{eventRewind, eventCompaction, eventPR, eventError} {
		if c := find(got, activityEvent, name); c.Count != 1 {
			t.Errorf("%s = %+v", name, c)
		}
	}
	// A fork's copied events were counted in the session they came from.
	if forked := foldActivity(events, 20); find(forked, activityTool, "shell").Count != 0 || find(forked, activityEvent, eventRewind).Count != 1 {
		t.Errorf("forked = %+v", forked)
	}
}

// GUI-021: past sessions count from the first look, a session that moves on is
// counted again without doubling, and a deleted one's work stays.
func TestActivityReport(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "", "ask")
	e.emit(ses.ID, event.TurnStarted, struct{}{})
	e.emit(ses.ID, event.TurnEnded, event.TurnEndedData{StopReason: "end_turn"})
	turns := func() int64 {
		rep, err := e.ActivityReport(ctx, 7)
		if err != nil || rep.Pending != 0 {
			t.Fatalf("report: %+v, %v", rep, err)
		}
		var n int64
		for _, r := range rep.Rows {
			if r.Kind == activityTurn && r.Name == "end_turn" {
				n += r.Count
				if r.SessionID != ses.ID || r.ProjectRoot == "" {
					t.Errorf("row = %+v", r)
				}
			}
		}
		return n
	}
	if n := turns(); n != 1 {
		t.Errorf("turns = %d", n)
	}
	if n := turns(); n != 1 {
		t.Errorf("asking twice: %d", n)
	}
	e.emit(ses.ID, event.TurnStarted, struct{}{})
	e.emit(ses.ID, event.TurnEnded, event.TurnEndedData{StopReason: "end_turn"})
	if n := turns(); n != 2 {
		t.Errorf("after another turn: %d", n)
	}
	// Deleting a session keeps its counts and removes what quotes its commands.
	e.emit(ses.ID, event.PermissionRequested, permission.Request{ID: "r", SessionID: ses.ID, Tool: "shell", Command: "curl -H 'token: secret' x"})
	e.emit(ses.ID, event.PermissionDecided, permission.Decision{RequestID: "r", Outcome: permission.Allowed, Actor: permission.ActorUser})
	rep, err := e.ActivityReport(ctx, 7)
	if err != nil || !slices.ContainsFunc(rep.Rows, func(r ActivityRow) bool { return r.Kind == activityAsked }) {
		t.Fatalf("the question was not counted: %+v, %v", rep.Rows, err)
	}
	if err := e.DeleteSession(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	rep, err = e.ActivityReport(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(rep.Rows, func(r ActivityRow) bool { return r.Kind == activityTurn && r.Deleted }) {
		t.Errorf("a deleted session's work went: %+v", rep.Rows)
	}
	if slices.ContainsFunc(rep.Rows, func(r ActivityRow) bool { return strings.Contains(r.Name, "secret") }) {
		t.Errorf("a deleted session's command stayed: %+v", rep.Rows)
	}
}

// GUI-021: a rule from a repeated approval covers exactly that command, and the
// report then says so.
func TestAllowRepeated(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "", "ask")
	e.emit(ses.ID, event.PermissionRequested, permission.Request{ID: "r", SessionID: ses.ID, Tool: "shell", Kind: permission.KindExecute, Command: "npm test"})
	e.emit(ses.ID, event.PermissionDecided, permission.Decision{RequestID: "r", Outcome: permission.Allowed, Actor: permission.ActorUser})
	covered := func() bool {
		rep, err := e.ActivityReport(ctx, 7)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rep.Rows {
			if r.Kind == activityAsked && strings.Contains(r.Name, "npm test") {
				return r.Covered
			}
		}
		t.Fatal("the question was not counted")
		return false
	}
	if covered() {
		t.Error("covered before a rule")
	}
	if err := e.AllowRepeated("shell", "npm test", ""); err != nil {
		t.Fatal(err)
	}
	if !covered() {
		t.Error("not covered after the rule")
	}
	rules := e.ListRules("")
	if !slices.ContainsFunc(rules, func(r permission.Rule) bool {
		return r.Effect == permission.Allow && r.Tool == "shell" && r.Literal && r.Matches(permission.Request{Tool: "shell", Command: "npm test"}, "") && !r.Matches(permission.Request{Tool: "shell", Command: "npm test; rm -rf ~"}, "")
	}) {
		t.Errorf("rules = %+v", rules)
	}
	if err := e.AllowRepeated("shell", "", ""); err == nil {
		t.Error("a rule with nothing to name was made")
	}
}
