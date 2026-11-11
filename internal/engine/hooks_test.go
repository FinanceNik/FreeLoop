package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
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
// the transcript; a post_tool_use hook reads the finished call on stdin; one
// exiting 0 changes nothing; project hooks are inert until the project is
// trusted; a session_start hook can stop a session from starting.
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
		{Event: config.HookPreToolUse, Match: "write_*", Command: "echo looked; exit 0"},
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
		t.Errorf("a hook exiting 0 changed the write: %q", b)
	}
	runs := hookEvents(t, e, ses.ID)
	if len(runs) != 3 || !runs[0].Blocked || runs[0].ExitCode != 2 || runs[1].Blocked || runs[1].ExitCode != 0 || runs[2].Event != config.HookPostToolUse {
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
	// The trust prompt shows what trusting would run.
	if s, _ := e.ProjectSettings(ctx, root); s.Trusted || len(s.Hooks) != 1 || !strings.Contains(s.Hooks[0].Command, "needs setup first") {
		t.Errorf("settings before trust = %+v", s)
	}
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

// A session started from several places at once starts once: its
// session_start hooks run a single time.
func TestSessionStartsOnce(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	runs := filepath.Join(t.TempDir(), "runs")
	c := e.Config()
	c.Hooks = []config.Hook{{Event: config.HookSessionStart, Command: "sleep 0.2; echo run >> " + runs}}
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: DemoProviderID})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	got := make([]*runtime, 5)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], _ = e.runtimeFor(ctx, ses.ID)
		}()
	}
	wg.Wait()
	b, _ := os.ReadFile(runs)
	if n := strings.Count(string(b), "run"); n != 1 {
		t.Errorf("session_start ran %d times", n)
	}
	for _, r := range got {
		if r == nil || r != got[0] {
			t.Fatalf("runtimes = %v", got)
		}
	}
}

// EXT-005: a pre_tool_use hook that times out or exits other than 0 or 2 lets
// nothing through unasked: the write asks, in accept-edits and bypass too, and the
// request says which hook failed and how. Exit 2 still denies and exit 0 still lets
// the mode decide.
func TestFailedGuardHookAsks(t *testing.T) {
	for _, c := range []struct {
		name, command, mode string
		// want is in the request's reason; "" when nobody is asked.
		want    string
		written bool
	}{
		{"timeout", "sleep 5", "accept_edits", "timed out after 1 s", false},
		{"timeout in bypass", "sleep 5", "bypass", "timed out after 1 s", false},
		{"exit 1", "exit 1", "accept_edits", "exited with code 1", false},
		{"exit 2", "echo no >&2; exit 2", "accept_edits", "", false},
		{"exit 0", "exit 0", "accept_edits", "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			e := openTest(t, t.TempDir())
			defer e.Close()
			ch, cancel := e.Subscribe()
			defer cancel()
			cfg := e.Config()
			cfg.Hooks = []config.Hook{{Event: config.HookPreToolUse, Match: "write_file", Command: c.command, Timeout: 1}}
			cfg.Permissions.AllowBypass = c.mode == "bypass"
			if _, err := e.SaveSettings(cfg); err != nil {
				t.Fatal(err)
			}
			withScript(e, "s",
				scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "a.txt", "content": "hi\n"}}}},
				scripted.Step{Text: "done"})
			root := t.TempDir()
			if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
				t.Fatal(err)
			}
			ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", Mode: "accept_edits"})
			if err != nil {
				t.Fatal(err)
			}
			if c.mode != "accept_edits" {
				if err := e.SetMode(ctx, ses.ID, c.mode); err != nil {
					t.Fatal(err)
				}
			}
			e.Prompt(ctx, ses.ID, "go")
			if c.want != "" {
				ev := waitFor(t, ch, func(ev event.Event) bool { return ev.SID == ses.ID && ev.Type == event.PermissionRequested })
				var req struct {
					RequestID  string `json:"requestId"`
					HookFailed string `json:"hookFailed"`
				}
				json.Unmarshal(ev.Data, &req)
				if !strings.Contains(req.HookFailed, c.want) || !strings.Contains(req.HookFailed, c.command) || !strings.Contains(req.HookFailed, config.HookPreToolUse) {
					t.Errorf("the request says %q, want %q and the hook", req.HookFailed, c.want)
				}
				if err := e.Decide(DecideRequest{RequestID: req.RequestID, OptionID: "reject_once"}); err != nil {
					t.Fatal(err)
				}
			}
			waitIdle(t, ch, ses.ID)
			evs, _ := e.Events(ctx, ses.ID, 0)
			asked := slices.ContainsFunc(evs, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
			if asked != (c.want != "") {
				t.Errorf("asked = %v", asked)
			}
			if _, err := os.Stat(filepath.Join(root, "a.txt")); (err == nil) != c.written {
				t.Errorf("written = %v, want %v", err == nil, c.written)
			}
		})
	}
}

// SEC-006, EXT-005: trusting a project approves its hooks as they are; hooks that
// differ since run none of them, wait in Settings and are named in the session,
// until approved; distrusting ends the approval, and trusting again approves what
// is there then.
func TestProjectHooksRunAsApproved(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	mem := secrets.NewMemory()
	e := openFor(t, dir, mem)
	defer func() { e.Close() }()
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	setHooks := func(name string) {
		t.Helper()
		os.MkdirAll(filepath.Join(root, ".freeloop"), 0o755)
		if err := os.WriteFile(filepath.Join(root, ".freeloop", "hooks.yaml"), []byte("version: 1\nhooks:\n  - event: session_start\n    command: echo "+name+" >> "+marker+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		e.projects.mu.Lock()
		e.projects.byDir = nil
		e.projects.mu.Unlock()
	}
	var sid string
	ran := func() string {
		t.Helper()
		os.Remove(marker)
		e.stopRuntime(sid)
		if _, err := e.runtimeFor(ctx, sid); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(marker)
		return strings.TrimSpace(string(b))
	}
	waiting := func() []PendingCommand {
		return slices.DeleteFunc(e.PendingCommands(), func(c PendingCommand) bool { return c.Kind != pendingProjectHooks })
	}

	setHooks("A")
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	withScript(e, "s", scripted.Step{Text: "hi"})
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	sid = ses.ID
	if got := ran(); got != "A" || len(waiting()) != 0 {
		t.Fatalf("trusted with A: ran %q, waiting %+v", got, waiting())
	}

	setHooks("B")
	if got := ran(); got != "" {
		t.Errorf("changed hooks ran unapproved: %q", got)
	}
	list := waiting()
	if len(list) != 1 || !strings.Contains(list[0].Runs, "echo B") || !strings.Contains(list[0].Source, root) {
		t.Fatalf("waiting = %+v", list)
	}
	evs, _ := e.Events(ctx, sid, 0)
	if !slices.ContainsFunc(evs, func(ev event.Event) bool {
		return ev.Type == event.Notice && strings.Contains(string(ev.Data), "not the ones approved") && strings.Contains(string(ev.Data), "echo B")
	}) {
		t.Error("the session does not say the project's hooks wait for approval")
	}
	if !slices.ContainsFunc(e.ConfigIssues(), func(i config.Issue) bool { return i.Approval && strings.Contains(i.Message, "echo B") }) {
		t.Error("the problems list does not name the waiting hooks")
	}
	if _, err := e.ApproveCommand(list[0].Fingerprint); err != nil {
		t.Fatal(err)
	}
	if got := ran(); got != "B" || len(waiting()) != 0 {
		t.Errorf("approved B: ran %q, waiting %+v", got, waiting())
	}

	// Distrusting ends B's approval: C, trusted, runs; B back again waits.
	if _, err := e.SetProjectTrust(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	if len(waiting()) != 0 {
		t.Errorf("an untrusted project's hooks wait: %+v", waiting())
	}
	setHooks("C")
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	if got := ran(); got != "C" {
		t.Errorf("trusted again with C: ran %q", got)
	}
	setHooks("B")
	if got := ran(); got != "" || len(waiting()) != 1 {
		t.Errorf("B after distrust: ran %q, waiting %+v", got, waiting())
	}

	// A version that did not ask yet adopts what trusted projects hold, once.
	ref := e.projectHooksAdoptedRef()
	e.Close()
	mem.Delete(ref)
	e = openFor(t, dir, mem)
	if len(waiting()) != 0 {
		t.Errorf("not adopted: %+v", waiting())
	}
	setHooks("D")
	e.Close()
	e = openFor(t, dir, mem)
	if len(waiting()) != 1 {
		t.Errorf("adopted twice: %+v", waiting())
	}
}

// A guard hook that cannot be found is a guard that has not answered: when the project or
// the profile cannot be looked up, or the session has no runtime, the call asks and says
// why, as for a hook that failed, where it used to run none and go ahead (SEC-003, EXT-005).
func TestGuardHooksFailClosedWhenTheyCannotBeLookedUp(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()

	// A session whose project is gone.
	r := &runtime{sid: "s", projectID: "prj_gone"}
	if _, err := e.hooksOf(ctx, r); err == nil {
		t.Error("hooksOf says nothing of a project that cannot be found")
	}
	res := e.runHooks(ctx, r, config.HookPreToolUse, "write_file", map[string]any{"tool": "write_file"})
	if res.Failed == "" || res.Block != "" {
		t.Errorf("runHooks for a project that cannot be found: %+v", res)
	}

	// A session whose profile is gone, in a project that is there.
	root := t.TempDir()
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	projects, _ := e.store.ListProjects(ctx)
	var pid string
	for _, p := range projects {
		if p.Root == environment.Canonical(root) {
			pid = p.ID
		}
	}
	if pid == "" {
		t.Fatalf("the project was not recorded: %+v", projects)
	}
	r = &runtime{sid: "s", projectID: pid, profileID: "profile-that-was-deleted"}
	if res := e.runHooks(ctx, r, config.HookPreToolUse, "write_file", nil); res.Failed == "" {
		t.Errorf("runHooks for a profile that cannot be found: %+v", res)
	}
	// With the default profile there is nothing missing.
	r = &runtime{sid: "s", projectID: pid}
	if res := e.runHooks(ctx, r, config.HookPreToolUse, "write_file", nil); res.Failed != "" || res.Block != "" {
		t.Errorf("runHooks with nothing missing: %+v", res)
	}

	// No runtime for a session that is named.
	h := engineHooks{e}
	pc := permission.Context{Mode: permission.ModeAcceptEdits}
	if res := h.PreToolUse(ctx, pc, permission.Request{SessionID: "ses_stopped", Tool: "write_file"}); res.Failed == "" {
		t.Errorf("a request of a session with no runtime ran no hooks and said nothing: %+v", res)
	}
	if res := h.PreToolUse(ctx, pc, permission.Request{Tool: "write_file"}); res.Failed != "" {
		t.Errorf("a request of no session: %+v", res)
	}
}
