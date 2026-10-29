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

func git(ctx context.Context, env environment.Environment, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := env.Exec(ctx, environment.ExecSpec{
		Argv:      append([]string{"git", "--no-optional-locks"}, args...),
		Env:       map[string]string{"GIT_TERMINAL_PROMPT": "0"},
		MaxOutput: 1 << 20,
	})
	if err != nil || res.ExitCode != 0 || res.TimedOut {
		return "", false
	}
	return res.Output, true
}

// gitState reads branch, HEAD and the number of changed files.
func gitState(ctx context.Context, env environment.Environment) event.GitStateData {
	out, ok := git(ctx, env, "status", "--porcelain=v2", "--branch")
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
