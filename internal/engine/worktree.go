package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Worktrees (SES-010): a session can get its own git worktree and branch, so
// two sessions on one repository never edit the same checkout. Worktrees
// live next to the data folder, not inside it (the protected-path policy
// keeps the agent out of the data folder) and not inside the repository.
// Like the diff panel, worktree commands need a trusted project whose git
// config names no programs (SEC-006), and run with hooks off.

// BranchPrefix starts the name of every session branch.
const BranchPrefix = "freeloop/"

// worktreesRoot is the folder that holds every session's worktree.
func (e *Engine) worktreesRoot() string {
	return filepath.Join(filepath.Dir(e.dataDir), filepath.Base(e.dataDir)+"-worktrees")
}

func missingRoot(ses session.Session) error {
	if ses.Worktree != "" {
		return fmt.Errorf("the session's worktree %s is missing; unarchive the session to recreate it", ses.Worktree)
	}
	return fmt.Errorf("the project folder %s is missing; relink the project", ses.ProjectRoot)
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug turns a title into a branch-safe word.
func slug(s string) string {
	s = strings.Trim(notSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 32 {
		s = strings.Trim(s[:32], "-")
	}
	if s == "" || s == "new-session" {
		s = "session"
	}
	return s
}

// projectGit returns an environment at the project's main checkout for the
// app's worktree commands, or why the project cannot have worktrees.
func (e *Engine) projectGit(ctx context.Context, p session.Project) (environment.Environment, error) {
	if !p.Trusted {
		return nil, errors.New("trust the project before giving sessions their own worktree")
	}
	env, err := e.localEnv(p.Root, nil)
	if err != nil {
		return nil, err
	}
	if why := blocked(ctx, env); why != "" {
		return nil, errors.New(why)
	}
	if _, ok := git(ctx, env, "rev-parse", "--verify", "--quiet", "HEAD"); !ok {
		return nil, errors.New("the project needs a git repository with at least one commit for a worktree")
	}
	return env, nil
}

// addWorktree creates a worktree on a new branch from start (the
// project's HEAD when empty).
func (e *Engine) addWorktree(ctx context.Context, p session.Project, title, start string) (path, branch string, err error) {
	env, err := e.projectGit(ctx, p)
	if err != nil {
		return "", "", err
	}
	var b [3]byte
	rand.Read(b[:])
	name := slug(title) + "-" + hex.EncodeToString(b[:])
	branch = BranchPrefix + name
	path = filepath.Join(e.worktreesRoot(), slug(p.Name), name)
	if start == "" {
		start = "HEAD"
	}
	if _, err := gitOK(ctx, env, "worktree", "add", "--quiet", "-b", branch, path, start); err != nil {
		return "", "", fmt.Errorf("create the worktree: %w", err)
	}
	return environment.Canonical(path), branch, nil
}

// removeWorktree removes a session's worktree when it holds no uncommitted
// work; its branch, and so its commits, stay. It reports whether it did.
func (e *Engine) removeWorktree(ctx context.Context, ses session.Session) (bool, error) {
	project, err := e.store.GetProject(ctx, ses.ProjectID)
	if err != nil {
		return false, err
	}
	env, err := e.projectGit(ctx, project)
	if err != nil {
		return false, err
	}
	if !environment.IsDir(ses.Worktree) {
		gitOK(ctx, env, "worktree", "prune")
		return true, nil
	}
	// A profile switch shares a worktree between sessions (HRN-012); it
	// goes with the last of them.
	if list, err := e.store.ListSessions(ctx); err == nil {
		for _, other := range list {
			if other.ID != ses.ID && other.Worktree == ses.Worktree && other.Status != session.StatusArchived {
				return false, fmt.Errorf("the session %q works in it too", other.Title)
			}
		}
	}
	// Commits made off the session's branch would be left unreachable.
	if wt, err := e.localEnv(ses.Worktree, nil); err == nil {
		head, _ := git(ctx, wt, "symbolic-ref", "--quiet", "--short", "HEAD")
		if head = strings.TrimSpace(head); head != ses.OwnBranch() {
			return false, fmt.Errorf("it is not on the session's branch %s", ses.OwnBranch())
		}
	}
	if _, err := gitOK(ctx, env, "worktree", "remove", ses.Worktree); err != nil {
		return false, err
	}
	return true, nil
}

// restoreWorktree checks the session's branch out again where its worktree
// was, after an archive removed it.
func (e *Engine) restoreWorktree(ctx context.Context, ses session.Session) error {
	if ses.Worktree == "" || environment.IsDir(ses.Worktree) {
		return nil
	}
	project, err := e.store.GetProject(ctx, ses.ProjectID)
	if err != nil {
		return err
	}
	env, err := e.projectGit(ctx, project)
	if err != nil {
		return err
	}
	if _, err := gitOK(ctx, env, "worktree", "add", "--quiet", ses.Worktree, ses.OwnBranch()); err != nil {
		return fmt.Errorf("recreate the worktree: %w", err)
	}
	return nil
}

// SetProjectWorktreeDefault remembers whether new sessions of a project get
// their own worktree: always, never, or "" to ask (OQ-008).
func (e *Engine) SetProjectWorktreeDefault(ctx context.Context, projectID, choice string) error {
	if choice != "" && choice != "always" && choice != "never" {
		return fmt.Errorf("%q must be always, never or empty", choice)
	}
	p, err := e.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	p.WorktreeDefault = choice
	return e.store.UpdateProject(ctx, p)
}

// MergeResult says what MergeWorktree did.
type MergeResult struct {
	Into    string `json:"into"`
	Branch  string `json:"branch"`
	Message string `json:"message"`
}

// MergeRisks lists the files a session's branch would bring into the main
// checkout that something outside a session acts on: the project's hooks and
// settings, editors' tasks, other agents' configuration.
func (e *Engine) MergeRisks(ctx context.Context, sid string) ([]string, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	if ses.Worktree == "" {
		return []string{}, nil
	}
	project, err := e.store.GetProject(ctx, ses.ProjectID)
	if err != nil {
		return nil, err
	}
	main, err := e.projectGit(ctx, project)
	if err != nil {
		return nil, err
	}
	into, ok := git(ctx, main, "symbolic-ref", "--quiet", "--short", "HEAD")
	if !ok {
		return []string{}, nil
	}
	return mergeRisks(ctx, main, strings.TrimSpace(into), ses.OwnBranch()), nil
}

func mergeRisks(ctx context.Context, main environment.Environment, into, branch string) []string {
	out := []string{}
	names, ok := git(ctx, main, "diff", "--name-only", "-z", "--no-renames", "--no-ext-diff", into+"..."+branch)
	if !ok {
		return []string{"(the changed files could not be listed)"}
	}
	for _, name := range strings.Split(names, "\x00") {
		if name != "" && actedOnFile(name) {
			out = append(out, name)
		}
	}
	return out
}

// actedOnFile reports a path in a repository, or any part of it, that something
// outside a session acts on, spelled however the file system would still open it:
// .FREELOOP/hooks.yaml is .freeloop/hooks.yaml on APFS. HEAD is on the list since
// a commit can carry a repository in the checkout as well as a command can make
// one (see bareRepositoryPatterns).
func actedOnFile(name string) bool {
	names := append([]string{ProjectDir, ".gitattributes", ".gitmodules", "HEAD"}, permission.ActedOnLater...)
	for _, part := range strings.Split(name, "/") {
		f := environment.FoldPath(part)
		if slices.ContainsFunc(names, func(n string) bool { return environment.FoldPath(n) == f }) {
			return true
		}
	}
	return false
}

// MergeWorktree merges a session's branch into the branch the project's
// main checkout is on. The session's work must be committed and the main
// checkout must have no uncommitted changes to tracked files; a merge that
// conflicts is aborted and its files listed. Hooks do not run. A merge bringing
// in files something outside a session acts on, a sandboxed agent cannot write
// them but can commit them, needs confirmed: see MergeRisks.
func (e *Engine) MergeWorktree(ctx context.Context, sid string, confirmed bool) (MergeResult, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return MergeResult{}, err
	}
	if ses.Worktree == "" {
		return MergeResult{}, errors.New("the session has no worktree of its own")
	}
	project, err := e.store.GetProject(ctx, ses.ProjectID)
	if err != nil {
		return MergeResult{}, err
	}
	main, err := e.projectGit(ctx, project)
	if err != nil {
		return MergeResult{}, err
	}
	if wt, err := e.localEnv(ses.Worktree, nil); err == nil {
		if out, _ := git(ctx, wt, "status", "--porcelain", "--untracked-files=no", "--ignore-submodules=dirty"); strings.TrimSpace(out) != "" {
			return MergeResult{}, errors.New("the session has uncommitted changes; commit them in the Diff panel first")
		}
	}
	if out, _ := git(ctx, main, "status", "--porcelain", "--untracked-files=no", "--ignore-submodules=dirty"); strings.TrimSpace(out) != "" {
		return MergeResult{}, fmt.Errorf("the main checkout at %s has uncommitted changes; commit or stash them first", project.Root)
	}
	into, ok := git(ctx, main, "symbolic-ref", "--quiet", "--short", "HEAD")
	into = strings.TrimSpace(into)
	if !ok || into == "" {
		return MergeResult{}, errors.New("the main checkout is not on a branch")
	}
	if into == ses.OwnBranch() {
		return MergeResult{}, errors.New("the main checkout is on the session's own branch")
	}
	res := MergeResult{Into: into, Branch: ses.OwnBranch()}
	if !confirmed {
		if risky := mergeRisks(ctx, main, into, ses.OwnBranch()); len(risky) > 0 {
			return res, fmt.Errorf("this merge changes %s, which can run commands on your Mac; check them and confirm", strings.Join(risky, ", "))
		}
	}
	if _, err := gitOK(ctx, main, "merge", "--no-ff", "--no-edit", ses.OwnBranch()); err != nil {
		conflicts, _ := git(ctx, main, "diff", "--name-only", "--diff-filter=U", "--ignore-submodules=dirty")
		gitOK(ctx, main, "merge", "--abort")
		if files := strings.Fields(conflicts); len(files) > 0 {
			return res, fmt.Errorf("merging %s into %s conflicts in %s; the merge was undone, resolve it in a terminal", ses.OwnBranch(), into, strings.Join(files, ", "))
		}
		return res, err
	}
	res.Message = fmt.Sprintf("Merged %s into %s.", ses.OwnBranch(), into)
	e.emit(sid, event.Notice, event.NoticeData{Message: res.Message})
	return res, nil
}
