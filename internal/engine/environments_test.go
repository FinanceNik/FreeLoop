package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// withScript registers a scripted provider under id.
func withScript(e *Engine, id string, steps ...scripted.Step) {
	e.mu.Lock()
	e.providers.built[id] = &configuredProvider{cfg: config.Provider{ID: id, Name: id, Type: "scripted"}, p: scripted.New(id, steps...)}
	e.mu.Unlock()
}

// approveAll answers every pending request with allow_once until stop closes.
func approveAll(e *Engine, stop <-chan struct{}) {
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
			for _, r := range e.PendingPermissions() {
				e.Decide(DecideRequest{RequestID: r.ID, OptionID: "allow_once"})
			}
		}
	}()
}

func waitIdle(t *testing.T, ch <-chan event.Event, sids ...string) {
	t.Helper()
	left := map[string]bool{}
	for _, s := range sids {
		left[s] = true
	}
	deadline := time.After(10 * time.Second)
	for len(left) > 0 {
		select {
		case ev := <-ch:
			if ev.Type == event.TurnEnded {
				delete(left, ev.SID)
			}
		case <-deadline:
			t.Fatalf("sessions still running: %v", left)
		}
	}
}

func TestEnvironmentSecretsReachCommandsButNotTheTranscript(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	mem := secrets.NewMemory()
	e, err := Open(Options{DataDir: dir, Secrets: mem, Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ch, unsub := e.Subscribe()
	defer unsub()

	const secret = "ghp_s3cr3t-value-0123456789"
	info, err := e.SaveEnvironment(SaveEnvironmentRequest{
		Name: "Work", Env: map[string]string{"STAGE": "dev"}, Secrets: map[string]string{"GITHUB_TOKEN": secret},
		DisabledTools: []string{"grep"}, DefaultMode: "accept_edits",
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "work" || len(info.SecretNames) != 1 {
		t.Errorf("info = %+v", info)
	}
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "echo token=$GITHUB_TOKEN stage=$STAGE"}}}},
		scripted.Step{Text: "done"},
	)
	root := t.TempDir()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if ses.Mode != "accept_edits" || ses.EnvironmentID != "work" {
		t.Errorf("session = %+v; mode should come from the environment", ses)
	}
	stop := make(chan struct{})
	defer close(stop)
	approveAll(e, stop)
	e.Prompt(ctx, ses.ID, "run")
	waitIdle(t, ch, ses.ID)

	evs, _ := e.Events(ctx, ses.ID, 0)
	var out string
	for _, ev := range evs {
		if ev.Type == event.ToolCallUpdate && strings.Contains(string(ev.Data), "stage=dev") {
			out = string(ev.Data)
		}
	}
	if !strings.Contains(out, "token=[REDACTED]") {
		t.Errorf("the command did not see the secret, or it was not redacted: %s", out)
	}
	// Nothing under the data directory holds the value (ENV-004).
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), secret) {
				t.Errorf("%s contains the secret", p)
			}
		}
		return nil
	})
	if v, _ := mem.Get(secrets.Ref{Service: "freeloop", Account: "environments/work/GITHUB_TOKEN"}); v != secret {
		t.Errorf("keychain = %q", v)
	}

	// Removing the secret deletes it from the Keychain; the environment's
	// other settings stay.
	if _, err := e.SaveEnvironment(SaveEnvironmentRequest{ID: "work", Name: "Work", Env: map[string]string{"STAGE": "dev"}, RemoveSecrets: []string{"GITHUB_TOKEN"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get(secrets.Ref{Service: "freeloop", Account: "environments/work/GITHUB_TOKEN"}); err == nil {
		t.Error("removed secret left in the keychain")
	}
}

// ENV-003: three sessions on three repos run at once, each with its own
// working directory and variables.
func TestThreeSessionsThreeReposConcurrently(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, unsub := e.Subscribe()
	defer unsub()
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "pwd; echo name=$PROJECT_NAME"}}}},
		scripted.Step{Text: "done"},
	)
	type run struct {
		sid, root, name string
	}
	var runs []run
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := e.SaveEnvironment(SaveEnvironmentRequest{Name: name, Env: map[string]string{"PROJECT_NAME": name}}); err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(t.TempDir(), name)
		os.MkdirAll(root, 0o755)
		ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: name})
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, run{ses.ID, ses.ProjectRoot, name})
	}
	stop := make(chan struct{})
	defer close(stop)
	approveAll(e, stop)
	var wg sync.WaitGroup
	for _, r := range runs {
		wg.Go(func() { e.Prompt(ctx, r.sid, "go") })
	}
	wg.Wait()
	waitIdle(t, ch, runs[0].sid, runs[1].sid, runs[2].sid)

	for _, r := range runs {
		evs, _ := e.Events(ctx, r.sid, 0)
		var out string
		for _, ev := range evs {
			if ev.Type == event.ToolCallUpdate && strings.Contains(string(ev.Data), "name=") {
				out = string(ev.Data)
			}
		}
		if !strings.Contains(out, r.root) || !strings.Contains(out, "name="+r.name) {
			t.Errorf("%s: output %s lacks its own cwd or variable", r.name, out)
		}
		for _, other := range runs {
			if other.name != r.name && strings.Contains(out, "name="+other.name) {
				t.Errorf("%s saw %s's variable", r.name, other.name)
			}
		}
	}
	projects, _ := e.ListProjects(ctx)
	if len(projects) != 3 {
		t.Errorf("projects = %+v", projects)
	}
}

func TestGitStateFollowsTheSession(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, unsub := e.Subscribe()
	defer unsub()
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "new.txt", "content": "x"}}}},
		scripted.Step{Text: "done"},
	)
	if _, err := e.SetProjectTrust(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "s", Model: "m", Mode: "accept_edits"})
	if err != nil {
		t.Fatal(err)
	}
	if ses.Branch != "main" || len(ses.BaseCommit) != 40 {
		t.Errorf("session = %+v", ses)
	}
	e.Prompt(ctx, ses.ID, "write")
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-ch:
			var st event.GitStateData
			if ev.Type == event.GitState && json.Unmarshal(ev.Data, &st) == nil && st.Dirty == 1 {
				if st.BaseCommit != ses.BaseCommit || st.Branch != "main" {
					t.Errorf("git state = %+v", st)
				}
				return
			}
		case <-deadline:
			t.Fatal("no git_state with the new file")
		}
	}
}

func TestProjectsFingerprintAndRelink(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	e := openTest(t, t.TempDir())
	defer e.Close()
	p, err := e.AddProject(ctx, repo)
	if err != nil || p.FirstCommit == "" || p.Remote != "https://example.com/r.git" {
		t.Fatalf("project = %+v, %v", p, err)
	}
	if again, _ := e.AddProject(ctx, repo); again.ID != p.ID {
		t.Error("adding the same folder twice made two projects")
	}
	moved := filepath.Join(filepath.Dir(repo), "moved")
	if err := os.Rename(repo, moved); err != nil {
		t.Fatal(err)
	}
	list, _ := e.ListProjects(ctx)
	if len(list) != 1 || !list[0].Missing {
		t.Fatalf("after move: %+v", list)
	}
	if _, err := e.RelinkProject(ctx, p.ID, t.TempDir()); err == nil || !strings.Contains(err.Error(), "different repository") {
		t.Errorf("relink to an unrelated folder: %v", err)
	}
	if got, err := e.RelinkProject(ctx, p.ID, moved); err != nil || !strings.HasSuffix(got.Root, "moved") {
		t.Errorf("relink = %+v, %v", got, err)
	}
}

// SEC-002: an environment's rules apply to its sessions; a deny there wins
// over a global allow, and saving the environment from the form keeps them.
func TestEnvironmentScopeRules(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	os.MkdirAll(filepath.Join(dir, config.EnvironmentsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.EnvironmentsDir, "ci.yaml"), []byte("name: CI\nrules:\n  - effect: deny\n    tool: shell\n    command: git push *\n"), 0o600)
	e.loadEnvironments()
	approveFiles(t, e)
	if err := e.AddRule(permission.Rule{Effect: permission.Allow, Tool: "shell", Command: "git *"}); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: DemoProviderID, EnvironmentID: "ci"})
	if err != nil {
		t.Fatal(err)
	}
	rules, _ := e.rules.Rules(ses.ID)
	var deny *permission.Rule
	for i := range rules {
		if rules[i].Scope == permission.ScopeEnvironment {
			deny = &rules[i]
		}
	}
	if deny == nil || deny.Source != "environments/ci.yaml" {
		t.Fatalf("rules = %+v", rules)
	}
	d := e.broker.Request(ctx, permission.Context{Mode: permission.ModeAsk, Root: ses.ProjectRoot}, permission.Request{SessionID: ses.ID, Tool: "shell", Kind: permission.KindExecute, Command: "git push origin"})
	if d.Outcome != permission.Denied {
		t.Errorf("git push in the CI environment = %s", d.Outcome)
	}
	if _, err := e.SaveEnvironment(SaveEnvironmentRequest{ID: "ci", Name: "CI renamed"}); err != nil {
		t.Fatal(err)
	}
	if env, _ := e.environment("ci"); len(env.Rules) != 1 {
		t.Error("saving from the form dropped the rules")
	}
}

// A local environment can keep the user's SSH agent from what it runs; the setting
// is saved and read back, and is left out when it is the default.
func TestAnEnvironmentCanKeepTheSSHAgentOut(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	off := false
	nokeys, err := e.SaveEnvironment(SaveEnvironmentRequest{Name: "No keys", SSHAgent: &off})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := e.SaveEnvironment(SaveEnvironmentRequest{Name: "Keys"})
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range e.ListEnvironments() {
		switch env.ID {
		case nokeys.ID:
			if env.SSHAgent == nil || *env.SSHAgent {
				t.Errorf("nokeys shares the agent: %+v", env.SSHAgent)
			}
		case keys.ID:
			if env.SSHAgent != nil {
				t.Errorf("keys stores the default: %v", *env.SSHAgent)
			}
		}
	}
	cfg, err := e.usableEnvironment(nokeys.ID)
	if err != nil || cfg.ShareSSHAgent() {
		t.Errorf("nokeys = %+v, %v", cfg, err)
	}
}
