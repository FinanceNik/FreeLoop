package engine

import (
	"context"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// The app's own git queries run through the session's environment like
// every process, but outside the permission broker: they are reads the app
// makes for its own display, not side effects of a harness.

// A repository's own .git/config can make git run commands
// (core.fsmonitor, hooks, content filters). The app's git calls turn off
// what can be turned off; in a project the user has not trusted (SEC-006)
// they are limited to plumbing reads that run nothing (rev-parse,
// symbolic-ref, rev-list, config --get), and in a trusted one the file scan
// is still skipped where filters are configured. Asking a nested repository
// whether its files changed starts git in it, under a config this repository
// knows nothing of; only the commit it stands at is compared.
//
// Even plumbing can run a program if it is allowed to fetch: a missing object in
// a repository with a promisor remote makes rev-list or status fetch it, and an
// ext:: URL is a command. So nothing here may fetch, whatever the config says.
// Only the push, which has to, gets its own list of transports.
var safeGit = []string{"git", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "diff.ignoreSubmodules=dirty"}

// gitArgv is safeGit followed by args, for git in env. On Windows /dev/null is no
// file to Git for Windows' hook lookup but \dev\null on the current drive, a
// folder anyone may make; hooks are looked for in System32 instead, which only an
// administrator can write (NFR-XPLAT-003).
func gitArgv(env environment.Environment, args ...string) []string {
	base := safeGit
	if localWindows(env) {
		base = safeGitOn("windows", os.Getenv("SystemRoot"))
	}
	return append(slices.Clone(base), args...)
}

// safeGitOn is safeGit for a git running on goos.
func safeGitOn(goos, systemRoot string) []string {
	out := slices.Clone(safeGit)
	if goos != "windows" {
		return out
	}
	if systemRoot == "" || !strings.Contains(systemRoot, `:\`) {
		systemRoot = `C:\Windows`
	}
	for i, a := range out {
		if a == "core.hooksPath=/dev/null" {
			out[i] = "core.hooksPath=" + strings.TrimRight(systemRoot, `\`) + `\System32\freeloop-no-hooks`
		}
	}
	return out
}

// gitEnv is the environment of the app's git commands, with more added.
func gitEnv(more map[string]string) map[string]string {
	out := map[string]string{"GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_NOSYSTEM": "1", "GIT_NO_LAZY_FETCH": "1", "GIT_ALLOW_PROTOCOL": ""}
	maps.Copy(out, more)
	return out
}

// gitEnvFor is gitEnv for git in env.
func gitEnvFor(env environment.Environment, more map[string]string) map[string]string {
	if localWindows(env) {
		return gitEnvOn("windows", more)
	}
	return gitEnv(more)
}

// gitEnvOn is gitEnv for a git running on goos. Git for Windows keeps in its system
// config, under Program Files where only an administrator writes, what makes it work
// there: core.autocrlf, without which every file checked out with \r\n shows as
// changed, the credential manager a push signs in with, and its certificates. So on
// Windows the system config is read (NFR-XPLAT-003).
func gitEnvOn(goos string, more map[string]string) map[string]string {
	out := gitEnv(more)
	if goos == "windows" {
		delete(out, "GIT_CONFIG_NOSYSTEM")
	}
	return out
}

func git(ctx context.Context, env environment.Environment, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := env.Exec(ctx, environment.ExecSpec{
		Argv:      gitArgv(env, args...),
		Env:       gitEnvFor(env, nil),
		MaxOutput: 1 << 20,
	})
	if err != nil || res.ExitCode != 0 || res.TimedOut {
		return "", false
	}
	return res.Output, true
}

// gitState reads branch, HEAD and the number of changed files. In an untrusted
// project, or a repository whose config names programs or whose common directory
// was moved, both of which git status may act on, it reads only branch and HEAD
// and reports the count as unknown (-1).
func gitState(ctx context.Context, env environment.Environment, trusted bool) event.GitStateData {
	if !trusted || blocked(ctx, env) != "" {
		if _, ok := git(ctx, env, "rev-parse", "--git-dir"); !ok {
			return event.GitStateData{}
		}
		st := event.GitStateData{Repo: true, Dirty: -1}
		if out, ok := git(ctx, env, "rev-parse", "--verify", "--quiet", "HEAD"); ok {
			st.Head = strings.TrimSpace(out)
		}
		if out, ok := git(ctx, env, "symbolic-ref", "--quiet", "--short", "HEAD"); ok {
			st.Branch = strings.TrimSpace(out)
		}
		return st
	}
	out, ok := git(ctx, env, "status", "--porcelain=v2", "--branch", "--ignore-submodules=all")
	if !ok {
		return event.GitStateData{}
	}
	return parseStatus(out)
}

// repoConfigKeys lists the keys matching pattern that the repository's own
// configuration sets: .git/config and what it includes, and, when
// extensions.worktreeConfig is on, the per-worktree config.worktree, which
// --local does not read and a command in a container could write. ok is
// false when git could not answer, which callers treat as a match.
func repoConfigKeys(ctx context.Context, env environment.Environment, pattern string) (keys []string, ok bool) {
	scopes := []string{"--local"}
	if out, _ := git(ctx, env, "config", "--local", "--type=bool", "--get", "extensions.worktreeConfig"); strings.TrimSpace(out) == "true" {
		scopes = append(scopes, "--worktree")
	}
	for _, scope := range scopes {
		out, code, answered := gitCode(ctx, env, "config", scope, "--includes", "--name-only", "--get-regexp", pattern)
		switch {
		case answered && code == 0:
			keys = append(keys, strings.Fields(out)...)
		case answered && code == 1: // nothing matches
		default:
			if _, isRepo := git(ctx, env, "rev-parse", "--git-dir"); isRepo {
				return keys, false
			}
		}
	}
	return keys, true
}

// gitCode is git that also returns the exit code; answered is false when
// git did not run to an exit.
func gitCode(ctx context.Context, env environment.Environment, args ...string) (string, int, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := env.Exec(ctx, environment.ExecSpec{
		Argv: gitArgv(env, args...),
		Env:  gitEnvFor(env, nil),
	})
	if err != nil || res.TimedOut {
		return "", 0, false
	}
	return res.Output, res.ExitCode, true
}

func parseStatus(out string) event.GitStateData {
	st := event.GitStateData{Repo: true}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			if oid := strings.TrimPrefix(line, "# branch.oid "); oid != "(initial)" {
				st.Head = oid
			}
		case strings.HasPrefix(line, "# branch.head "):
			if h := strings.TrimPrefix(line, "# branch.head "); h != "(detached)" {
				st.Branch = h
			}
		case line == "" || strings.HasPrefix(line, "#"):
		default:
			st.Dirty++
		}
	}
	return st
}

// fingerprint identifies a repository across moves (OQ-020): its oldest
// root commit and the origin URL.
func fingerprint(ctx context.Context, env environment.Environment) (firstCommit, remote string) {
	if out, ok := git(ctx, env, "rev-list", "--max-parents=0", "HEAD"); ok {
		roots := strings.Fields(out)
		slices.Sort(roots)
		if len(roots) > 0 {
			firstCommit = roots[0]
		}
	}
	if out, ok := git(ctx, env, "config", "--get", "remote.origin.url"); ok {
		remote = strings.TrimSpace(out)
	}
	return firstCommit, remote
}

// sameGit reports two git states the same apart from the commits listed with them.
func sameGit(a, b event.GitStateData) bool {
	return a.Repo == b.Repo && a.Branch == b.Branch && a.Head == b.Head && a.Dirty == b.Dirty && a.BaseCommit == b.BaseCommit
}

// maxNewCommits caps the commits one HEAD move lists.
const maxNewCommits = 100

// newCommits lists the commits a checkout's HEAD moved forward over, from old to
// now, merges left out (GUI-051). A move that is not a fast-forward (an amend, a
// rebase, a reset, a switch of branch) lists none. Their lines are counted only in
// a trusted project whose config runs nothing; elsewhere only rev-list runs, which
// is allowed plumbing.
func newCommits(ctx context.Context, env environment.Environment, old, now string, trusted bool) []event.CommitInfo {
	if old == "" || now == "" || old == now {
		return nil
	}
	if _, ok := git(ctx, env, "merge-base", "--is-ancestor", old, now); !ok {
		return nil
	}
	out, ok := git(ctx, env, "rev-list", "--no-merges", "--timestamp", "--max-count="+strconv.Itoa(maxNewCommits), now, "^"+old)
	if !ok {
		return nil
	}
	var commits []event.CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		at, sha, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		when, _ := strconv.ParseInt(at, 10, 64)
		commits = append(commits, event.CommitInfo{SHA: sha, At: when, Added: -1, Deleted: -1, Files: -1})
	}
	if len(commits) == 0 || !trusted || blocked(ctx, env) != "" {
		return commits
	}
	for i := range commits {
		out, ok := git(ctx, env, "diff-tree", "-r", "--root", "--no-commit-id", "--numstat", "--no-ext-diff", "--no-textconv", "--no-renames", commits[i].SHA)
		if !ok {
			continue
		}
		commits[i].Added, commits[i].Deleted, commits[i].Files = numstat(out)
	}
	return commits
}

// numstat adds up git's --numstat lines; a binary file counts as a file only.
func numstat(out string) (added, deleted, files int) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 {
			continue
		}
		files++
		a, errA := strconv.Atoi(f[0])
		d, errD := strconv.Atoi(f[1])
		if errA == nil && errD == nil {
			added += a
			deleted += d
		}
	}
	return added, deleted, files
}

// committed records a commit the app made for a session, the Diff tab's or an AFK
// run's, as that session's (GUI-051): before is HEAD before it.
func (e *Engine) committed(ctx context.Context, sid string, env environment.Environment, before string, trusted bool) {
	st := gitState(ctx, env, trusted)
	if !st.Repo || st.Head == "" || st.Head == before {
		return
	}
	st.Cause = event.CauseCommit
	st.Commits = newCommits(ctx, env, before, st.Head, trusted)
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if ok {
		r.gitMu.Lock()
		defer r.gitMu.Unlock()
		st.BaseCommit = r.base
		r.lastGit = st
		r.lastGit.Commits, r.lastGit.Cause = nil, ""
	} else if ses, err := e.store.GetSession(ctx, sid); err == nil {
		st.BaseCommit = ses.BaseCommit
	}
	e.emit(sid, event.GitState, st)
}
