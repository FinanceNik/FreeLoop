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
