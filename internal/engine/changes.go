package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// The diff and commit panel (GUI-005) shows a session's changes against its
// base commit and lets the user stage, commit and discard them. These are the
// user's own actions from the GUI, not a harness's side effects, so they run
// through the session's environment but not through the permission broker,
// and with the same guards as the app's git reads: no hooks, no fsmonitor,
// no external diff or textconv drivers, and nothing at all in a repository
// whose local config can make git run commands.

// emptyTree is git's empty tree, the base for a repository that had no
// commit when the session started.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// ChangedFile is one file that differs from the session's base commit or has
// uncommitted changes.
type ChangedFile struct {
	Path string `json:"path"`
	// Status against the base commit: added, modified, deleted, or
	// untracked for a new file git does not know yet.
	Status string `json:"status"`
	// Staged and Unstaged say whether the file has changes in the index and
	// in the working tree; neither means the change is already committed.
	Staged   bool `json:"staged"`
	Unstaged bool `json:"unstaged"`
}

// Changes is what the diff panel lists.
type Changes struct {
	Repo   bool          `json:"repo"`
	Base   string        `json:"base"`
	Head   string        `json:"head"`
	Branch string        `json:"branch"`
	Files  []ChangedFile `json:"files"`
	// Blocked explains why the panel cannot show or change anything in this
	// repository.
	Blocked string `json:"blocked,omitempty"`
}

func gitRun(ctx context.Context, env environment.Environment, timeout time.Duration, args ...string) (environment.ExecResult, error) {
	return gitRunOver(ctx, env, timeout, "", args...)
}

// gitRunOver is gitRun for the few commands that must reach a repository, over
// the transports listed: a colon-separated GIT_ALLOW_PROTOCOL, never ext or fd.
func gitRunOver(ctx context.Context, env environment.Environment, timeout time.Duration, transports string, args ...string) (environment.ExecResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, err := env.Exec(ctx, environment.ExecSpec{
		Argv:      append(slices.Clone(safeGit), args...),
		Env:       gitEnv(map[string]string{"GIT_EDITOR": "true", "GIT_ALLOW_PROTOCOL": transports}),
		MaxOutput: 1 << 20,
		Timeout:   timeout,
	})
	if err == nil && res.TimedOut {
		err = errors.New("git timed out")
	}
	return res, err
}

// gitOK runs git and turns a non-zero exit into an error carrying git's
// message.
func gitOK(ctx context.Context, env environment.Environment, args ...string) (string, error) {
	res, err := gitRun(ctx, env, 30*time.Second, args...)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Output)
		if msg == "" {
			msg = fmt.Sprintf("git %s failed", args[0])
		}
		return "", errors.New(msg)
	}
	return res.Output, nil
}

// sessionGit returns the environment and base commit the panel works on.
func (e *Engine) sessionGit(ctx context.Context, sid string) (environment.Environment, session.Session, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, ses, err
	}
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if ok {
		return r.host, ses, nil
	}
	if !environment.IsDir(ses.WorkRoot()) {
		return nil, ses, missingRoot(ses)
	}
	env, err := e.localEnv(ses.WorkRoot(), nil)
	return env, ses, err
}

// blockedKeys are the keys of a repository's own config that name programs
// git runs: content filters (status, diff, add, checkout), signing programs
// and config-based hooks (commit, which core.hooksPath does not cover), merge
// drivers, and a promisor remote, which a missing object makes git fetch from
// (through ext:: a program; even a plain diff can need one).
const blockedKeys = `^(filter\.|gpg\.|hook\.|merge\..*\.driver$|extensions\.partialclone$|remote\..*\.promisor$|protocol\.)`

// blocked says why the panel must not run git in env, or "". Until projects
// are trusted (SEC-006) the panel stays off in a repository that sets
// blockedKeys, or whose common directory is not where it belongs.
func blocked(ctx context.Context, env environment.Environment) string {
	if movedCommonDir(ctx, env) {
		return "This repository's .git/commondir points somewhere unusual, so its config and hooks may not be its own. The diff panel stays off for it."
	}
	keys, ok := repoConfigKeys(ctx, env, blockedKeys)
	if len(keys) > 0 {
		return fmt.Sprintf("This repository's own git config sets %s, which can run programs. The diff panel stays off for it.", keys[0])
	}
	if !ok {
		return "Freeloop could not read this repository's git config, so the diff panel stays off for it."
	}
	return ""
}

// movedCommonDir reports a repository whose common directory, which holds its
// config and hooks, is neither its git directory nor, for a linked worktree,
// the main one. A command in a container can write .git/commondir, though not
// .git/config, so this is how it would plant a config of its own.
func movedCommonDir(ctx context.Context, env environment.Environment) bool {
	out, ok := git(ctx, env, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if !ok {
		return false // not a repository
	}
	dirs := strings.Split(strings.TrimSpace(out), "\n")
	if len(dirs) != 2 {
		return true
	}
	own, common := environment.Canonical(dirs[0]), environment.Canonical(dirs[1])
	if own == common {
		return false
	}
	parent := filepath.Dir(own)
	return filepath.Base(parent) != "worktrees" || filepath.Dir(parent) != common
}

// plantedGitFile reports a .git file that names a git directory which does not
// name it back. A linked worktree's folder records its .git file; a .git file
// written by anything else would have the sandbox or a container open up another
// repository, whose config the user's own git trusts.
func plantedGitFile(host *environment.Local, own string) bool {
	dotGit := filepath.Join(host.Root(), ".git")
	if fi, err := host.FS().Stat(dotGit); err != nil || fi.IsDir {
		return false
	}
	back, err := host.FS().ReadFile(filepath.Join(own, "gitdir"))
	return err != nil || environment.Canonical(strings.TrimSpace(string(back))) != environment.Canonical(dotGit)
}

// Changes lists the files that differ from the session's base commit plus
// uncommitted ones, by path.
func (e *Engine) Changes(ctx context.Context, sid string) (Changes, error) {
	env, ses, err := e.sessionGit(ctx, sid)
	if err != nil {
		return Changes{}, err
	}
	if !e.trusted(ses.ProjectID) {
		_, repo := git(ctx, env, "rev-parse", "--git-dir")
		return Changes{Repo: repo, Base: ses.BaseCommit, Files: []ChangedFile{}, Blocked: untrustedDiff}, nil
	}
	return changes(ctx, env, ses.BaseCommit)
}

// untrustedDiff is why the diff panel is off in an untrusted project.
const untrustedDiff = "Trust this project to see and commit its changes. Until then the app runs no git command its configuration could extend (SEC-006)."

func changes(ctx context.Context, env environment.Environment, base string) (Changes, error) {
	top, ok := git(ctx, env, "rev-parse", "--show-toplevel")
	if !ok {
		return Changes{Files: []ChangedFile{}}, nil
	}
	c := Changes{Repo: true, Base: base, Files: []ChangedFile{}}
	if c.Blocked = blocked(ctx, env); c.Blocked != "" {
		return c, nil
	}
	if strings.TrimSpace(top) != env.Root() && environment.Canonical(strings.TrimSpace(top)) != environment.Canonical(env.Root()) {
		// Paths below are relative to the repository, the panel's to the
		// project; keep them the same by refusing a project in a subfolder.
		c.Blocked = "The project folder is inside a larger repository; the diff panel works on a repository's top folder only."
		return c, nil
	}
	st, err := gitOK(ctx, env, "status", "--porcelain=v2", "-z", "--branch", "--no-renames", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		return c, err
	}
	files := map[string]*ChangedFile{}
	entries := strings.Split(st, "\x00")
	for _, ent := range entries {
		switch {
		case strings.HasPrefix(ent, "# branch.oid "):
			if oid := strings.TrimPrefix(ent, "# branch.oid "); oid != "(initial)" {
				c.Head = oid
			}
		case strings.HasPrefix(ent, "# branch.head "):
			if h := strings.TrimPrefix(ent, "# branch.head "); h != "(detached)" {
				c.Branch = h
			}
		case strings.HasPrefix(ent, "1 "), strings.HasPrefix(ent, "u "):
			f := strings.SplitN(ent, " ", 9)
			if len(f) < 9 {
				continue
			}
			path := f[8]
			if ent[0] == 'u' { // unmerged lines have one more field
				f = strings.SplitN(ent, " ", 11)
				if len(f) < 11 {
					continue
				}
				path = f[10]
			}
			xy := f[1]
			files[path] = &ChangedFile{Path: path, Status: "modified", Staged: xy[0] != '.', Unstaged: xy[1] != '.'}
		case strings.HasPrefix(ent, "? "):
			path := strings.TrimPrefix(ent, "? ")
			files[path] = &ChangedFile{Path: path, Status: "untracked", Unstaged: true}
		}
	}
	from := base
	if from == "" {
		from = emptyTree
	}
	diff, err := gitOK(ctx, env, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=all", "--name-status", "-z", from)
	if err != nil {
		return c, err
	}
	parts := strings.Split(diff, "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		status, path := parts[i], parts[i+1]
		f, ok := files[path]
		if !ok {
			f = &ChangedFile{Path: path}
			files[path] = f
		}
		switch status {
		case "A":
			f.Status = "added"
		case "D":
			f.Status = "deleted"
		default:
			f.Status = "modified"
		}
	}
	for _, f := range files {
		c.Files = append(c.Files, *f)
	}
	slices.SortFunc(c.Files, func(a, b ChangedFile) int { return strings.Compare(a.Path, b.Path) })
	return c, nil
}

// changedFiles returns the entries of paths in the session's current
// changes, so the panel's actions work only on files it lists.
func (e *Engine) changedFiles(ctx context.Context, sid string, paths ...string) (environment.Environment, session.Session, []ChangedFile, error) {
	env, ses, err := e.sessionGit(ctx, sid)
	if err != nil {
		return nil, ses, nil, err
	}
	if !e.trusted(ses.ProjectID) {
		return nil, ses, nil, errors.New(untrustedDiff)
	}
	c, err := changes(ctx, env, ses.BaseCommit)
	if err != nil {
		return nil, ses, nil, err
	}
	if c.Blocked != "" {
		return nil, ses, nil, errors.New(c.Blocked)
	}
	out := make([]ChangedFile, 0, len(paths))
	for _, p := range paths {
		i := slices.IndexFunc(c.Files, func(f ChangedFile) bool { return f.Path == p })
		if !filepath.IsLocal(p) || i < 0 {
			return nil, ses, nil, fmt.Errorf("%s has no changes", p)
		}
		out = append(out, c.Files[i])
	}
	return env, ses, out, nil
}

// FileDiff is one file's unified diff against the session's base commit.
type FileDiff struct {
	Path      string `json:"path"`
	Diff      string `json:"diff"`
	Truncated bool   `json:"truncated"`
}

// FileDiff returns path's diff against the base commit; an untracked file
// shows as added.
func (e *Engine) FileDiff(ctx context.Context, sid, path string) (FileDiff, error) {
	env, ses, files, err := e.changedFiles(ctx, sid, path)
	if err != nil {
		return FileDiff{}, err
	}
	f := files[0]
	from := ses.BaseCommit
	if from == "" {
		from = emptyTree
	}
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--ignore-submodules=dirty", from, "--", path}
	if f.Status == "untracked" {
		args = []string{"diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--", "/dev/null", path}
	}
	res, err := gitRun(ctx, env, 30*time.Second, args...)
	if err != nil {
		return FileDiff{}, err
	}
	// --no-index exits 1 when the files differ, which they always do here.
	if res.ExitCode != 0 && !(f.Status == "untracked" && res.ExitCode == 1) {
		return FileDiff{}, errors.New(strings.TrimSpace(res.Output))
	}
	return FileDiff{Path: path, Diff: res.Output, Truncated: res.Truncated}, nil
}

// StageFiles adds paths to the index, or with staged false takes them out.
func (e *Engine) StageFiles(ctx context.Context, sid string, paths []string, staged bool) error {
	if len(paths) == 0 {
		return nil
	}
	env, _, _, err := e.changedFiles(ctx, sid, paths...)
	if err != nil {
		return err
	}
	args := append([]string{"add", "--all", "--"}, paths...)
	if !staged {
		args = append([]string{"reset", "--quiet", "--"}, paths...)
		// Before the first commit there is no HEAD to reset to.
		if _, ok := git(ctx, env, "rev-parse", "--verify", "--quiet", "HEAD"); !ok {
			args = append([]string{"rm", "--cached", "--quiet", "--"}, paths...)
		}
	}
	_, err = gitOK(ctx, env, args...)
	e.gitChanged(sid)
	return err
}

// Commit commits what is staged with message and returns the new commit.
// Hooks do not run (see safeGit).
func (e *Engine) Commit(ctx context.Context, sid, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", errors.New("write a commit message")
	}
	env, ses, err := e.sessionGit(ctx, sid)
	if err != nil {
		return "", err
	}
	if !e.trusted(ses.ProjectID) {
		return "", errors.New(untrustedDiff)
	}
	if why := blocked(ctx, env); why != "" {
		return "", errors.New(why)
	}
	if _, err := gitOK(ctx, env, "commit", "--quiet", "--cleanup=strip", "-m", message); err != nil {
		return "", err
	}
	head, _ := git(ctx, env, "rev-parse", "HEAD")
	e.gitChanged(ses.ID)
	return strings.TrimSpace(head), nil
}

// DiscardFile throws away path's uncommitted changes: a tracked file returns
// to HEAD, a new file is deleted. Changes already committed stay.
func (e *Engine) DiscardFile(ctx context.Context, sid, path string) error {
	env, _, files, err := e.changedFiles(ctx, sid, path)
	if err != nil {
		return err
	}
	f := files[0]
	if !f.Staged && !f.Unstaged {
		return fmt.Errorf("%s has no uncommitted changes", path)
	}
	_, inHead := git(ctx, env, "cat-file", "-e", "HEAD:"+path)
	switch {
	case f.Status == "untracked":
		_, err = gitOK(ctx, env, "clean", "--force", "--quiet", "--", path)
	case !inHead:
		_, err = gitOK(ctx, env, "rm", "--force", "--quiet", "--", path)
	default:
		// Restoring writes what HEAD holds, outside any sandbox. A confined agent
		// can commit a file it may not write, and this would write it for it.
		if actedOnFile(path) && e.sessionConfined(ctx, sid) {
			return fmt.Errorf("%s is a file the app or your tools act on, and a confined session may have committed what is in it, so it is not restored from here. Check it, then restore it in a terminal", path)
		}
		_, err = gitOK(ctx, env, "checkout", "HEAD", "--", path)
	}
	e.gitChanged(sid)
	return err
}

// sessionConfined reports a session whose commands run in a sandbox or container,
// or one the app cannot tell about.
func (e *Engine) sessionConfined(ctx context.Context, sid string) bool {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return true
	}
	cfg, err := e.environment(ses.EnvironmentID)
	return err != nil || cfg.SandboxOn() || cfg.Kind == string(environment.KindDocker)
}

// gitChanged records the session's new git state after a panel action.
func (e *Engine) gitChanged(sid string) {
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if ok {
		e.refreshGitLater(r)
		return
	}
	e.bg.Go(func() {
		ctx := context.Background()
		env, ses, err := e.sessionGit(ctx, sid)
		if err != nil {
			return
		}
		st := gitState(ctx, env, e.trusted(ses.ProjectID))
		st.BaseCommit = ses.BaseCommit
		e.emit(sid, event.GitState, st)
	})
}

// maxSuggestDiff caps the diff sent to the model for a commit message.
const maxSuggestDiff = 40_000

// SuggestCommitMessage asks the session's model for a commit message for
// what is staged, or for every uncommitted change when nothing is.
func (e *Engine) SuggestCommitMessage(ctx context.Context, sid string) (string, error) {
	env, ses, err := e.sessionGit(ctx, sid)
	if err != nil {
		return "", err
	}
	if !e.trusted(ses.ProjectID) {
		return "", errors.New(untrustedDiff)
	}
	if why := blocked(ctx, env); why != "" {
		return "", errors.New(why)
	}
	diff, err := gitOK(ctx, env, "diff", "--cached", "--no-ext-diff", "--no-textconv", "--no-color")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(diff) == "" {
		if diff, err = gitOK(ctx, env, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--ignore-submodules=dirty", "HEAD"); err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(diff) == "" {
		return "", errors.New("there is nothing to commit")
	}
	if len(diff) > maxSuggestDiff {
		diff = diff[:maxSuggestDiff] + "\n[diff truncated]"
	}
	msg, err := e.complete(ctx, ses,
		"You write git commit messages. Reply with the message only: a summary line of at most 72 characters "+
			"in the imperative mood, then, if the change needs it, a blank line and a short body. No code fences, no quotes.",
		"Write a commit message for this diff:\n\n"+diff, 400)
	if err != nil {
		return "", err
	}
	return msg, nil
}

// complete asks the session's model for one reply to prompt, for the
// panel's suggestions; the user asked for it, so no tools and no transcript.
func (e *Engine) complete(ctx context.Context, ses session.Session, system, prompt string, maxTokens int) (string, error) {
	p, err := e.provider(ses.ProviderID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// Little reasoning for a short text, and room for the reasoning there
	// is: models that reason count it against the output limit.
	ch, err := p.p.Stream(ctx, provider.ChatRequest{
		Model:     ses.Model,
		System:    system,
		Messages:  []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: prompt}}}},
		MaxTokens: max(maxTokens, 4000),
		Effort:    "low",
	})
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for ev := range ch {
		switch ev.Type {
		case provider.EventError:
			return "", ev.Err
		case provider.EventDone:
			if ev.Message != nil {
				for _, part := range ev.Message.Parts {
					if part.Type == provider.PartText {
						text.WriteString(part.Text)
					}
				}
			}
		}
	}
	msg := strings.TrimSpace(text.String())
	if msg == "" {
		return "", errors.New("the model returned no message")
	}
	return msg, nil
}
