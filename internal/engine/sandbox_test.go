//go:build darwin

package engine

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
	for _, bad := range []config.Sandbox{{Enabled: true, Network: "everywhere"}, {Enabled: true, Read: []string{"/"}}, {Enabled: true, Write: []string{"~"}}, {Enabled: true, Write: []string{"relative"}}} {
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
