package engine

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// The app's own git queries run through the session's environment like
// every process, but outside the permission broker: they are reads the app
// makes for its own display, not side effects of a harness.

// A repository's own .git/config can make git run commands
// (core.fsmonitor, hooks, content filters). Until projects are trusted
// explicitly (SEC-006), the app's git calls turn off what can be turned off
// and skip the file scan where filters are configured.
var safeGit = []string{"git", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null"}

func git(ctx context.Context, env environment.Environment, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := env.Exec(ctx, environment.ExecSpec{
		Argv:      append(slices.Clone(safeGit), args...),
		Env:       map[string]string{"GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_NOSYSTEM": "1"},
		MaxOutput: 1 << 20,
	})
	if err != nil || res.ExitCode != 0 || res.TimedOut {
		return "", false
	}
	return res.Output, true
}

// gitState reads branch, HEAD and the number of changed files. In a
// repository that configures content filters, which git status may run, it
// reads only branch and HEAD and reports the count as unknown (-1).
func gitState(ctx context.Context, env environment.Environment) event.GitStateData {
	if filters, ok := git(ctx, env, "config", "--local", "--name-only", "--get-regexp", `^filter\.`); ok && strings.TrimSpace(filters) != "" {
		st := event.GitStateData{Repo: true, Dirty: -1}
		if out, ok := git(ctx, env, "rev-parse", "HEAD"); ok {
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
