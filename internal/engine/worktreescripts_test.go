package engine

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// SES-016: a new worktree gets the ignored files it names and its setup
// before its first prompt runs; each worktree session has its own ports; a
// failed setup holds the session until the user retries or skips it; the
// archive command runs before the worktree goes.
func TestWorktreeScripts(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	gitIn(t, repo, "config", "user.name", "t")
	gitIn(t, repo, "config", "user.email", "t@t")
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".env\nsetup.txt\nport.txt\n"), 0o644)
	gitIn(t, repo, "add", ".gitignore")
	gitIn(t, repo, "commit", "-qm", "ignore")
	os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), 0o600)
	os.MkdirAll(filepath.Join(repo, ProjectDir), 0o755)
	os.WriteFile(filepath.Join(repo, ProjectDir, config.WorktreeFileName), []byte(
		"copy: [.env, missing.json]\nsetup: sleep 0.3; echo set up > setup.txt; echo $FREELOOP_PORT > port.txt\narchive: touch \"$FREELOOP_ROOT_PATH/archived.txt\"\n"), 0o644)

	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	p := scripted.New("sp", scripted.Step{Text: "hello"}, scripted.Step{Text: "again"})
	e.mu.Lock()
	e.providers.built["sp"] = &configuredProvider{cfg: config.Provider{ID: "sp", Name: "sp", Type: "scripted"}, p: p}
	e.mu.Unlock()
	if info, _ := e.ProjectSettings(ctx, repo); info.Worktree.Setup == "" {
		t.Error("the trust prompt does not show the setup command")
	}
	e.SetProjectTrust(ctx, repo, true)

	a, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "sp", Model: "m", Mode: "ask", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "sp", Model: "m", Mode: "ask", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if a.PortBase < FirstPort || b.PortBase == a.PortBase {
		t.Errorf("ports = %d, %d", a.PortBase, b.PortBase)
	}
	// The prompt starts only once the setup has written its file.
	if _, err := e.Prompt(ctx, a.ID, "hi"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, a.ID)
	if got, _ := os.ReadFile(filepath.Join(a.Worktree, "setup.txt")); string(got) != "set up\n" {
		t.Errorf("setup.txt = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(a.Worktree, "port.txt")); strings.TrimSpace(string(got)) != strconv.Itoa(a.PortBase) {
		t.Errorf("port.txt = %q, want %d", got, a.PortBase)
	}
	if got, _ := os.ReadFile(filepath.Join(a.Worktree, ".env")); string(got) != "SECRET=1\n" {
		t.Errorf(".env = %q", got)
	}
	evs, _ := e.Events(ctx, a.ID, 0)
	var done string
	for _, ev := range evs {
		if strings.Contains(string(ev.Data), `"status":"done"`) && ev.Type == "worktree_setup" {
			done = string(ev.Data)
		}
	}
	if !strings.Contains(done, `"copied":[".env"]`) || !strings.Contains(done, `"missing":["missing.json"]`) {
		t.Errorf("setup event = %s", done)
	}

	// A failing setup holds the session until it is skipped.
	if err := e.SetProjectScripts(ctx, a.ProjectID, config.WorktreeScripts{Setup: "echo broken; exit 3"}); err != nil {
		t.Fatal(err)
	}
	c, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "sp", Model: "m", Mode: "ask", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, ch, func(ev event.Event) bool {
		return ev.SID == c.ID && ev.Type == "worktree_setup" && strings.Contains(string(ev.Data), "failed")
	})
	if _, err := e.Prompt(ctx, c.ID, "hi"); err == nil || !strings.Contains(err.Error(), "worktree setup failed") {
		t.Errorf("prompt after a failed setup = %v", err)
	}
	e.SkipSetup(ctx, c.ID)
	if _, err := e.Prompt(ctx, c.ID, "hi"); err != nil {
		t.Errorf("prompt after skipping = %v", err)
	}
	waitIdle(t, ch, c.ID)
	info, _ := e.ProjectScripts(ctx, a.ProjectID)
	if info.Effective.Setup != "echo broken; exit 3" || info.Effective.Run != "" || len(info.Effective.Copy) != 2 {
		t.Errorf("effective scripts = %+v", info.Effective)
	}

	// Archiving runs the archive command.
	if err := e.ArchiveSession(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(repo, "archived.txt")); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the archive command did not run")
}

// standInDocker puts a docker on PATH that records its calls in the returned
// file and keeps one container's state.
func standInDocker(t *testing.T) (calls string) {
	t.Helper()
	dir := t.TempDir()
	calls, state := filepath.Join(dir, "calls"), filepath.Join(dir, "state")
	os.WriteFile(filepath.Join(dir, "docker"), []byte(`#!/bin/sh
echo "$*" >> '`+calls+`'
case "$1" in
inspect) cat '`+state+`' 2>/dev/null || exit 1 ;;
run|start) echo running > '`+state+`' ;;
stop) echo exited > '`+state+`' ;;
exec) case "$*" in *" ls -A "*) echo a.txt ;; esac ;;
esac
`), 0o755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

// OQ-026: in a Docker environment the setup and archive commands run in the
// session's container, never on the Mac, where they could run what the
// container wrote into the worktree; it stops again when no session uses it.
// The copy still comes from the main checkout, on the host.
func TestScriptsRunInTheContainer(t *testing.T) {
	ctx := context.Background()
	calls := standInDocker(t)
	repo := gitRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	os.MkdirAll(filepath.Join(repo, ProjectDir), 0o755)
	os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), 0o600)
	os.WriteFile(filepath.Join(repo, ProjectDir, config.WorktreeFileName), []byte(
		"copy: [.env]\nsetup: touch "+marker+"-setup\narchive: touch "+marker+"-archive\n"), 0o644)
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	e.SetProjectTrust(ctx, repo, true)
	os.MkdirAll(filepath.Join(dir, config.EnvironmentsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.EnvironmentsDir, "box.yaml"), []byte("name: Box\nkind: docker\nimage: alpine:3.20\n"), 0o600)
	e.loadEnvironments()

	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID, EnvironmentID: "box", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.waitSetup(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	log := func() string {
		b, _ := os.ReadFile(calls)
		return string(b)
	}
	ranIn := func(what string) {
		t.Helper()
		for _, line := range strings.Split(log(), "\n") {
			if strings.HasPrefix(line, "exec ") && strings.Contains(line, marker+"-"+what) {
				return
			}
		}
		t.Errorf("the %s command did not run in the container:\n%s", what, log())
	}
	ranIn("setup")
	if !strings.HasSuffix(strings.TrimSpace(log()), containerName(ses.ID)) || !strings.Contains(log(), "stop ") {
		t.Errorf("the container was left running:\n%s", log())
	}
	if b, err := os.ReadFile(filepath.Join(ses.Worktree, ".env")); err != nil || string(b) != "SECRET=1\n" {
		t.Errorf("the copied file: %q, %v", b, err)
	}

	// A retry after the container has run stays in the container too.
	os.Remove(calls)
	if err := e.RetrySetup(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.waitSetup(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	ranIn("setup")

	os.Remove(calls)
	if err := e.ArchiveSession(ctx, ses.ID, true); err != nil {
		t.Fatal(err)
	}
	ranIn("archive")
	for _, what := range []string{"setup", "archive"} {
		if _, err := os.Stat(marker + "-" + what); err == nil {
			t.Errorf("the %s command ran on the Mac", what)
		}
	}

	// A container that never started has nothing to clean up.
	os.WriteFile(filepath.Join(repo, ProjectDir, config.WorktreeFileName), []byte("archive: touch "+marker+"-archive\n"), 0o644)
	other, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID, EnvironmentID: "box", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(calls)
	e.runArchiveScript(ctx, other)
	if strings.Contains(log(), "exec ") || strings.Contains(log(), "run ") {
		t.Errorf("a container started for nothing:\n%s", log())
	}

	// A worktree a container has had does not go back to the Mac.
	moved := other
	moved.EnvironmentID = ""
	e.emit(other.ID, event.EnvHealth, map[string]string{"status": "healthy", "container": containerName(other.ID)})
	if _, _, err := e.scriptHost(ctx, moved); err == nil {
		t.Error("a worktree a container had would run its commands on the Mac")
	}
}
