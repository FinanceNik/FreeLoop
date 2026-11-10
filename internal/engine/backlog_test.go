package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/backlog"
	"github.com/FinanceNik/Freeloop/internal/config"
)

// writeIssue writes an issue file into a project's backlog.
func writeIssue(t *testing.T, root, id, front string) {
	t.Helper()
	p := filepath.Join(root, "backlog", "issues", id+".md")
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte("---\n"+front+"\n---\n\n## What\n"+id+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// backlogRepo is a committed repository with a PRD and two AFK issues, the second
// blocked by the first.
func backlogRepo(t *testing.T) string {
	t.Helper()
	root := gitRepo(t)
	os.MkdirAll(filepath.Join(root, "backlog", "prds"), 0o755)
	os.WriteFile(filepath.Join(root, "backlog", "prds", "drafts.md"), []byte("---\ntitle: Drafts\n---\n"), 0o644)
	writeIssue(t, root, "001-store", "title: Store a draft\nstatus: open\ntype: afk\nprd: drafts")
	writeIssue(t, root, "002-search", "title: Search drafts\nstatus: open\ntype: afk\nblockedBy: [001-store]\nprd: drafts")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "backlog")
	return root
}

// WFL-004: a trusted project's backlog is read, with what blocks what; an untrusted
// one's is not.
func TestBacklogNeedsTrust(t *testing.T) {
	ctx := context.Background()
	e, _ := queueEngine(t)
	root := backlogRepo(t)
	if _, err := e.AddProject(ctx, root); err != nil {
		t.Fatal(err)
	}
	// An untrusted project with a backlog folder gets a row to trust it by, and nothing in the
	// folder is read for it: the folder is only looked for (SEC-006).
	list, _ := e.Backlogs(ctx)
	if len(list) != 1 || list[0].Trusted || len(list[0].Issues) != 0 || len(list[0].PRDs) != 0 || list[0].Finished != 0 || list[0].Folder != "backlog" || list[0].ProjectRoot == "" || list[0].ProjectName == "" {
		t.Errorf("an untrusted project's row: %+v", list)
	}
	// A project without a backlog folder has no row, trusted or not.
	other := gitRepo(t)
	e.AddProject(ctx, other)
	if list, _ := e.Backlogs(ctx); len(list) != 1 {
		t.Errorf("a project with no backlog got a row: %+v", list)
	}
	e.SetProjectTrust(ctx, other, true)
	if list, _ := e.Backlogs(ctx); len(list) != 1 {
		t.Errorf("a trusted project with no backlog got a row: %+v", list)
	}
	if _, err := e.Backlog(ctx, root, ""); err == nil {
		t.Error("an untrusted project's backlog was read")
	}
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	list, err := e.Backlogs(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("backlogs = %+v, %v", list, err)
	}
	b := list[0]
	if !b.Trusted {
		t.Error("a trusted project's row says it is not trusted")
	}
	if b.WorkflowID != config.BuiltinWorkflowID || b.Folder != "backlog" || len(b.Issues) != 2 || len(b.PRDs) != 1 || b.Uncommitted {
		t.Fatalf("backlog = %+v", b)
	}
	if b.Issues[0].State != backlog.Ready || b.Issues[1].State != backlog.Blocked || strings.Join(b.Issues[0].Blocks, ",") != "002-search" {
		t.Errorf("issues = %+v", b.Issues)
	}
	writeIssue(t, root, "003-new", "title: New\ntype: human")
	if b, _ := e.Backlog(ctx, root, ""); !b.Uncommitted || len(b.Issues) != 3 {
		t.Errorf("an uncommitted issue: %+v", b)
	}
}

// WFL-004, OQ-041: "Archive finished" removes done issues and finished PRDs from the
// project in a commit of its own, which git history keeps, and leaves the rest.
func TestArchiveBacklog(t *testing.T) {
	ctx := context.Background()
	e, _ := queueEngine(t)
	root := backlogRepo(t)
	e.AddProject(ctx, root)
	e.SetProjectTrust(ctx, root, true)
	if _, err := e.ArchiveBacklog(ctx, root, ""); err == nil {
		t.Error("archived with nothing finished")
	}
	writeIssue(t, root, "001-store", "title: Store a draft\nstatus: done\ntype: afk\nprd: drafts")
	if _, err := e.ArchiveBacklog(ctx, root, ""); err == nil || !strings.Contains(err.Error(), "commit") {
		t.Errorf("an uncommitted done issue: %v", err)
	}
	gitIn(t, root, "commit", "-qam", "done")
	res, err := e.ArchiveBacklog(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Moved, ",") != "backlog/issues/001-store.md" {
		t.Errorf("moved %v", res.Moved)
	}
	if _, err := os.Stat(filepath.Join(root, "backlog", "issues", "001-store.md")); !os.IsNotExist(err) {
		t.Error("the done issue is still in the project")
	}
	if got := gitIn(t, root, "show", res.Commit+"^:backlog/issues/001-store.md"); !strings.Contains(got, "status: done") {
		t.Errorf("git history does not keep it: %q", got)
	}
	if _, err := os.Stat(filepath.Join(e.dataDir, "archive")); !os.IsNotExist(err) {
		t.Error("a copy was kept in the data folder")
	}
	if st := gitIn(t, root, "status", "--porcelain"); strings.TrimSpace(st) != "" {
		t.Errorf("the removal was not committed: %s", st)
	}
	if !strings.Contains(gitIn(t, root, "log", "-1", "--format=%s"), "Archive 1 finished") {
		t.Error("no archive commit")
	}
	if _, err := os.Stat(filepath.Join(root, "backlog", "issues", "002-search.md")); err != nil {
		t.Error("an open issue was moved")
	}
}
