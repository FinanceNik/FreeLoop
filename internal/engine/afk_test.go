package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// afkEngine is an engine whose profile "main" runs on a scripted provider that plays
// the built-in workflow's stages, with a trusted project holding a committed backlog
// of two AFK issues, the second blocked by the first.
func afkEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	e := openBinding(t, t.TempDir(), secrets.NewMemory())
	withScript(e, "s")
	if err := e.SaveProfileYAML("main", "name: Main\nprovider: s\nmodel: m\n"); err != nil {
		t.Fatal(err)
	}
	root := backlogRepo(t)
	gitIn(t, root, "config", "user.name", "t")
	gitIn(t, root, "config", "user.email", "t@t")
	ctx := context.Background()
	if _, err := e.AddProject(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	if err := e.SetProjectProfile(ctx, mustProject(t, e, root).ID, "main"); err != nil {
		t.Fatal(err)
	}
	return e, root
}

func mustProject(t *testing.T, e *Engine, root string) ProjectInfo {
	t.Helper()
	list, err := e.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.Root == root || filepath.Base(p.Root) == filepath.Base(root) {
			return p
		}
	}
	t.Fatalf("no project at %s", root)
	return ProjectInfo{}
}

// waitRun waits until the run is no longer running, and returns it.
func waitRun(t *testing.T, e *Engine, id string) config.AFKRun {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		e.KickRuns()
		runs, _ := e.AFKRuns(context.Background())
		for _, r := range runs {
			if r.ID == id && !r.Active() {
				return r
			}
			if r.ID == id && time.Now().After(deadline) {
				t.Fatalf("the run is still %s: %+v", r.State, r)
			}
		}
		if len(e.PendingPermissions()) > 0 {
			t.Fatalf("an unattended session asked: %+v", e.PendingPermissions()[0])
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// WFL-005, WFL-006: a run does the ready AFK issue, then the one it unblocked, each in
// a fresh session of its own in the run's one worktree, with only the commands the
// user allowed up front, and lists both for QA; the user's checkout is untouched.
func TestAFKRunDoesTheIssuesInOrder(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	plan, err := e.PrepareAFK(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Problem != "" || plan.Next == nil || plan.Next.ID != "001-store" || plan.Ready != 1 || plan.Waiting != 1 || plan.Mode != "accept_edits" {
		t.Fatalf("plan = %+v", plan)
	}
	var offered []string
	for _, c := range plan.Commands {
		offered = append(offered, c.Command+"="+c.Outcome)
	}
	if !slices.Contains(offered, "git commit *=ask") || !slices.Contains(offered, "git add *=ask") {
		t.Errorf("offered %v", offered)
	}
	if _, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"rm -rf *"}}); err == nil {
		t.Error("a command the plan did not offer was allowed")
	}
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"git add *", "git commit *"}, StartedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root}); err == nil {
		t.Error("a second run started in the same project")
	}
	run = waitRun(t, e, run.ID)
	if run.State != config.RunFinished || run.Reason != "Every AFK issue is done." {
		t.Fatalf("run = %+v", run)
	}
	if len(run.Steps) != 2 || run.Steps[0].Issue != "001-store" || run.Steps[1].Issue != "002-search" {
		t.Fatalf("steps = %+v", run.Steps)
	}
	var worktrees []string
	for i, s := range run.Steps {
		if s.State != config.StepQA || s.Commit == "" {
			t.Errorf("step %+v", s)
		}
		// Each was reviewed, read-only, in the run's worktree, before the next began.
		rev, err := e.store.GetSession(ctx, s.ReviewID)
		if err != nil || rev.Mode != "plan" || rev.ParentID != s.SessionID || !slices.Contains(rev.Tags, ReviewTag) {
			t.Errorf("review of %s: %+v %v", s.Issue, rev, err)
		}
		if i > 0 {
			if prev, _ := e.store.GetSession(ctx, run.Steps[i-1].ReviewID); !s.StartedAt.After(prev.CreatedAt) {
				t.Errorf("%s started before the review of the one before", s.Issue)
			}
		}
		ses, err := e.store.GetSession(ctx, s.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		worktrees = append(worktrees, ses.Worktree)
		if !slices.Contains(ses.Tags, AFKTag) || ses.Mode != "accept_edits" {
			t.Errorf("session %s: tags %v, mode %s", ses.ID, ses.Tags, ses.Mode)
		}
		// The allowed commands are the session's own rules, and nothing more.
		var rules []string
		evs, _ := e.store.Events(ctx, ses.ID, 0)
		for _, ev := range evs {
			if ev.Type == event.SessionRuleChanged {
				rules = append(rules, string(ev.Data))
			}
		}
		if len(rules) != 2 || !strings.Contains(rules[1], "git commit *") {
			t.Errorf("session rules %v", rules)
		}
	}
	if worktrees[0] == "" || worktrees[0] != worktrees[1] || worktrees[0] != run.Worktree || run.Branch == "" {
		t.Errorf("worktrees %v, run's %s on %s", worktrees, run.Worktree, run.Branch)
	}
	// The issues are done on the run's branch, with a commit each, and not in the
	// user's checkout.
	b, _ := os.ReadFile(filepath.Join(run.Worktree, "backlog", "issues", "002-search.md"))
	if !strings.Contains(string(b), "status: done") {
		t.Errorf("002 in the worktree:\n%s", b)
	}
	log := gitIn(t, run.Worktree, "log", "--format=%s", "-n", "3")
	if !strings.Contains(log, "001-store: done") || !strings.Contains(log, "002-search: done") {
		t.Errorf("log:\n%s", log)
	}
	b, _ = os.ReadFile(filepath.Join(root, "backlog", "issues", "001-store.md"))
	if !strings.Contains(string(b), "status: open") {
		t.Error("the run changed the user's checkout")
	}
	// Both wait for QA in the attention view, until QA'd.
	items, _ := e.Attention(ctx)
	var qa []string
	for _, it := range items {
		if it.QA != nil {
			qa = append(qa, it.QA.Issue)
		}
	}
	if strings.Join(qa, ",") != "001-store,002-search" {
		t.Errorf("ready for QA: %v", qa)
	}
	for _, it := range items {
		if it.QA != nil && !strings.Contains(it.QA.Reply, "found nothing wrong") {
			t.Errorf("%s's review reply: %q", it.QA.Issue, it.QA.Reply)
		}
	}
	if err := e.QADone(ctx, run.ID, "001-store"); err != nil {
		t.Fatal(err)
	}
	items, _ = e.Attention(ctx)
	if slices.ContainsFunc(items, func(it AttentionItem) bool { return it.QA != nil && it.QA.Issue == "001-store" }) {
		t.Error("a QA'd issue is still listed")
	}
}

// WFL-005: a session that ends without marking its issue done stops the run, which
// says so, and the issue after it is not started.
func TestAFKRunStopsAtAnIssueNotDone(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	// The scripted agent marks an issue done by changing "status: open"; this one
	// says todo, which is open too, so its edit fails.
	writeIssue(t, root, "001-store", "title: Store a draft\nstatus: todo\ntype: afk\nprd: drafts")
	gitIn(t, root, "commit", "-qam", "todo")
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"git add *", "git commit *"}})
	if err != nil {
		t.Fatal(err)
	}
	run = waitRun(t, e, run.ID)
	if run.State != config.RunStopped || !strings.Contains(run.Reason, "001-store's session ended without marking it done") {
		t.Fatalf("run = %+v", run)
	}
	if len(run.Steps) != 1 || run.Steps[0].State != config.StepFailed {
		t.Errorf("steps = %+v", run.Steps)
	}
}

// WFL-005: the cap ends a run, a paused run starts nothing until resumed, and an
// uncommitted backlog refuses to start one.
func TestAFKRunCapPauseAndUncommitted(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	dir := filepath.Join(e.dataDir, config.WorkflowsDir)
	os.MkdirAll(dir, 0o700)
	w := config.BuiltinWorkflows()[0]
	body := "name: One at most\nstages:\n  - {command: afk, title: Run, kind: afk}\nafk:\n  maxIssues: 1\n  commands: [\"git add *\", \"git commit *\"]\n  prompt: |\n" + indent(w.AFK.Prompt) + "review:\n  off: true\n"
	os.WriteFile(filepath.Join(dir, "one.yaml"), []byte(body), 0o600)
	e.loadWorkflows()
	if _, err := e.PrepareAFK(ctx, root, "one"); err == nil {
		t.Error("an unapproved workflow was prepared")
	}
	for _, s := range e.ListWorkflows(ctx) {
		if s.ID == "one" {
			if _, err := e.ApproveWorkflow(ctx, "one", s.Fingerprint); err != nil {
				t.Fatal(err)
			}
		}
	}
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, WorkflowID: "one", Allow: []string{"git add *", "git commit *"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.PauseAFK(ctx, run.ID, true); err != nil {
		t.Fatal(err)
	}
	e.moveQueue()
	if runs, _ := e.AFKRuns(ctx); len(runs[0].Steps) > 1 {
		t.Errorf("a paused run went on: %+v", runs[0].Steps)
	}
	if _, err := e.PauseAFK(ctx, run.ID, false); err != nil {
		t.Fatal(err)
	}
	run = waitRun(t, e, run.ID)
	if run.State != config.RunFinished || !strings.Contains(run.Reason, "cap of 1") || len(run.Steps) != 1 {
		t.Fatalf("run = %+v", run)
	}
	// Review off: the issue goes straight to QA.
	if run.Steps[0].State != config.StepQA || run.Steps[0].ReviewID != "" {
		t.Errorf("step = %+v", run.Steps[0])
	}

	writeIssue(t, root, "003-new", "title: New\ntype: afk")
	plan, _ := e.PrepareAFK(ctx, root, "")
	if !strings.Contains(plan.Problem, "Commit backlog first") {
		t.Errorf("an uncommitted backlog: %q", plan.Problem)
	}
	if _, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root}); err == nil {
		t.Error("a run started on an uncommitted backlog")
	}
}

// WFL-005: stopping a run takes its queued issue off the queue.
func TestAFKRunStop(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	if _, err := e.SetQueue(true, 0); err != nil {
		t.Fatal(err)
	}
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; {
		runs, _ := e.AFKRuns(ctx)
		if len(runs[0].Steps) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the first issue was not queued")
		}
		e.KickRuns()
		time.Sleep(20 * time.Millisecond)
	}
	if plan, _ := e.PrepareAFK(ctx, root, ""); !plan.QueuePaused {
		t.Error("the plan does not say the queue is paused")
	}
	// The run says why its issue has not started, for the Board.
	for deadline := time.Now().Add(5 * time.Second); ; {
		runs, _ := e.AFKRuns(ctx)
		if runs[0].Steps[0].Waiting == "the queue is paused" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting = %q", runs[0].Steps[0].Waiting)
		}
		e.KickRuns()
		time.Sleep(20 * time.Millisecond)
	}
	if q, _ := e.Queue(); len(q.Tasks) != 1 || q.Tasks[0].RunID != run.ID {
		t.Fatalf("queue = %+v", q.Tasks)
	}
	run, err = e.StopAFK(ctx, run.ID)
	if err != nil || run.State != config.RunStopped {
		t.Fatalf("stop: %+v %v", run, err)
	}
	if q, _ := e.Queue(); len(q.Tasks) != 0 {
		t.Errorf("the queued issue stayed: %+v", q.Tasks)
	}
	if run.Steps[0].State != config.StepFailed {
		t.Errorf("step = %+v", run.Steps[0])
	}
	_ = permission.Allow
}

func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("    " + line + "\n")
	}
	return b.String()
}

// WFL-006: /review has the workflow's review profile review the session's changes.
func TestReviewStage(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "main", Workflow: config.BuiltinWorkflowID})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "change.txt"), []byte("a change\n"), 0o644)
	res, err := e.RunStage(ctx, ses.ID, "review", "the naming")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := e.store.GetSession(ctx, res.SessionID)
	if err != nil || rev.Mode != "plan" || rev.ParentID != ses.ID {
		t.Errorf("review = %+v, %v", rev, err)
	}
	if _, err := e.RunStage(ctx, ses.ID, "afk", ""); err == nil || !strings.Contains(err.Error(), "dialog") {
		t.Errorf("/afk from the engine: %v", err)
	}
}

// WFL-009: the plan offers the project's other check targets, and the user can allow
// commands of their own, though never everything at once.
func TestAFKRunOwnCommands(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	os.WriteFile(filepath.Join(root, "Makefile"), []byte("test:\n\tgo test ./...\ncheck-app:\n\t./check.sh\nbuild:\n\tgo build\n"), 0o644)
	gitIn(t, root, "add", "Makefile")
	gitIn(t, root, "commit", "-qm", "make")
	plan, err := e.PrepareAFK(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	from := map[string]string{}
	for _, c := range plan.Commands {
		from[c.Command] = c.From
	}
	if from["make test"] != "project" || from["make check-app"] != "project-more" || from["make build"] != "" {
		t.Errorf("offered %v", from)
	}
	for _, bad := range []string{"*", "* rm", "  "} {
		if _, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Extra: []string{bad}}); err == nil {
			t.Errorf("%q was allowed", bad)
		}
	}
	if _, err := e.SetQueue(true, 0); err != nil {
		t.Fatal(err)
	}
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, Allow: []string{"git commit *"}, Extra: []string{"  npm   run e2e "}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(run.Allow, "|") != "git commit *|npm run e2e" {
		t.Errorf("allow = %v", run.Allow)
	}
	e.StopAFK(ctx, run.ID)
}
