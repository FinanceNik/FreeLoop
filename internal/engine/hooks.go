package engine

import (
	"context"
	"encoding/json"
	"log"
	"strings"
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
// not go through the permission broker; they are the broker's: its
// pre_tool_use and permission_request hooks can deny a call, never allow
// one. See config.Hook for the exit-code contract.

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
}

// hookCall is what a post_tool_use hook learns about the call.
type hookCall struct {
	Tool     string          `json:"tool"`
	Title    string          `json:"title"`
	Kind     string          `json:"kind"`
	RawInput json.RawMessage `json:"rawInput"`
}

// hooksOf returns the hooks in force for a runtime's session, read afresh
// so edits and trust changes apply to the next event.
func (e *Engine) hooksOf(ctx context.Context, r *runtime) []config.Hook {
	hooks := append([]config.Hook{}, e.Config().Hooks...)
	for i := range hooks {
		hooks[i].Source = config.FileName
	}
	project, err := e.store.GetProject(ctx, r.projectID)
	if err != nil {
		return hooks
	}
	if prof, err := e.profileFor(project, r.profileID); err == nil {
		for _, h := range prof.Hooks {
			h.Source = "profile " + prof.Name
			hooks = append(hooks, h)
		}
	}
	return append(hooks, e.projectConfig(project).Hooks...)
}

// runHooks runs the hooks for event and tool in order and returns the
// reason the first blocking one gave, or "". payload is added to the JSON
// the hooks read.
func (e *Engine) runHooks(ctx context.Context, r *runtime, ev, tool string, payload map[string]any) string {
	var hooks []config.Hook
	for _, h := range e.hooksOf(ctx, r) {
		if h.Matches(ev, tool) {
			hooks = append(hooks, h)
		}
	}
	if len(hooks) == 0 {
		return ""
	}
	in := map[string]any{"event": ev, "sessionId": r.sid, "cwd": r.env.Root()}
	for k, v := range payload {
		in[k] = v
	}
	stdin, err := json.Marshal(in)
	if err != nil {
		log.Printf("engine: hook input: %v", err)
		return ""
	}
	callID, _ := payload["toolCallId"].(string)
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
		if err != nil {
			d.Error = err.Error()
		}
		// A timed-out hook is killed; its exit code says nothing.
		d.Blocked = err == nil && !res.TimedOut && res.ExitCode == 2
		e.emit(r.sid, event.HookRan, d)
		if d.Blocked {
			why := strings.TrimSpace(res.Stderr)
			if why == "" {
				why = strings.TrimSpace(res.Output)
			}
			if why == "" {
				why = "blocked by a hook (" + h.Command + ")"
			}
			return capText(why)
		}
	}
	return ""
}

func capText(s string) string {
	if len(s) > hookOutputCap {
		return s[:hookOutputCap] + "\n[truncated]"
	}
	return s
}

// engineHooks lets the permission broker run a session's hooks.
type engineHooks struct{ e *Engine }

func (h engineHooks) request(ctx context.Context, ev string, pc permission.Context, req permission.Request) string {
	h.e.mu.Lock()
	r := h.e.runtimes[req.SessionID]
	h.e.mu.Unlock()
	if r == nil {
		return ""
	}
	return h.e.runHooks(ctx, r, ev, req.Tool, map[string]any{
		"tool": req.Tool, "toolCallId": req.ToolCallID, "kind": req.Kind, "title": req.Title,
		"input": req.Input, "paths": req.Paths, "command": req.Command, "mode": pc.Mode,
	})
}

func (h engineHooks) PreToolUse(ctx context.Context, pc permission.Context, req permission.Request) string {
	return h.request(ctx, config.HookPreToolUse, pc, req)
}

func (h engineHooks) PermissionRequest(ctx context.Context, pc permission.Context, req permission.Request) string {
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
	if ok && (d.Status == "completed" || d.Status == "failed") {
		delete(r.calls, d.ToolCallID)
	}
	r.callsMu.Unlock()
	if !ok || (d.Status != "completed" && d.Status != "failed") {
		return
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
	if why := e.runHooks(context.Background(), r, config.HookPostToolUse, call.Tool, map[string]any{
		"tool": call.Tool, "toolCallId": d.ToolCallID, "kind": call.Kind, "title": call.Title, "input": call.RawInput,
		"status": d.Status, "output": capText(strings.Join(out, "\n")), "paths": paths,
	}); why != "" {
		// The call already ran; a blocking post_tool_use hook can only say
		// so.
		e.emit(r.sid, event.Notice, event.NoticeData{Message: "A post_tool_use hook objected to " + call.Title + ": " + why})
	}
}
