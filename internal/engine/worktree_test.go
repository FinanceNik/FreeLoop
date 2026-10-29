package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// SES-010: two sessions on one repository edit the same file in their own
// worktrees without interfering; a committed session merges back into the
// main checkout; archive removes a clean worktree and keeps a dirty one;
// unarchive brings it back.
func TestWorktreeSessions(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	gitIn(t, repo, "config", "user.name", "t")
	gitIn(t, repo, "config", "user.email", "t@t")
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	for _, id := range []string{"one", "two"} {
		p := scripted.New(id, scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "a.txt", "content": "from " + id + "\n"}}}}, scripted.Step{Text: "done"})
		e.mu.Lock()
		e.providers.built[id] = &configuredProvider{cfg: config.Provider{ID: id, Name: id, Type: "scripted"}, p: p}
		e.mu.Unlock()
	}

	if _, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "one", Model: "m", Worktree: true}); err == nil {
		t.Fatal("a worktree was made in an untrusted project")
	}
	e.SetProjectTrust(ctx, repo, true)
	a, err := e.CreateSession(ctx, CreateSessionRequest{Title: "Fix A", ProjectRoot: repo, ProviderID: "one", Model: "m", Mode: "accept_edits", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.CreateSession(ctx, CreateSessionRequest{Title: "Fix B", ProjectRoot: repo, ProviderID: "two", Model: "m", Mode: "accept_edits", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if a.Worktree == "" || a.Worktree == b.Worktree || !strings.HasPrefix(a.Branch, BranchPrefix+"fix-a-") || strings.HasPrefix(a.Worktree, e.dataDir+string(filepath.Separator)) {
		t.Fatalf("sessions = %+v / %+v", a, b)
	}
	e.Prompt(ctx, a.ID, "go")
	e.Prompt(ctx, b.ID, "go")
	waitIdle(t, ch, a.ID, b.ID)
	read := func(dir string) string {
		got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
		return string(got)
	}
	if read(a.Worktree) != "from one\n" || read(b.Worktree) != "from two\n" || read(repo) != "a" {
		t.Fatalf("a = %q, b = %q, main = %q", read(a.Worktree), read(b.Worktree), read(repo))
	}

	// Each session's diff panel shows its own change.
	c, _ := e.Changes(ctx, a.ID)
	if len(c.Files) != 1 || c.Files[0].Path != "a.txt" || c.Branch != a.Branch {
		t.Errorf("changes in a = %+v", c)
	}
	if _, err := e.MergeWorktree(ctx, a.ID); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("merging uncommitted work = %v", err)
	}
	e.StageFiles(ctx, a.ID, []string{"a.txt"}, true)
	if _, err := e.Commit(ctx, a.ID, "A's change"); err != nil {
		t.Fatal(err)
	}
	res, err := e.MergeWorktree(ctx, a.ID)
	if err != nil || res.Into != "main" {
		t.Fatalf("merge = %+v, %v", res, err)
	}
	if read(repo) != "from one\n" {
		t.Errorf("main after merge = %q", read(repo))
	}

	// Archive: a clean worktree goes, a dirty one stays.
	if err := e.ArchiveSession(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Worktree); !os.IsNotExist(err) {
		t.Error("the clean worktree was not removed")
	}
	if out := gitIn(t, repo, "branch", "--list", a.Branch); !strings.Contains(out, a.Branch) {
		t.Error("archiving removed the branch")
	}
	e.ArchiveSession(ctx, b.ID, true)
	if _, err := os.Stat(filepath.Join(b.Worktree, "a.txt")); err != nil {
		t.Error("the worktree with uncommitted work was removed")
	}
	if err := e.ArchiveSession(ctx, a.ID, false); err != nil {
		t.Fatal(err)
	}
	if read(a.Worktree) != "from one\n" {
		t.Errorf("unarchived worktree = %q", read(a.Worktree))
	}

	if err := e.SetProjectWorktreeDefault(ctx, a.ProjectID, "always"); err != nil {
		t.Fatal(err)
	}
	if p, _ := e.store.GetProject(ctx, a.ProjectID); p.WorktreeDefault != "always" {
		t.Errorf("project = %+v", p)
	}
}
