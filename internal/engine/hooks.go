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
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// Hooks (EXT-005): the user's shell commands at points of a session's
// life, from config.yaml, the session's profile and a trusted project's
// .freeloop/hooks.yaml. They run in the session's environment, inside its
// container for a Docker session, with the event as JSON on stdin. They are
// the user's own commands, like the diff panel's git actions, so they do
// not go through the permission broker, which is why one from a file runs only
// once the user approved it (approvals.go), and a project's run only as the
// set its trust or a later approval saw (projecthooks.go); they are the
// broker's: its pre_tool_use and permission_request hooks can deny a call,
// never allow one, and one that fails to answer puts the call to the user. See
// config.Hook for the exit-code contract.

// hookOutputCap bounds the output a hook_ran event records.
const hookOutputCap = 4 << 10

// HookRanData is the payload of hook_ran.
type HookRanData struct {
	Event      string `json:"event"`
	Command    string `json:"command"`
	Source     string `json:"source,omitempty"`
	Tool       string `json:"tool,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	ExitCode   int    `json:"exitCode"`
	TimedOut   bool   `json:"timedOut,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	// Blocked is set when the hook exited 2 and stopped what it ran for.
	Blocked bool `json:"blocked,omitempty"`
	// Error is set when the hook could not run at all.
	Error string `json:"error,omitempty"`
	// Stopped is set when the user stopped the turn while the hook ran.
	Stopped bool `json:"stopped,omitempty"`
}

// hookCall is what a post_tool_use hook learns about the call; started
// is set once the call ran, past the permission broker.
type hookCall struct {
	Tool     string          `json:"tool"`
	Title    string          `json:"title"`
	Kind     string          `json:"kind"`
	RawInput json.RawMessage `json:"rawInput"`
	started  bool
}

// hooksOf returns the hooks in force for a runtime's session, read afresh
// so edits and trust changes apply to the next event. When the session's project or
// profile cannot be looked up it returns the hooks it has with the reason: hooks it
// could not read may have been guards, so the caller does not read the rest as all
// there is (SEC-003).
func (e *Engine) hooksOf(ctx context.Context, r *runtime) ([]config.Hook, error) {
	hooks := append([]config.Hook{}, e.Config().Hooks...)
	for i := range hooks {
		hooks[i].Source = config.FileName
	}
	hooks = e.approvedHooks(r, hooks)
	project, err := e.store.GetProject(ctx, r.projectID)
	if err != nil {
		return hooks, fmt.Errorf("the session's project could not be looked up, so its hooks could not be read: %w", err)
	}
	pc := e.projectConfig(project)
	// A trusted project's hooks, its profiles' included, run as the set the user
	// approved: trusting it approved them as they were.
	projectOK := e.projectHooksRun(r, project, pc)
	var missing error
	if prof, err := e.profileFor(project, r.profileID); err == nil {
		var own []config.Hook
		for _, h := range prof.Hooks {
			h.Source = "profile " + prof.Name
			own = append(own, h)
		}
		if !strings.HasPrefix(r.profileID, config.ProjectProfilePrefix) {
			own = e.approvedHooks(r, own)
		} else if !projectOK {
			own = nil
		}
		hooks = append(hooks, own...)
	} else {
		missing = fmt.Errorf("the session's profile could not be looked up, so its hooks could not be read: %w", err)
	}
	if !projectOK {
		return hooks, missing
	}
	return append(hooks, pc.Hooks...), missing
}

// warned remembers what the log said, so a problem that every call meets is named once.
var warned sync.Map

func warnOnce(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if _, seen := warned.LoadOrStore(msg, true); !seen {
		log.Print(msg)
	}
}

// runHooks runs the hooks for event and tool in order. Block is the reason
// the first blocking one gave; Failed says which ran but did not answer (timed
// out, could not start, was killed, was stopped or exited other than 0 and 2),
// which only a guard's caller acts on. payload is added to the JSON the hooks
// read.
func (e *Engine) runHooks(ctx context.Context, r *runtime, ev, tool string, payload map[string]any) (res permission.HookResult) {
	all, lookup := e.hooksOf(ctx, r)
	if lookup != nil {
		// Said as a hook that did not answer, which a guard's caller turns into a question.
		warnOnce("engine: %s: %v", r.sid, lookup)
		defer func() { res.Failed = strings.TrimPrefix(lookup.Error()+"; "+res.Failed, "; ") }()
	}
	var hooks []config.Hook
	for _, h := range all {
		if h.Matches(ev, tool) {
			hooks = append(hooks, h)
		}
	}
	if len(hooks) == 0 {
		return permission.HookResult{}
	}
	in := map[string]any{"event": ev, "sessionId": r.sid, "cwd": r.env.Root()}
	for k, v := range payload {
		in[k] = v
	}
	stdin, err := json.Marshal(in)
	if err != nil {
		log.Printf("engine: hook input: %v", err)
		return permission.HookResult{Failed: fmt.Sprintf("the %s hooks could not be given the call: %v", ev, err)}
	}
	callID, _ := payload["toolCallId"].(string)
	var failed []string
	for _, h := range hooks {
		timeout := h.Timeout
		if timeout == 0 {
			timeout = config.DefaultHookTimeout
		}
		res, err := r.env.Exec(ctx, environment.ExecSpec{
			Command: h.Command, Stdin: stdin, SeparateStderr: true, MaxOutput: 64 << 10,
			Timeout: time.Duration(timeout) * time.Second,
			Env:     map[string]string{"FREELOOP_HOOK_EVENT": ev, "FREELOOP_SESSION_ID": r.sid, "FREELOOP_TOOL": tool},
		})
		d := HookRanData{Event: ev, Command: h.Command, Source: h.Source, Tool: tool, ToolCallID: callID,
			ExitCode: res.ExitCode, TimedOut: res.TimedOut, DurationMs: res.Duration.Milliseconds(),
			Stdout: capText(res.Output), Stderr: capText(res.Stderr)}
		switch {
		case err != nil && ctx.Err() != nil:
			d.Stopped = true
		case err != nil:
			d.Error = err.Error()
		}
		// A timed-out hook is killed; its exit code says nothing.
		d.Blocked = err == nil && !res.TimedOut && res.ExitCode == 2
		e.emit(r.sid, event.HookRan, d)
		if d.Stopped {
			return permission.HookResult{Failed: hookFailure(h, d, timeout)}
		}
		if d.Blocked {
			why := strings.TrimSpace(res.Stderr)
			if why == "" {
				why = strings.TrimSpace(res.Output)
			}
			if why == "" {
				why = "blocked by a hook (" + h.Command + ")"
			}
			return permission.HookResult{Block: capText(why)}
		}
		if why := hookFailure(h, d, timeout); why != "" {
			failed = append(failed, why)
		}
	}
	return permission.HookResult{Failed: strings.Join(failed, "; ")}
}

// hookFailure says how a hook run failed to answer, or "" for exit 0 or 2.
func hookFailure(h config.Hook, d HookRanData, timeout int) string {
	name := fmt.Sprintf("the %s hook %q", h.Event, capCommand(h.Command))
	if h.Source != "" {
		name += " from " + h.Source
	}
	switch {
	case d.Stopped:
		return name + " was stopped"
	case d.TimedOut:
		return fmt.Sprintf("%s timed out after %d s", name, timeout)
	case d.Error != "":
		return name + " could not start: " + d.Error
	case d.ExitCode < 0:
		return name + " was killed"
	case d.ExitCode != 0 && d.ExitCode != 2:
		return fmt.Sprintf("%s exited with code %d", name, d.ExitCode)
	}
	return ""
}

// capCommand shortens a hook's command for a reason shown on one line.
func capCommand(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
}

func capText(s string) string {
	if len(s) > hookOutputCap {
		return s[:hookOutputCap] + "\n[truncated]"
	}
	return s
}

// engineHooks lets the permission broker run a session's hooks.
type engineHooks struct{ e *Engine }

func (h engineHooks) request(ctx context.Context, ev string, pc permission.Context, req permission.Request) permission.HookResult {
	h.e.mu.Lock()
	r := h.e.runtimes[req.SessionID]
	if r == nil {
		r = h.e.starting[req.SessionID]
	}
	h.e.mu.Unlock()
	if r == nil {
		if req.SessionID == "" {
			return permission.HookResult{}
		}
		// The session's hooks cannot be found, and one of them may be a guard.
		warnOnce("engine: a request of %s, which has no running session, ran no hooks", req.SessionID)
		return permission.HookResult{Failed: "the session is not running, so its hooks could not be run"}
	}
	return h.e.runHooks(ctx, r, ev, req.Tool, map[string]any{
		"tool": req.Tool, "toolCallId": req.ToolCallID, "kind": req.Kind, "title": req.Title,
		"input": req.Input, "paths": req.Paths, "command": req.Command, "mode": pc.Mode,
	})
}

func (h engineHooks) PreToolUse(ctx context.Context, pc permission.Context, req permission.Request) permission.HookResult {
	return h.request(ctx, config.HookPreToolUse, pc, req)
}

func (h engineHooks) PermissionRequest(ctx context.Context, pc permission.Context, req permission.Request) permission.HookResult {
	return h.request(ctx, config.HookPermissionRequest, pc, req)
}

// toolHooks follows a session's tool calls and runs post_tool_use hooks
// when one finishes, before the harness goes on, so a formatter's changes
// are in place for the next step.
func (e *Engine) toolHooks(r *runtime, typ event.Type, data any) {
	if typ != event.ToolCall && typ != event.ToolCallUpdate {
		return
	}
	b, _ := json.Marshal(data)
	var d struct {
		hookCall
		ToolCallID string `json:"toolCallId"`
		Status     string `json:"status"`
		Content    []struct {
			Type    string             `json:"type"`
			Content event.ContentBlock `json:"content"`
			Path    string             `json:"path"`
		} `json:"content"`
	}
	if json.Unmarshal(b, &d) != nil || d.ToolCallID == "" {
		return
	}
	r.callsMu.Lock()
	if typ == event.ToolCall {
		if r.calls == nil {
			r.calls = map[string]hookCall{}
		}
		r.calls[d.ToolCallID] = d.hookCall
		r.callsMu.Unlock()
		return
	}
	call, ok := r.calls[d.ToolCallID]
	if ok && d.Status == "in_progress" {
		call.started = true
		r.calls[d.ToolCallID] = call
	}
	if ok && (d.Status == "completed" || d.Status == "failed") {
		delete(r.calls, d.ToolCallID)
	}
	r.callsMu.Unlock()
	// A call that failed before it started was denied or refused: it never
	// ran, so it gets no post_tool_use hook.
	if !ok || !(d.Status == "completed" || (d.Status == "failed" && call.started)) {
		return
	}
	// The turn's context, so Stop also stops the hook.
	r.mu.Lock()
	ctx := r.turn
	r.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	var out []string
	var paths []string
	for _, c := range d.Content {
		switch c.Type {
		case "content":
			out = append(out, c.Content.Text)
		case "diff":
			paths = append(paths, c.Path)
		}
	}
	if why := e.runHooks(ctx, r, config.HookPostToolUse, call.Tool, map[string]any{
		"tool": call.Tool, "toolCallId": d.ToolCallID, "kind": call.Kind, "title": call.Title, "input": call.RawInput,
		"status": d.Status, "output": capText(strings.Join(out, "\n")), "paths": paths,
	}).Block; why != "" {
		// The call already ran; a blocking post_tool_use hook can only say
		// so.
		e.emit(r.sid, event.Notice, event.NoticeData{Message: "A post_tool_use hook objected to " + call.Title + ": " + why})
	}
}
