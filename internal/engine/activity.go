package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// Activity analytics (GUI-021): what the agents did and how it went, folded from
// the transcripts into counts per hour and session. It is worked out when asked
// for, a session at a time, and only for sessions that have moved on, so nothing
// is added to a running turn and past sessions count from the first look.

// The kinds of ActivityCount.
const (
	activityTool     = "tool"     // Name is the tool; Failures, Millis running.
	activityTurn     = "turn"     // Name is why it ended; Millis long; at its start.
	activityDecision = "decision" // Name is actor:outcome; Millis waiting for the user.
	activityAsked    = "asked"    // Name is askedName; Count allowed, Failures denied.
	activityEvent    = "event"    // Name is one of the activity* names below.
)

const (
	eventEdit       = "edit"
	eventPR         = "pull_request"
	eventRewind     = "rewind"
	eventCompaction = "compaction"
	eventHandoff    = "handoff"
	eventError      = "error"
)

// A question about a longer command is not one a rule would be made from.
const maxAskedCommand = 1000

// askedName packs what the user was asked about, for a rule made from it.
func askedName(tool, command, path string) string { return tool + "\t" + command + "\t" + path }

type activityKey struct {
	day        string
	hour       int
	kind, name string
}

// foldActivity counts one transcript. Events up to copiedSeq were counted in the
// session a fork came from (SES-009).
func foldActivity(events []event.Event, copiedSeq int64) []store.ActivityCount {
	counts := map[activityKey]*store.ActivityCount{}
	bump := func(at time.Time, kind, name string, count, failures, millis int64) {
		at = at.In(time.Local)
		k := activityKey{at.Format(time.DateOnly), at.Hour(), kind, name}
		c := counts[k]
		if c == nil {
			c = &store.ActivityCount{Day: k.day, Hour: k.hour, Kind: kind, Name: name}
			counts[k] = c
		}
		c.Count += count
		c.Failures += failures
		c.Millis += millis
	}
	type toolRun struct {
		name, kind string
		started    time.Time
		notRun     bool
		finished   bool
	}
	type ask struct {
		at                  time.Time
		tool, command, path string
	}
	tools := map[string]*toolRun{}
	asks := map[string]ask{}
	var turnStart time.Time
	for _, ev := range events {
		if ev.Seq <= copiedSeq {
			continue
		}
		switch ev.Type {
		case event.UserMessageChunk:
			if turnStart.IsZero() {
				turnStart = ev.TS
			}
		case event.TurnStarted:
			turnStart = ev.TS
		case event.TurnEnded:
			var d event.TurnEndedData
			json.Unmarshal(ev.Data, &d)
			start, millis := ev.TS, int64(0)
			if !turnStart.IsZero() {
				start, millis = turnStart, ev.TS.Sub(turnStart).Milliseconds()
			}
			bump(start, activityTurn, cmp.Or(d.StopReason, "unknown"), 1, 0, millis)
			turnStart = time.Time{}
		case event.ToolCall:
			var d struct {
				ToolCallID string `json:"toolCallId"`
				Tool       string `json:"tool"`
				Kind       string `json:"kind"`
				Status     string `json:"status"`
			}
			json.Unmarshal(ev.Data, &d)
			if d.ToolCallID == "" || tools[d.ToolCallID] != nil {
				continue
			}
			name := d.Tool
			if name == "" {
				name = d.Kind
			}
			tools[d.ToolCallID] = &toolRun{name: name, kind: d.Kind, started: ev.TS}
		case event.ToolCallUpdate:
			var d struct {
				ToolCallID string `json:"toolCallId"`
				Status     string `json:"status"`
			}
			json.Unmarshal(ev.Data, &d)
			t := tools[d.ToolCallID]
			if t == nil || t.finished {
				continue
			}
			switch d.Status {
			case "in_progress":
				t.started = ev.TS // the wait for an approval is not running
			case "completed", "failed":
				t.finished = true
				var failures, millis int64
				if !t.notRun { // a denied or cancelled call never ran, and did not fail
					millis = ev.TS.Sub(t.started).Milliseconds()
					if d.Status == "failed" {
						failures = 1
					}
				}
				bump(ev.TS, activityTool, t.name, 1, failures, millis)
				if d.Status == "completed" && t.kind == string(permission.KindEdit) {
					bump(ev.TS, activityEvent, eventEdit, 1, 0, 0)
				}
			}
		case event.PermissionRequested:
			var d struct {
				RequestID string   `json:"requestId"`
				Tool      string   `json:"tool"`
				Command   string   `json:"command"`
				Paths     []string `json:"paths"`
			}
			json.Unmarshal(ev.Data, &d)
			a := ask{at: ev.TS, tool: d.Tool, command: d.Command}
			if len(d.Paths) == 1 {
				a.path = d.Paths[0]
			}
			asks[d.RequestID] = a
		case event.PermissionDecided:
			var d permission.Decision
			json.Unmarshal(ev.Data, &d)
			if t := tools[d.ToolCallID]; t != nil && d.Outcome != permission.Allowed {
				t.notRun = true
			}
			var millis int64
			if a, ok := asks[d.RequestID]; ok {
				millis = ev.TS.Sub(a.at).Milliseconds()
				if d.Actor == permission.ActorUser && d.Outcome != permission.Cancelled && (a.command != "" || a.path != "") && len(a.command) <= maxAskedCommand {
					name := askedName(a.tool, a.command, a.path)
					if d.Outcome == permission.Allowed {
						bump(ev.TS, activityAsked, name, 1, 0, millis)
					} else {
						bump(ev.TS, activityAsked, name, 0, 1, millis)
					}
				}
			}
			bump(ev.TS, activityDecision, string(d.Actor)+":"+string(d.Outcome), 1, 0, millis)
		case event.PullRequest:
			bump(ev.TS, activityEvent, eventPR, 1, 0, 0)
		case event.Rewind:
			bump(ev.TS, activityEvent, eventRewind, 1, 0, 0)
		case event.Handoff:
			bump(ev.TS, activityEvent, eventHandoff, 1, 0, 0)
		case event.Error:
			bump(ev.TS, activityEvent, eventError, 1, 0, 0)
		case event.CompactionUpdate:
			var d event.CompactionData
			json.Unmarshal(ev.Data, &d)
			if d.Status == "completed" {
				bump(ev.TS, activityEvent, eventCompaction, 1, 0, 0)
			}
		}
	}
	out := make([]store.ActivityCount, 0, len(counts))
	for _, c := range counts {
		out = append(out, *c)
	}
	return out
}

// ActivityRow is a store row, and for something the user was asked about, whether
// a rule now covers it.
type ActivityRow struct {
	store.ActivityRow
	Covered bool `json:"covered"`
}

// ActivityReport is what the analytics view works from. Pending counts sessions
// not folded yet; asking again continues.
type ActivityReport struct {
	Since   string        `json:"since"`
	Until   string        `json:"until"`
	Rows    []ActivityRow `json:"rows"`
	Pending int           `json:"pending"`
}

// activityBudget bounds the folding one report does, so a long history shows
// itself filling in rather than holding the page.
const activityBudget = 1500 * time.Millisecond

var activityMu sync.Mutex

// foldStale folds sessions that have moved on, until the budget is spent, and
// reports how many it left. One that cannot be read waits for the next report.
func (e *Engine) foldStale(ctx context.Context) int {
	activityMu.Lock()
	defer activityMu.Unlock()
	deadline := time.Now().Add(activityBudget)
	stale, err := e.store.ActivityStale(ctx)
	if err != nil {
		log.Printf("engine: activity: %v", err)
		return 0
	}
	for i, st := range stale {
		if !time.Now().Before(deadline) {
			return len(stale) - i
		}
		evs, err := e.store.Events(ctx, st.ID, 0)
		if err == nil {
			err = e.store.ReplaceActivity(ctx, st.ID, st.LastSeq, foldActivity(evs, st.CopiedSeq))
		}
		if err != nil {
			log.Printf("engine: activity of %s: %v", st.ID, err)
		}
	}
	return 0
}

// ActivityReport returns the last days days of activity, today included.
func (e *Engine) ActivityReport(ctx context.Context, days int) (ActivityReport, error) {
	if days <= 0 || days > MaxUsageDays {
		days = MaxUsageDays
	}
	pending := e.foldStale(ctx)
	now := time.Now()
	rep := ActivityReport{Since: now.AddDate(0, 0, 1-days).Format(time.DateOnly), Until: now.Format(time.DateOnly), Pending: pending}
	rows, err := e.store.ActivityRowsSince(ctx, rep.Since)
	if err != nil {
		return ActivityReport{}, err
	}
	rules := e.ListRules("")
	rep.Rows = make([]ActivityRow, 0, len(rows))
	for _, r := range rows {
		row := ActivityRow{ActivityRow: r}
		if r.Kind == activityAsked {
			row.Covered = coveredByRule(rules, r.Name)
		}
		rep.Rows = append(rep.Rows, row)
	}
	return rep, nil
}

func askedRequest(name string) (permission.Request, bool) {
	parts := strings.Split(name, "\t")
	if len(parts) != 3 {
		return permission.Request{}, false
	}
	req := permission.Request{Tool: parts[0], Command: parts[1]}
	if parts[2] != "" {
		req.Paths = []string{parts[2]}
	}
	return req, true
}

func coveredByRule(rules []permission.Rule, name string) bool {
	req, ok := askedRequest(name)
	if !ok {
		return false
	}
	for _, r := range rules {
		if r.Effect == permission.Allow && !r.Pending && r.Matches(req, "") {
			return true
		}
	}
	return false
}

// AllowRepeated makes the rule that stops the question the user keeps answering
// the same way: exactly that command or file, everywhere, as "Allow always" does.
func (e *Engine) AllowRepeated(tool, command, path string) error {
	req, ok := askedRequest(askedName(tool, command, path))
	if !ok || tool == "" || (command == "" && path == "") {
		return errors.New("nothing to make a rule from")
	}
	r, ok := permission.RuleFor(req, "", permission.ScopeGlobal)
	if !ok {
		return fmt.Errorf("a rule cannot be made for %s", tool)
	}
	return e.AddRule(r)
}
