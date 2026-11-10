package engine

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/backlog"
	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// A workflow's backlog (WFL-004): PRDs and issues as Markdown files in a project
// folder agents write like any other (OQ-039). The app reads them only in a trusted
// project (SEC-006), through the environment (ENV-001), since what they say goes into
// prompts that run unattended.

const (
	// maxBacklogFiles bounds what one read of a backlog looks at.
	maxBacklogFiles = 500
	// maxBacklogFile bounds one file; a bigger one is not an issue.
	maxBacklogFile = 256 << 10
)

// BacklogInfo is a project's backlog as the Board shows it.
type BacklogInfo struct {
	ProjectID    string `json:"projectId"`
	ProjectRoot  string `json:"projectRoot"`
	ProjectName  string `json:"projectName"`
	WorkflowID   string `json:"workflowId"`
	WorkflowName string `json:"workflowName"`
	// Trusted says the project is trusted, so its backlog was read. An untrusted project
	// with a backlog folder has a row with nothing in it, for the user to trust it by.
	Trusted bool           `json:"trusted"`
	Folder  string         `json:"folder"`
	Issues  []backlog.Item `json:"issues"`
	PRDs    []backlog.PRD  `json:"prds"`
	// Finished counts the files "Archive finished" would move.
	Finished int `json:"finished"`
	// Uncommitted says the backlog has changes git has not committed, which a run,
	// working from the last commit, would not see.
	Uncommitted bool `json:"uncommitted"`
	// Run is the project's AFK run going on, or its last one (WFL-005).
	Run *config.AFKRun `json:"run"`
}

// readBacklog reads a backlog folder through env. A missing folder is an empty
// backlog.
func readBacklog(env environment.Environment, folder string) ([]backlog.Issue, []backlog.PRD, error) {
	read := func(sub string, each func(rel string, b []byte)) error {
		dir := path.Join(folder, sub)
		entries, err := env.FS().ReadDir(dir)
		if err != nil {
			if _, statErr := env.FS().Stat(dir); statErr != nil {
				return nil
			}
			return err
		}
		n := 0
		for _, fi := range entries {
			if fi.IsDir || !strings.HasSuffix(fi.Name, ".md") || strings.HasPrefix(fi.Name, ".") || fi.Size > maxBacklogFile {
				continue
			}
			if n++; n > maxBacklogFiles {
				return fmt.Errorf("%s holds more than %d files", dir, maxBacklogFiles)
			}
			rel := path.Join(dir, fi.Name)
			b, err := env.FS().ReadFile(rel)
			if err != nil {
				continue
			}
			each(rel, b)
		}
		return nil
	}
	var issues []backlog.Issue
	var prds []backlog.PRD
	if err := read(backlog.IssuesDir, func(rel string, b []byte) { issues = append(issues, backlog.ParseIssue(rel, b)) }); err != nil {
		return nil, nil, err
	}
	if err := read(backlog.PRDsDir, func(rel string, b []byte) { prds = append(prds, backlog.ParsePRD(rel, b)) }); err != nil {
		return nil, nil, err
	}
	slices.SortFunc(prds, func(a, b backlog.PRD) int { return strings.Compare(a.ID, b.ID) })
	return issues, prds, nil
}

// projectBacklog reads the backlog of a workflow in the folder root, which is a
// trusted project's checkout or a worktree of it.
func (e *Engine) projectBacklog(ctx context.Context, project session.Project, root string, w config.Workflow) (BacklogInfo, error) {
	info := BacklogInfo{ProjectID: project.ID, ProjectRoot: project.Root, ProjectName: project.Name, WorkflowID: w.ID, WorkflowName: w.Name, Folder: w.BacklogDir(), Issues: []backlog.Item{}, PRDs: []backlog.PRD{}}
	if !project.Trusted {
		return info, errors.New("the backlog is read only in a trusted project")
	}
	info.Trusted = true
	env, err := e.localEnv(root, nil)
	if err != nil {
		return info, err
	}
	issues, prds, err := readBacklog(env, w.BacklogDir())
	if err != nil {
		return info, err
	}
	info.Issues, info.PRDs = backlog.Analyze(issues), prds
	if info.PRDs == nil {
		info.PRDs = []backlog.PRD{}
	}
	for _, it := range info.Issues {
		if it.State == backlog.Done {
			info.Finished++
		}
	}
	info.Finished += len(backlog.Finished(info.PRDs, info.Issues))
	if blocked(ctx, env) == "" {
		if out, ok := git(ctx, env, "status", "--porcelain", "--untracked-files=all", "--", w.BacklogDir()); ok && strings.TrimSpace(out) != "" {
			info.Uncommitted = true
		}
	}
	return info, nil
}

// backlogWorkflow is the workflow whose backlog a project shows: the one asked for,
// else the default, else the first usable one with a backlog folder in the project.
func (e *Engine) backlogWorkflow(project session.Project, id string) (config.Workflow, error) {
	if id != "" {
		return e.usableWorkflow(id)
	}
	ids := []string{e.Config().Defaults.Workflow, config.BuiltinWorkflowID}
	for _, w := range e.workflowList() {
		ids = append(ids, w.ID)
	}
	var first config.Workflow
	for _, wid := range ids {
		w, err := e.usableWorkflow(wid)
		if err != nil {
			continue
		}
		if first.ID == "" {
			first = w
		}
		if environment.IsDir(filepath.Join(project.Root, filepath.FromSlash(w.BacklogDir()))) {
			return w, nil
		}
	}
	if first.ID == "" {
		return first, errors.New("no workflow can be used")
	}
	return first, nil
}

// Backlog reads the backlog of the project at root; workflowID chooses whose, empty
// for the project's usual one.
func (e *Engine) Backlog(ctx context.Context, root, workflowID string) (BacklogInfo, error) {
	project, err := e.store.ProjectByRoot(ctx, environment.Canonical(root))
	if err != nil {
		return BacklogInfo{}, err
	}
	w, err := e.backlogWorkflow(project, workflowID)
	if err != nil {
		return BacklogInfo{}, err
	}
	info, err := e.projectBacklog(ctx, project, project.Root, w)
	info.Run = e.lastRun(project.ID)
	return info, err
}

// Backlogs lists the backlogs of trusted projects that have one, for the Board.
func (e *Engine) Backlogs(ctx context.Context) ([]BacklogInfo, error) {
	projects, err := e.store.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := []BacklogInfo{}
	for _, p := range projects {
		if !environment.IsDir(p.Root) {
			continue
		}
		w, err := e.backlogWorkflow(p, "")
		if err != nil || !environment.IsDir(filepath.Join(p.Root, filepath.FromSlash(w.BacklogDir()))) {
			continue
		}
		if !p.Trusted {
			// Only that the folder is there, which is not reading it: nothing in an
			// untrusted project is (SEC-006). The row lets the user trust the project.
			out = append(out, BacklogInfo{ProjectID: p.ID, ProjectRoot: p.Root, ProjectName: p.Name, WorkflowID: w.ID, WorkflowName: w.Name, Folder: w.BacklogDir(), Issues: []backlog.Item{}, PRDs: []backlog.PRD{}})
			continue
		}
		info, err := e.projectBacklog(ctx, p, p.Root, w)
		if err != nil {
			continue
		}
		info.Run = e.lastRun(p.ID)
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b BacklogInfo) int { return strings.Compare(a.ProjectName, b.ProjectName) })
	return out, nil
}

// ArchiveResult says what "Archive finished" moved.
type ArchiveResult struct {
	Moved []string `json:"moved"`
	// Left are finished files it left, with why.
	Left []string `json:"left"`
	// Commit is the commit that removed them, where git history keeps them.
	Commit string `json:"commit"`
}

// ArchiveBacklog takes a backlog's done issues, and PRDs whose issues are all done, out
// of the project (WFL-004, OQ-041): git removes them in a commit of their own, which
// names them, so later agents are not misled by stale docs and git history keeps them
// (git log -- backlog/). No copy is kept elsewhere. Only files committed as they are
// now are removed, so nothing unsaved is lost.
func (e *Engine) ArchiveBacklog(ctx context.Context, root, workflowID string) (ArchiveResult, error) {
	res := ArchiveResult{Moved: []string{}, Left: []string{}}
	project, err := e.store.ProjectByRoot(ctx, environment.Canonical(root))
	if err != nil {
		// A worktree's root is not a project's: find the project it belongs to.
		project, err = e.projectOfWorktree(ctx, root)
		if err != nil {
			return res, err
		}
	}
	w, err := e.backlogWorkflow(project, workflowID)
	if err != nil {
		return res, err
	}
	info, err := e.projectBacklog(ctx, project, root, w)
	if err != nil {
		return res, err
	}
	env, err := e.localEnv(root, nil)
	if err != nil {
		return res, err
	}
	if why := blocked(ctx, env); why != "" {
		return res, errors.New(why)
	}
	var paths []string
	for _, it := range info.Issues {
		if it.State == backlog.Done {
			paths = append(paths, it.Path)
		}
	}
	for _, p := range backlog.Finished(info.PRDs, info.Issues) {
		paths = append(paths, p.Path)
	}
	if len(paths) == 0 {
		return res, errors.New("nothing in the backlog is finished")
	}
	var move []string
	for _, p := range paths {
		tracked, _ := git(ctx, env, "ls-files", "--", p)
		changed, _ := git(ctx, env, "status", "--porcelain", "--", p)
		if strings.TrimSpace(tracked) == "" || strings.TrimSpace(changed) != "" {
			res.Left = append(res.Left, p+": not committed as it is")
			continue
		}
		move = append(move, p)
	}
	if len(move) == 0 {
		return res, errors.New("no finished file is committed as it is now; commit the backlog first")
	}
	if _, err := gitOK(ctx, env, append([]string{"rm", "--quiet", "--"}, move...)...); err != nil {
		return res, err
	}
	msg := fmt.Sprintf("Archive %d finished backlog files\n\nThey stay in git history:\n\n%s\n", len(move), strings.Join(move, "\n"))
	if _, err := gitOK(ctx, env, append([]string{"commit", "--quiet", "-m", msg, "--"}, move...)...); err != nil {
		return res, err
	}
	head, _ := git(ctx, env, "rev-parse", "--short", "HEAD")
	res.Moved, res.Commit = move, strings.TrimSpace(head)
	return res, nil
}

// projectOfWorktree finds the project a session's worktree folder belongs to.
func (e *Engine) projectOfWorktree(ctx context.Context, root string) (session.Project, error) {
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		return session.Project{}, err
	}
	root = environment.Canonical(root)
	for _, s := range list {
		if s.Worktree != "" && environment.Canonical(s.Worktree) == root {
			return e.store.GetProject(ctx, s.ProjectID)
		}
	}
	return session.Project{}, fmt.Errorf("%s is not a project", root)
}

// cleanupStage archives the backlog of a session's folder (the cleanup stage).
func (e *Engine) cleanupStage(ctx context.Context, ses session.Session, w config.Workflow) (StageResult, error) {
	res, err := e.ArchiveBacklog(ctx, ses.WorkRoot(), w.ID)
	if err != nil {
		return StageResult{Kind: config.StageCleanup}, err
	}
	msg := fmt.Sprintf("Archived %d finished backlog files: commit %s removed them, and git history keeps them.", len(res.Moved), res.Commit)
	if len(res.Left) > 0 {
		msg += " Left: " + strings.Join(res.Left, "; ") + "."
	}
	e.emit(ses.ID, event.Notice, event.NoticeData{Message: msg})
	e.gitChanged(ses.ID)
	return StageResult{Kind: config.StageCleanup, Message: msg}, nil
}

// backlogSummary is a backlog's issues for a prompt, or a note why there are none.
func (e *Engine) backlogSummary(ctx context.Context, ses session.Session, w config.Workflow) string {
	project := e.sessionProject(ctx, ses)
	info, err := e.projectBacklog(ctx, project, ses.WorkRoot(), w)
	if err != nil {
		return "(" + err.Error() + ")"
	}
	return cmp.Or(backlog.Summary(info.Issues), "(no issues yet)")
}
