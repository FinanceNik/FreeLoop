package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

func hookEvents(t *testing.T, e *Engine, sid string) []HookRanData {
	t.Helper()
	evs, _ := e.Events(context.Background(), sid, 0)
	var out []HookRanData
	for _, ev := range evs {
		if ev.Type == event.HookRan {
			var d HookRanData
			json.Unmarshal(ev.Data, &d)
			out = append(out, d)
		}
	}
	return out
}

// EXT-005: a pre_tool_use hook blocks a shell command with its message in
// the transcript; a post_tool_use hook reads the finished call on stdin; a
// failing hook does not block; project hooks are inert until the project
// is trusted; a session_start hook can stop a session from starting.
func TestHooks(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	root := t.TempDir()
	seen := filepath.Join(t.TempDir(), "post.json")
	deniedRan := filepath.Join(t.TempDir(), "denied-post")
	c := e.Config()
	c.Hooks = []config.Hook{
		{Event: config.HookPreToolUse, Match: "shell", Command: `grep -q curl && { echo "no network from hooks test" >&2; exit 2; }; exit 0`},
		{Event: config.HookPreToolUse, Match: "write_*", Command: "exit 1"},
		{Event: config.HookPostToolUse, Match: "write_file", Command: "cat > " + seen},
		{Event: config.HookPostToolUse, Match: "shell", Command: "touch " + deniedRan},
	}
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	p := scripted.New("hp",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "curl https://example.com"}}}},
		scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "a.txt", "content": "hi\n"}}}},
		scripted.Step{Text: "done"})
	e.mu.Lock()
	e.providers.built["hp"] = &configuredProvider{cfg: config.Provider{ID: "hp", Name: "hp", Type: "scripted"}, p: p}
	e.mu.Unlock()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "hp", Model: "m", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)

	evs, _ := e.Events(ctx, ses.ID, 0)
	var shown bool
	for _, ev := range evs {
		if ev.Type == event.ToolCallUpdate && strings.Contains(string(ev.Data), `"failed"`) && strings.Contains(string(ev.Data), "no network from hooks test") {
			shown = true
		}
		if ev.Type == event.PermissionRequested {
			t.Error("the blocked command was put to the user")
		}
	}
	if !shown {
		t.Error("the hook's message is not on the failed tool call")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(b) != "hi\n" {
		t.Errorf("a hook exiting 1 blocked the write: %q", b)
	}
	runs := hookEvents(t, e, ses.ID)
	if len(runs) != 3 || !runs[0].Blocked || runs[0].ExitCode != 2 || runs[1].Blocked || runs[1].ExitCode != 1 || runs[2].Event != config.HookPostToolUse {
		t.Fatalf("hook runs = %+v", runs)
	}
	if _, err := os.Stat(deniedRan); err == nil {
		t.Error("a post_tool_use hook ran for a call that was denied")
	}
	var post map[string]any
	b, _ := os.ReadFile(seen)
	if json.Unmarshal(b, &post) != nil || post["tool"] != "write_file" || post["status"] != "completed" || post["sessionId"] != ses.ID {
		t.Errorf("post_tool_use input = %s", b)
	}

	// A project's hooks wait for trust; then one stops the session.
	os.MkdirAll(filepath.Join(root, ".freeloop"), 0o755)
	os.WriteFile(filepath.Join(root, ".freeloop", "hooks.yaml"), []byte("version: 1\nhooks:\n  - event: session_start\n    command: echo 'the project needs setup first' >&2; exit 2\n"), 0o644)
	e.stopRuntime(ses.ID)
	if _, err := e.Prompt(ctx, ses.ID, "again"); err != nil {
		t.Fatalf("an untrusted project's hook ran: %v", err)
	}
	waitIdle(t, ch, ses.ID)
	e.stopRuntime(ses.ID)
	e.SetProjectTrust(ctx, root, true)
	if _, err := e.Prompt(ctx, ses.ID, "and again"); err == nil || !strings.Contains(err.Error(), "the project needs setup first") {
		t.Errorf("session_start block = %v", err)
	}
}

// Stop also stops a post_tool_use hook: the turn does not wait out the
// hook's timeout.
func TestStopEndsPostToolUseHook(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	c := e.Config()
	c.Hooks = []config.Hook{{Event: config.HookPostToolUse, Command: "sleep 60", Timeout: 120}}
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	p := scripted.New("sp",
		scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "a.txt", "content": "x\n"}}}},
		scripted.Step{Text: "done"})
	e.mu.Lock()
	e.providers.built["sp"] = &configuredProvider{cfg: config.Provider{ID: "sp", Name: "sp", Type: "scripted"}, p: p}
	e.mu.Unlock()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "sp", Model: "m", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "go")
	// Wait for the hook to start, then stop the turn.
	for start := time.Now(); ; time.Sleep(20 * time.Millisecond) {
		evs, _ := e.Events(ctx, ses.ID, 0)
		if slices.ContainsFunc(evs, func(ev event.Event) bool {
			return ev.Type == event.ToolCallUpdate && strings.Contains(string(ev.Data), `"completed"`)
		}) {
			break
		}
		if time.Since(start) > 10*time.Second {
			t.Fatal("the write never completed")
		}
	}
	stopped := time.Now()
	e.Cancel(ses.ID)
	waitIdle(t, ch, ses.ID)
	if took := time.Since(stopped); took > 10*time.Second {
		t.Errorf("the turn ended %s after Stop", took)
	}
	for _, d := range hookEvents(t, e, ses.ID) {
		if d.Error != "" || !d.Stopped {
			t.Errorf("a hook Stop ended = %+v", d)
		}
	}
}
