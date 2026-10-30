package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// ENV-005: the built-in harness's shell calls run inside the session's
// container (uname says Linux), its file writes land in the project, and
// deleting the session removes the container. Skipped without Docker.
func TestDockerSession(t *testing.T) {
	ctx := context.Background()
	docker, err := environment.FindDocker()
	if err != nil {
		t.Skip(err)
	}
	if err := hostRun(docker, "info"); err != nil {
		t.Skip("no Docker daemon is running")
	}
	cache, _ := os.UserCacheDir()
	root, err := os.MkdirTemp(cache, "freeloop-docker-session-")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(root)

	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	os.MkdirAll(filepath.Join(dir, config.EnvironmentsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.EnvironmentsDir, "box.yaml"), []byte("name: Box\nkind: docker\nimage: alpine:3.20\nhealth: true\nenv:\n  GREETING: hi\n"), 0o600)
	e.loadEnvironments()
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "uname -s > uname.txt; echo $GREETING >> uname.txt"}}}},
		scripted.Step{Text: "done"},
	)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box"})
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	approveAll(e, stop)
	defer close(stop)
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)
	if b, _ := os.ReadFile(filepath.Join(root, "uname.txt")); string(b) != "Linux\nhi\n" {
		t.Fatalf("uname.txt = %q", b)
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	healthy := false
	for _, ev := range evs {
		healthy = healthy || (ev.Type == event.EnvHealth && strings.Contains(string(ev.Data), `"healthy"`))
	}
	if !healthy {
		t.Error("no env_health event")
	}
	if err := e.DeleteSession(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := hostRun(docker, "inspect", containerName(ses.ID)); err == nil {
		t.Error("the container outlived its session")
	}
}

// hostRun runs a host command for the test, through the environment layer.
func hostRun(argv ...string) error {
	l, err := environment.NewLocal(environment.LocalOptions{Root: os.TempDir(), Guard: allowAllGuard{}})
	if err != nil {
		return err
	}
	res, err := l.Exec(context.Background(), environment.ExecSpec{Argv: argv, Timeout: 30 * time.Second})
	if err == nil && res.ExitCode != 0 {
		err = os.ErrNotExist
	}
	return err
}

type allowAllGuard struct{}

func (allowAllGuard) CheckRead(string) error  { return nil }
func (allowAllGuard) CheckWrite(string) error { return nil }

// A worktree session in a Docker environment sees the main checkout's .git
// its worktree points into, so git works in the container; the
// repository's hooks and config are read-only there, so a command cannot
// plant what the user's own git on the host would run. Skipped without
// Docker.
func TestDockerWorktreeGit(t *testing.T) {
	ctx := context.Background()
	docker, err := environment.FindDocker()
	if err != nil {
		t.Skip(err)
	}
	if err := hostRun(docker, "info"); err != nil {
		t.Skip("no Docker daemon is running")
	}
	cache, _ := os.UserCacheDir()
	base, err := os.MkdirTemp(cache, "freeloop-docker-worktree-")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(base)
	root := filepath.Join(base, "repo")
	os.MkdirAll(root, 0o755)
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "config", "user.name", "t")
	gitIn(t, root, "config", "user.email", "t@t")
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644)
	gitIn(t, root, "add", "a.txt")
	gitIn(t, root, "commit", "-qm", "init")
	common := environment.Canonical(filepath.Join(root, ".git"))

	dir := filepath.Join(base, "data")
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	os.MkdirAll(filepath.Join(dir, config.EnvironmentsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.EnvironmentsDir, "box.yaml"), []byte("name: Box\nkind: docker\nimage: alpine:3.20\n"), 0o600)
	e.loadEnvironments()
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "test -f " + common + "/HEAD && echo seen > seen.txt; " +
			"touch " + common + "/hooks/pre-commit 2>/dev/null && echo hook >> wrote.txt; echo '[core]' >> " + common + "/config 2>/dev/null && echo config >> wrote.txt; true"}}}},
		scripted.Step{Text: "done"},
	)
	e.SetProjectTrust(ctx, root, true)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m", EnvironmentID: "box", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.DeleteSession(ctx, ses.ID)
	stop := make(chan struct{})
	approveAll(e, stop)
	defer close(stop)
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)
	if b, _ := os.ReadFile(filepath.Join(ses.Worktree, "seen.txt")); string(b) != "seen\n" {
		t.Errorf("the container does not see the main checkout's .git (seen.txt = %q)", b)
	}
	if b, _ := os.ReadFile(filepath.Join(ses.Worktree, "wrote.txt")); len(b) > 0 {
		t.Errorf("the container wrote the repository's %s", b)
	}
	if _, err := os.Stat(filepath.Join(common, "hooks", "pre-commit")); err == nil {
		t.Error("a hook was planted")
	}
}
