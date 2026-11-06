//go:build darwin

package engine

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/platform/darwin"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// ENV-009, end to end on the real sandbox: a command in the project runs with no
// approval, one reaching outside is refused by the system and not by a prompt,
// leaving the sandbox asks, and the app's own git still works.
func TestSandboxedSession(t *testing.T) {
	ctx := context.Background()
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		t.Skip(err)
	}
	home := environment.Canonical(t.TempDir())
	outside := filepath.Join(home, "Documents", "taxes.txt")
	os.MkdirAll(filepath.Dir(outside), 0o755)
	os.WriteFile(outside, []byte("tax-return-marker"), 0o644)
	repo := filepath.Join(home, "code", "repo")
	os.MkdirAll(repo, 0o755)
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "config", "user.name", "t")
	gitIn(t, repo, "config", "user.email", "t@t")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644)
	gitIn(t, repo, "add", "a.txt")
	gitIn(t, repo, "commit", "-qm", "one")
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("secret.env\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "secret.env"), []byte("denied-rule-marker"), 0o644)

	e, err := Open(Options{DataDir: filepath.Join(home, "data"), Secrets: secrets.NewMemory(), Home: home, Confiner: darwin.Seatbelt{}, WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.AddRule(permission.Rule{Effect: permission.Deny, Tool: "read_file", Path: "*.env"}); err != nil {
		t.Fatal(err)
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	shell := func(input map[string]any) scripted.Step {
		return scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: input}}}
	}
	withScript(e, "s",
		shell(map[string]any{"command": "echo inside > made.txt; git add made.txt; git commit -qm two; git log --oneline | wc -l > count.txt; echo $GOCACHE > cache.txt"}),
		shell(map[string]any{"command": "cat " + outside}),
		shell(map[string]any{"command": "cat secret.env"}),
		shell(map[string]any{"command": "touch " + outside + ".left", "outside_sandbox": true, "why": "to test"}),
		scripted.Step{Text: "done"},
	)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "s", Model: "m", EnvironmentID: SandboxedEnvironmentID, Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	var asked []permission.Request
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		seen := map[string]bool{}
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
			}
			for _, r := range e.broker.Pending() {
				if !seen[r.ID] {
					seen[r.ID] = true
					asked = append(asked, r)
					e.broker.Decide(r.ID, "allow_once", "", "")
				}
			}
		}
	}()
	if _, err := e.Prompt(ctx, ses.ID, "go"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)

	if b, _ := os.ReadFile(filepath.Join(repo, "made.txt")); string(b) != "inside\n" {
		t.Errorf("made.txt = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "count.txt")); strings.TrimSpace(string(b)) != "2" {
		t.Errorf("the commit in the sandbox: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "cache.txt")); !strings.Contains(string(b), filepath.Join("Library", "Caches", "Freeloop", "sandbox")) {
		t.Errorf("the Go cache is at %q", b)
	}
	if len(asked) != 1 || !asked[0].LeavesSandbox || !strings.Contains(asked[0].Title, "Outside the sandbox") {
		t.Errorf("asked = %+v", asked)
	}
	for _, r := range asked {
		for _, o := range r.Options {
			if o.Kind == "allow_always" {
				t.Errorf("leaving the sandbox can be allowed always: %+v", r.Options)
			}
		}
	}
	if _, err := os.Stat(outside + ".left"); err != nil {
		t.Errorf("the approved command did not run outside: %v", err)
	}

	evs, _ := e.Events(ctx, ses.ID, 0)
	var results []string
	for _, ev := range evs {
		if ev.Type == event.ToolCallUpdate {
			results = append(results, string(ev.Data))
		}
	}
	all := strings.Join(results, "\n")
	if strings.Contains(all, "tax-return-marker") {
		t.Error("the agent read a file outside the project")
	}
	if strings.Contains(all, "denied-rule-marker") {
		t.Error("a deny rule did not hold for the shell")
	}
	if !strings.Contains(all, "not permitted") {
		t.Errorf("no refusal in %s", all)
	}
	decided := 0
	for _, ev := range evs {
		var d permission.Decision
		if ev.Type == event.PermissionDecided && json.Unmarshal(ev.Data, &d) == nil && d.Actor == permission.ActorSandbox {
			decided++
		}
	}
	if decided != 3 {
		t.Errorf("%d commands were decided by the sandbox", decided)
	}
	// The app's own git is not confined, and still reads the repository.
	if c, err := e.Changes(ctx, ses.ID); err != nil || !c.Repo {
		t.Errorf("changes = %+v, %v", c, err)
	}
}

// ENV-009: no sandbox on this system, or one that does not hold, runs nothing.
func TestSandboxFailsClosed(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	for name, c := range map[string]environment.Confiner{"none": nil, "useless": uselessConfiner{}, "refuses": refusingConfiner{}} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), Confiner: c, WatchInterval: -1})
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			withScript(e, "s", scripted.Step{Text: "hello"})
			ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "s", Model: "m", EnvironmentID: SandboxedEnvironmentID})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Prompt(ctx, ses.ID, "go"); err == nil {
				t.Error("a session started with no working sandbox")
			}
		})
	}
}

// uselessConfiner confines nothing: the self-test has to catch it.
type uselessConfiner struct{}

func (uselessConfiner) Confine(_ environment.Confinement, argv []string) ([]string, error) {
	return argv, nil
}

type refusingConfiner struct{}

func (refusingConfiner) Confine(environment.Confinement, []string) ([]string, error) {
	return nil, os.ErrPermission
}

// The built-in sandboxed environment is offered and can be saved and changed.
func TestSandboxSettings(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	var found bool
	for _, info := range e.ListEnvironments() {
		if info.ID == SandboxedEnvironmentID {
			found = info.Sandbox != nil && info.Sandbox.Enabled && info.Builtin
		}
	}
	if !found {
		t.Fatal("no sandboxed environment is offered")
	}
	info, err := e.SaveEnvironment(SaveEnvironmentRequest{Name: "Tight", Kind: "local", Sandbox: &config.Sandbox{Enabled: true, Network: "none", Read: []string{"~/tools"}}})
	if err != nil || info.Sandbox == nil || info.Sandbox.Network != "none" {
		t.Fatalf("saved = %+v, %v", info, err)
	}
	if _, err := e.SaveEnvironment(SaveEnvironmentRequest{Name: "Listed", Kind: "local", Sandbox: &config.Sandbox{Enabled: true, Network: "hosts", Hosts: []string{"registry.npmjs.org", "*.github.com", "git.example.com:8443"}}}); err != nil {
		t.Errorf("a list of hosts: %v", err)
	}
	for _, bad := range []config.Sandbox{
		{Enabled: true, Network: "hosts"},
		{Enabled: true, Network: "hosts", Hosts: []string{"*"}},
		{Enabled: true, Network: "hosts", Hosts: []string{"*.com.*"}},
		{Enabled: true, Network: "hosts", Hosts: []string{"localhost"}},
		{Enabled: true, Network: "hosts", Hosts: []string{"a.example.com/path"}},
		{Enabled: true, Network: "everywhere"}, {Enabled: true, Read: []string{"/"}}, {Enabled: true, Write: []string{"~"}}, {Enabled: true, Write: []string{"relative"}}} {
		s := bad
		if _, err := e.SaveEnvironment(SaveEnvironmentRequest{Name: "Bad", Kind: "local", Sandbox: &s}); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
	if _, err := e.SaveEnvironment(SaveEnvironmentRequest{Name: "Docker", Kind: "docker", Image: "alpine", Sandbox: &config.Sandbox{Enabled: true}}); err != nil {
		t.Errorf("a docker environment ignores the sandbox: %v", err)
	}
}

// ENV-009: in plan mode shell commands can look but not change anything, and a
// worktree can commit in the sandbox while its git settings stay out of reach.
func TestSandboxPlanModeAndWorktree(t *testing.T) {
	ctx := context.Background()
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		t.Skip(err)
	}
	home := environment.Canonical(t.TempDir())
	repo := filepath.Join(home, "code", "repo")
	os.MkdirAll(filepath.Join(repo, ProjectDir), 0o755)
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "config", "user.name", "t")
	gitIn(t, repo, "config", "user.email", "t@t")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("project-line\n"), 0o644)
	gitIn(t, repo, "add", "a.txt")
	gitIn(t, repo, "commit", "-qm", "one")
	marker := filepath.Join(home, "outside-setup")
	os.WriteFile(filepath.Join(repo, ProjectDir, config.WorktreeFileName), []byte("setup: echo set up > setup.txt; touch "+marker+"; true\n"), 0o644)
	e, err := Open(Options{DataDir: filepath.Join(home, "data"), Secrets: secrets.NewMemory(), Home: home, Confiner: darwin.Seatbelt{}, WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.SetProjectTrust(ctx, repo, true)
	ch, cancel := e.Subscribe()
	defer cancel()
	shell := func(c string) scripted.Step {
		return scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": c}}}}
	}

	withScript(e, "plan", shell("cat a.txt"), shell("touch made-in-plan.txt"), scripted.Step{Text: "done"})
	plan, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "plan", Model: "m", EnvironmentID: SandboxedEnvironmentID, Mode: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, plan.ID, "look")
	waitIdle(t, ch, plan.ID)
	if _, err := os.Stat(filepath.Join(repo, "made-in-plan.txt")); err == nil {
		t.Error("plan mode wrote a file")
	}
	evs, _ := e.Events(ctx, plan.ID, 0)
	text := ""
	for _, ev := range evs {
		if ev.Type == event.ToolCallUpdate {
			text += string(ev.Data)
		}
	}
	if !strings.Contains(text, "project-line") || !strings.Contains(text, "not permitted") {
		t.Errorf("plan mode's commands: %s", text)
	}

	gitDir := ""
	withScript(e, "wt",
		shell("echo w > w.txt; git add w.txt; git commit -qm from-the-sandbox"),
		shell("git config core.fsmonitor x; touch evil-after-config"),
		shell("echo 'gitdir: /tmp/x' > .git"),
		scripted.Step{Text: "done"},
	)
	wt, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "wt", Model: "m", EnvironmentID: SandboxedEnvironmentID, Mode: "ask", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.waitSetup(ctx, wt.ID); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(wt.Worktree, "setup.txt")); string(b) != "set up\n" {
		t.Errorf("the setup in the sandbox: %q", b)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the setup reached outside the sandbox")
	}
	e.Prompt(ctx, wt.ID, "go")
	waitIdle(t, ch, wt.ID)
	if log := gitIn(t, wt.Worktree, "log", "--format=%s", "-n", "1"); log != "from-the-sandbox" {
		t.Errorf("the worktree's commit: %q", log)
	}
	gitDir = gitIn(t, wt.Worktree, "rev-parse", "--git-dir")
	if !strings.Contains(gitDir, "worktrees") {
		t.Errorf("the worktree's .git was changed: %s", gitDir)
	}
	if out, err := exec.Command("git", "-C", repo, "config", "--get", "core.fsmonitor").Output(); err == nil {
		t.Errorf("core.fsmonitor was set: %s", out)
	}
}

// ENV-010: a session with a list of hosts is told it, starts a proxy, and is told
// when a command is refused. No internet: the refused host never leaves the proxy.
func TestSandboxHostList(t *testing.T) {
	ctx := context.Background()
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		t.Skip(err)
	}
	home := environment.Canonical(t.TempDir())
	repo := filepath.Join(home, "repo")
	os.MkdirAll(repo, 0o755)
	e, err := Open(Options{DataDir: filepath.Join(home, "data"), Secrets: secrets.NewMemory(), Home: home, Confiner: darwin.Seatbelt{}, WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	info, err := e.SaveEnvironment(SaveEnvironmentRequest{Name: "Packages", Kind: "local", Sandbox: &config.Sandbox{Enabled: true, Network: "hosts", Hosts: []string{"registry.example.invalid"}}})
	if err != nil {
		t.Fatal(err)
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "echo $HTTPS_PROXY > proxy.txt; curl -sS -m 5 -o /dev/null https://other.example.invalid/"}}}},
		scripted.Step{Text: "done"},
	)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "s", Model: "m", EnvironmentID: info.ID, Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)
	b, _ := os.ReadFile(filepath.Join(repo, "proxy.txt"))
	if !strings.Contains(string(b), "127.0.0.1") {
		t.Errorf("the proxy variable: %q", b)
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	noticed := false
	for _, ev := range evs {
		var n event.NoticeData
		if ev.Type == event.Notice && json.Unmarshal(ev.Data, &n) == nil && strings.Contains(n.Message, "other.example.invalid") {
			noticed = true
		}
	}
	if !noticed {
		t.Error("the user was not told of the refused host")
	}
	// The proxy goes with the session.
	proxy := strings.TrimSpace(string(b))
	e.stopRuntime(ses.ID)
	r, err := e.localEnv(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := r.Exec(ctx, environment.ExecSpec{Command: "curl -sS -m 3 -x '" + proxy + "' http://registry.example.invalid/"})
	if res.ExitCode == 0 || !strings.Contains(res.Output, "connect") && !strings.Contains(res.Output, "Failed") {
		t.Errorf("the proxy outlived its session: exit %d: %s", res.ExitCode, res.Output)
	}
}

// sandboxedProject is a trusted repository with its sandboxed environment.
func sandboxedProject(t *testing.T) (*Engine, *environment.Local, string, string) {
	t.Helper()
	home := environment.Canonical(t.TempDir())
	repo := filepath.Join(home, "Documents", "repo")
	os.MkdirAll(repo, 0o755)
	gitIn(t, repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644)
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "one")
	e, err := Open(Options{DataDir: filepath.Join(home, "data"), Secrets: secrets.NewMemory(), Home: home, Confiner: darwin.Seatbelt{}, WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	p, err := e.SetProjectTrust(context.Background(), repo, true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Environment{ID: "sandboxed", Kind: "local", Sandbox: &config.Sandbox{Enabled: true}}
	env, done, err := e.sandboxEnv(context.Background(), session.Session{ID: "ses_x", ProjectID: p.ID, ProjectRoot: repo}, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(done)
	return e, env, repo, home
}

func inSandbox(t *testing.T, env *environment.Local, script string) (string, int) {
	t.Helper()
	res, err := env.Exec(context.Background(), environment.ExecSpec{Argv: []string{"/bin/sh", "-c", script}, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return res.Output, res.ExitCode
}

// One sandboxed command, which needs no approval, moved .git aside, edited its
// config and moved it back; the app's next git refresh then ran the program it
// named, outside the sandbox (security review, 2026-11-02).
func TestSandboxCannotMoveGitAside(t *testing.T) {
	e, env, repo, home := sandboxedProject(t)
	before, _ := os.ReadFile(filepath.Join(repo, ".git", "config"))
	marker := filepath.Join(home, "escaped")
	tree := strings.TrimSpace(gitIn(t, repo, "rev-parse", "HEAD^{tree}"))
	for _, script := range []string{
		"mv .git .g2",
		"cp -R .git .g2 && rm -rf .git && mv .g2 .git",
		"mkdir -p .git2 && python3 -I -c 'import os; os.rename(\".git\", \".git2/x\")'",
		"echo gitdir: ./evil > .git",
		"rm -f .git/objects/" + tree[:2] + "/" + tree[2:] + "; echo x >> .git/config",
	} {
		out, _ := inSandbox(t, env, script)
		t.Logf("%s: %s", script, out)
	}
	if b, err := os.ReadFile(filepath.Join(repo, ".git", "config")); err != nil || string(b) != string(before) {
		t.Fatalf(".git/config changed or moved: %v", err)
	}
	host, _ := e.localEnv(repo, nil)
	gitState(context.Background(), host, true)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a command left the sandbox through the app's git")
	}
}

// Git still has to work in the sandbox.
func TestSandboxGitStillCommits(t *testing.T) {
	_, env, repo, _ := sandboxedProject(t)
	out, code := inSandbox(t, env, "echo b > b.txt && git add b.txt && git -c user.name=t -c user.email=t@t commit -qm two && git checkout -qb side && git log --oneline | wc -l")
	if code != 0 || !strings.Contains(out, "2") {
		t.Fatalf("exit %d: %s", code, out)
	}
	if strings.TrimSpace(gitIn(t, repo, "rev-parse", "--abbrev-ref", "HEAD")) != "side" {
		t.Error("the commit did not land")
	}
}

// A planted .git/commondir chose what the next sandbox could write.
func TestSandboxIgnoresAPlantedCommonDir(t *testing.T) {
	e, _, repo, home := sandboxedProject(t)
	other := filepath.Join(home, "Documents", "private")
	os.MkdirAll(other, 0o755)
	gitIn(t, other, "init", "-q")
	os.WriteFile(filepath.Join(repo, ".git", "commondir"), []byte(filepath.Join(other, ".git")+"\n"), 0o644)
	cfg := config.Environment{ID: "sandboxed", Kind: "local", Sandbox: &config.Sandbox{Enabled: true}}
	env, done, err := e.sandboxEnv(context.Background(), session.Session{ID: "ses_y", ProjectRoot: repo}, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	planted := filepath.Join(other, ".git", "refs", "heads", "planted")
	inSandbox(t, env, "echo deadbeef > "+planted)
	if _, err := os.Stat(planted); err == nil {
		t.Error("a planted common directory opened another repository for writing")
	}
}

// load_skill read what the sandbox refused, through a link the sandbox let a
// command plant, with no prompt (security review, 2026-11-02).
func TestSkillLinksStayInTheSkill(t *testing.T) {
	e, env, repo, home := sandboxedProject(t)
	os.WriteFile(filepath.Join(home, ".netrc"), []byte("machine x password hunter2"), 0o600)
	// A sandboxed command cannot plant a skill at all now (SEC-005): skills steer the
	// sessions that follow. One planted another way still must not lead out.
	if out, code := inSandbox(t, env, "mkdir -p .agents/skills/y && echo x > .agents/skills/y/SKILL.md"); code == 0 {
		t.Errorf("a sandboxed command wrote a project skill: %s", out)
	}
	dir := filepath.Join(repo, ".agents", "skills", "x")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: x\ndescription: d\n---\nb\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "own.txt"), []byte("note"), 0o644)
	if err := os.Symlink(filepath.Join(home, ".netrc"), filepath.Join(dir, "ref")); err != nil {
		t.Fatal(err)
	}
	p, err := e.store.ProjectByRoot(context.Background(), environment.Canonical(repo))
	if err != nil {
		t.Fatal(err)
	}
	e.forgetSkills()
	skills := sessionSkills{e: e, projectID: p.ID, root: p.Root}
	if c, err := skills.Load("x", "ref"); err == nil || strings.Contains(c.Text, "hunter2") {
		t.Errorf("load_skill followed a link out of the skill: %q, %v", c.Text, err)
	}
	if c, err := skills.Load("x", "own.txt"); err != nil || c.Text != "note" {
		t.Errorf("a file of the skill: %q, %v", c.Text, err)
	}
	if c, err := skills.Load("x", ""); err != nil || !strings.Contains(c.Text, "b") {
		t.Errorf("the skill itself: %q, %v", c.Text, err)
	}
}

// Keys in the shell's startup files reached commands that need no approval, and
// the files were readable (security review, 2026-11-02).
func TestSandboxKeepsStartupSecretsOut(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	repo := filepath.Join(home, "Documents", "repo")
	os.MkdirAll(repo, 0o755)
	gitIn(t, repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644)
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "one")
	os.WriteFile(filepath.Join(home, ".zshenv"), []byte("export MY_API_KEY=fake-from-zshenv\nexport MY_TOOL_DIR=/opt/tool\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".zprofile"), []byte("export GH_TOKEN=fake-from-zprofile\nexport PATH=/stand/in/bin:$PATH\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[url \"https://x-access-token:fake-gh-token@github.com/\"]\n\tinsteadOf = https://github.com/\n"), 0o644)
	e, err := Open(Options{DataDir: filepath.Join(home, "data"), Secrets: secrets.NewMemory(), Home: home, Confiner: darwin.Seatbelt{}, WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	p, err := e.SetProjectTrust(context.Background(), repo, true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Environment{ID: "sandboxed", Kind: "local", Sandbox: &config.Sandbox{Enabled: true}}
	env, done, err := e.sandboxEnv(context.Background(), session.Session{ID: "ses_x", ProjectID: p.ID, ProjectRoot: repo}, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	res, err := env.Exec(context.Background(), environment.ExecSpec{Command: "env; cat ~/.zshenv; cat ~/.zprofile; cat ~/.gitconfig; true", Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fake-from-zshenv", "fake-from-zprofile", "fake-gh-token"} {
		if strings.Contains(res.Output, secret) {
			t.Errorf("a sandboxed command got %s:\n%s", secret, res.Output)
		}
	}
	for _, want := range []string{"MY_TOOL_DIR=/opt/tool", "/stand/in/bin"} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("what the startup files set is missing %s:\n%s", want, res.Output)
		}
	}
	// Git still works without its config.
	res, err = env.Exec(context.Background(), environment.ExecSpec{Command: "echo b > b.txt && git add b.txt && git -c user.name=t -c user.email=t@t commit -qm two && git log --oneline | wc -l", Timeout: 20 * time.Second})
	if err != nil || res.ExitCode != 0 || !strings.Contains(res.Output, "2") {
		t.Errorf("git in the sandbox: %v: %s", err, res.Output)
	}
}

func TestSandboxClosesNestedRepositoriesAndAgentConfig(t *testing.T) {
	_, env, repo, _ := sandboxedProject(t)
	os.MkdirAll(filepath.Join(repo, "sub", ".git", "hooks"), 0o755)
	for _, rel := range []string{"sub/.git/config", "sub/.git/hooks/pre-commit", "sub/.freeloop/hooks.yaml", ".gemini/settings.json", ".codex/config.toml", ".husky/pre-commit", ".pre-commit-config.yaml", "opencode.json"} {
		inSandbox(t, env, "mkdir -p \"$(dirname "+rel+")\"; echo x > "+rel)
		if _, err := os.Stat(filepath.Join(repo, rel)); err == nil {
			t.Errorf("a sandboxed command wrote %s", rel)
		}
	}
	if out, code := inSandbox(t, env, "mkdir -p src && echo x > src/a.go && echo y > .gitignore"); code != 0 {
		t.Errorf("ordinary writes: %s", out)
	}
}

// A folder with HEAD, objects and refs is a repository to git. A sandboxed command
// could build one, and the user's own git would run what its config names outside
// the sandbox. Git still has to replace its own HEAD.
func TestSandboxCannotMakeARepositoryOfTheProject(t *testing.T) {
	_, env, repo, home := sandboxedProject(t)
	marker := filepath.Join(home, "escaped")
	os.WriteFile(filepath.Join(repo, "evil.sh"), []byte("#!/bin/sh\necho pwned > "+marker+"\nexit 1\n"), 0o755)
	for _, name := range []string{"HEAD", "head", "sub/HEAD", "sub/deep/head", ".gitx/HEAD", ".g/HEAD"} {
		script := "mkdir -p \"$(dirname " + name + ")\"; echo 'ref: refs/heads/main' > " + name
		if out, code := inSandbox(t, env, script); code == 0 {
			t.Errorf("%s was written: %s", name, out)
		}
		if _, err := os.Stat(filepath.Join(repo, name)); err == nil {
			t.Errorf("%s exists", name)
		}
	}
	inSandbox(t, env, "mv .git/HEAD .git/HEAD.x; mkdir -p objects refs/heads; printf '[core]\\n\\tbare = false\\n\\tworktree = .\\n\\tfsmonitor = "+filepath.Join(repo, "evil.sh")+"\\n' > config; echo 'ref: refs/heads/main' > HEAD; mv .git/HEAD.x .git/HEAD")
	cmd := exec.Command("git", "status")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	cmd.Run()
	if _, err := os.Stat(marker); err == nil {
		t.Error("the user's git ran a planted program")
	}
	// Changing branch replaces .git/HEAD.
	if out, code := inSandbox(t, env, "git checkout -q -b other && git rev-parse --abbrev-ref HEAD"); code != 0 || !strings.Contains(out, "other") {
		t.Errorf("git could not change branch: exit %d: %s", code, out)
	}
}

// /private/tmp is where other programs keep their sockets. A command that could
// write there could move one into the project and connect to it.
func TestSandboxCannotMoveASocketIn(t *testing.T) {
	_, env, _, _ := sandboxedProject(t)
	dir, err := os.MkdirTemp("/private/tmp", "flsock")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(dir)
	ln, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	out, code := inSandbox(t, env, "mv "+filepath.Join(dir, "s")+" ./moved && ls moved")
	if code == 0 {
		t.Errorf("the socket moved: %s", out)
	}
	if out, code := inSandbox(t, env, "echo x > /private/tmp/flsock-written"); code == 0 {
		os.Remove("/private/tmp/flsock-written")
		t.Errorf("wrote to /private/tmp: %s", out)
	}
	// What programs use instead.
	if out, code := inSandbox(t, env, "echo x > \"$TMPDIR/own\" && cat \"$TMPDIR/own\""); code != 0 {
		t.Errorf("own temp folder: exit %d: %s", code, out)
	}
}

// A .git file naming another repository must not open that repository up.
func TestSandboxIgnoresAPlantedGitFile(t *testing.T) {
	e, _, repo, _ := sandboxedProject(t)
	other := t.TempDir()
	gitIn(t, other, "init", "-q")
	os.RemoveAll(filepath.Join(repo, ".git"))
	os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: "+filepath.Join(other, ".git")+"\n"), 0o644)
	host, err := e.localEnv(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := environment.Confinement{}
	e.gitProtected(context.Background(), host, &c)
	if slices.ContainsFunc(c.Write, func(w string) bool { return strings.Contains(w, environment.Canonical(other)) }) {
		t.Errorf("the sandbox may write the other repository: %v", c.Write)
	}
}

func TestKeepLoginVarDropsWhatLooksLikeASecret(t *testing.T) {
	for name, value := range map[string]string{
		"OPENAI": "sk-abcdefghijklmnop", "STRIPE_SK": "sk_live_abcdefghijklmnop", "SLACK": "xoxb-1234567890-abcdef", "GITLAB_ACCESS": "glpat-abcdefghijklmnop",
		"HF_HUB": "hf_abcdefghijklmnop", "BLOB": "abcdefghijklmnopqrstuvwxyz0123456789ABCD", "NODE_OPTIONS": "--require=/tmp/x.js", "GIT_SSH_COMMAND": "ssh -i x",
	} {
		if keepLoginVar(name, value) {
			t.Errorf("kept %s", name)
		}
	}
	for name, value := range map[string]string{
		"PATH": "/opt/homebrew/bin:/usr/bin:/bin:/Users/someone/.cargo/bin:/Users/someone/go/bin", "LANG": "en_US.UTF-8", "GOPATH": "/Users/someone/go", "JAVA_HOME": "/Library/Java/JavaVirtualMachines/x/Contents/Home",
	} {
		if !keepLoginVar(name, value) {
			t.Errorf("dropped %s", name)
		}
	}
}
