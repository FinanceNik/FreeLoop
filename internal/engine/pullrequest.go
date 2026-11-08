package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Pull requests (SES-011): the diff panel pushes the session's branch and
// opens a pull request with the GitHub CLI, which signs in on its own; the
// app never reads its token. Like committing, this is the user's own action
// from the GUI, so it runs through the session's host environment with the
// panel's git guards but not through the permission broker. The URL is
// recorded in the transcript as a pull_request event.

// PullRequestInfo is what the pull request form shows.
type PullRequestInfo struct {
	// Branch is the branch the pull request proposes; Base the one it
	// targets, by default the remote's default branch.
	Branch string `json:"branch"`
	Base   string `json:"base"`
	Remote string `json:"remote"`
	// Commits are the subjects of the commits on Branch that Base lacks,
	// newest first.
	Commits []string `json:"commits"`
	// Title suggests one: the only commit's subject, or the session's title.
	Title string `json:"title"`
	// Opened is the last pull request opened from this session.
	Opened *event.PullRequestData `json:"opened,omitempty"`
	// Blocked says why no pull request can be opened from here yet.
	Blocked string `json:"blocked,omitempty"`
}

// CreatePullRequestRequest is the form's content.
type CreatePullRequestRequest struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	// Base overrides the target branch.
	Base  string `json:"base"`
	Draft bool   `json:"draft"`
}

// PullRequestText is a drafted title and description.
type PullRequestText struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// prTimeout bounds the push and the gh call; both talk to the network.
const prTimeout = 2 * time.Minute

var branchName = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// PullRequestInfo reads the session's branch, its target and the commits a
// pull request would propose.
func (e *Engine) PullRequestInfo(ctx context.Context, sid string) (PullRequestInfo, error) {
	info, _, err := e.pullRequestInfo(ctx, sid, "")
	return info, err
}

func (e *Engine) pullRequestInfo(ctx context.Context, sid, base string) (PullRequestInfo, environment.Environment, error) {
	info := PullRequestInfo{Commits: []string{}}
	env, ses, err := e.sessionGit(ctx, sid)
	if err != nil {
		return info, nil, err
	}
	info.Title = ses.Title
	info.Opened = e.lastPullRequest(ctx, sid)
	if !e.trusted(ses.ProjectID) {
		info.Blocked = untrustedDiff
		return info, env, nil
	}
	if _, ok := git(ctx, env, "rev-parse", "--git-dir"); !ok {
		info.Blocked = "The project is not a git repository."
		return info, env, nil
	}
	if why := blocked(ctx, env); why != "" {
		info.Blocked = why
		return info, env, nil
	}
	branch, ok := git(ctx, env, "symbolic-ref", "--quiet", "--short", "HEAD")
	info.Branch = strings.TrimSpace(branch)
	if !ok || info.Branch == "" {
		info.Blocked = "HEAD is not on a branch. Check out a branch to open a pull request from it."
		return info, env, nil
	}
	remotes, _ := git(ctx, env, "remote")
	for _, r := range strings.Fields(remotes) {
		if info.Remote == "" || r == "origin" {
			info.Remote = r
		}
	}
	if info.Remote == "" {
		info.Blocked = "The repository has no remote to push the branch to."
		return info, env, nil
	}
	info.Base = base
	if info.Base == "" {
		info.Base = defaultBranch(ctx, env, info.Remote)
	}
	if info.Base == info.Branch {
		info.Blocked = fmt.Sprintf("The session is on %s, the branch a pull request would target. Work on another branch (a worktree session has its own) to open one.", info.Base)
		return info, env, nil
	}
	// The commits the remote's base lacks; without that ref, those since
	// the session started.
	from := info.Remote + "/" + info.Base
	if _, ok := git(ctx, env, "rev-parse", "--verify", "--quiet", "refs/remotes/"+from); !ok {
		from = ses.BaseCommit
	}
	if from != "" {
		if out, ok := git(ctx, env, "log", "--format=%s", "-n", "50", from+"..HEAD"); ok {
			for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
				if l != "" {
					info.Commits = append(info.Commits, l)
				}
			}
		}
	}
	if len(info.Commits) == 0 {
		info.Blocked = fmt.Sprintf("%s has no commits that %s lacks. Commit the changes first.", info.Branch, info.Base)
		return info, env, nil
	}
	if len(info.Commits) == 1 {
		info.Title = info.Commits[0]
	}
	return info, env, nil
}

// defaultBranch is the remote's default branch as git last saw it, else
// main or master when the remote has one, else main.
func defaultBranch(ctx context.Context, env environment.Environment, remote string) string {
	if ref, ok := git(ctx, env, "symbolic-ref", "--quiet", "--short", "refs/remotes/"+remote+"/HEAD"); ok {
		if b, ok := strings.CutPrefix(strings.TrimSpace(ref), remote+"/"); ok && b != "" {
			return b
		}
	}
	for _, b := range []string{"main", "master"} {
		if _, ok := git(ctx, env, "rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+b); ok {
			return b
		}
	}
	return "main"
}

// lastPullRequest returns the last pull request the session's transcript
// records.
func (e *Engine) lastPullRequest(ctx context.Context, sid string) *event.PullRequestData {
	evs, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return nil
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == event.PullRequest {
			var d event.PullRequestData
			if json.Unmarshal(evs[i].Data, &d) == nil {
				return &d
			}
		}
	}
	return nil
}

// maxPRDiff caps the diff sent to the model for a description.
const maxPRDiff = 40_000

// SuggestPullRequest asks the session's model for a title and description
// from the branch's commits and diff, for the user to edit.
func (e *Engine) SuggestPullRequest(ctx context.Context, sid string) (PullRequestText, error) {
	info, env, err := e.pullRequestInfo(ctx, sid, "")
	if err != nil {
		return PullRequestText{}, err
	}
	if info.Blocked != "" {
		return PullRequestText{}, errors.New(info.Blocked)
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return PullRequestText{}, err
	}
	from := info.Remote + "/" + info.Base
	if _, ok := git(ctx, env, "rev-parse", "--verify", "--quiet", "refs/remotes/"+from); !ok {
		from = ses.BaseCommit
	}
	log, _ := gitOK(ctx, env, "log", "--format=%B%n---", "-n", "50", from+"..HEAD")
	diff, _ := gitOK(ctx, env, "diff", "--no-ext-diff", "--no-textconv", "--no-color", from+"...HEAD")
	if len(diff) > maxPRDiff {
		diff = diff[:maxPRDiff] + "\n[diff truncated]"
	}
	text, err := e.complete(provider.WithPurpose(ctx, provider.PurposePullRequest, "The session's commit messages and diff ("+sizeText(len(log)+len(diff))+")"), ses,
		"You write GitHub pull request descriptions. Reply with a title of at most 72 characters on the first line, "+
			"a blank line, then the description in Markdown: what changes and why, and how it was tested if the commits say. "+
			"Be brief and factual. No code fences around the reply.",
		"Commits:\n\n"+log+"\n\nDiff:\n\n"+diff, 1200)
	if err != nil {
		return PullRequestText{}, err
	}
	title, body, _ := strings.Cut(text, "\n")
	title = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(title), "#"))
	return PullRequestText{Title: title, Body: strings.TrimSpace(body)}, nil
}

// CreatePullRequest pushes the session's branch to its remote and opens a
// pull request for it with gh; it returns what it recorded.
func (e *Engine) CreatePullRequest(ctx context.Context, req CreatePullRequestRequest) (event.PullRequestData, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return event.PullRequestData{}, errors.New("give the pull request a title")
	}
	base := strings.TrimSpace(req.Base)
	if base != "" && (!branchName.MatchString(base) || strings.HasPrefix(base, "-")) {
		return event.PullRequestData{}, fmt.Errorf("%q is not a branch name", base)
	}
	if netguard.LocalOnly() {
		return event.PullRequestData{}, errors.New("the app is set to connect to nothing but this Mac; turn local-only off in Settings to push and open a pull request")
	}
	info, env, err := e.pullRequestInfo(ctx, req.SessionID, base)
	if err != nil {
		return event.PullRequestData{}, err
	}
	if info.Blocked != "" {
		return event.PullRequestData{}, errors.New(info.Blocked)
	}
	if why := pushBlocked(ctx, env); why != "" {
		return event.PullRequestData{}, errors.New(why)
	}
	if !branchName.MatchString(info.Branch) || strings.HasPrefix(info.Branch, "-") {
		return event.PullRequestData{}, fmt.Errorf("the branch name %q cannot be passed to gh", info.Branch)
	}
	if strings.HasPrefix(info.Remote, "-") {
		return event.PullRequestData{}, fmt.Errorf("the remote name %q cannot be passed to git", info.Remote)
	}
	// Hooks do not run (safeGit), so a pre-push hook cannot either; the ext
	// and fd transports would run a program the remote's URL names.
	res, err := gitRunOver(ctx, env, prTimeout, "https:http:ssh:git:file", "-c", "protocol.ext.allow=never", "-c", "protocol.fd.allow=never", "push", "--porcelain", "--set-upstream", info.Remote, "HEAD:refs/heads/"+info.Branch)
	if err != nil {
		return event.PullRequestData{}, err
	}
	if res.ExitCode != 0 {
		return event.PullRequestData{}, fmt.Errorf("git push failed: %s", strings.TrimSpace(res.Output))
	}
	e.gitChanged(req.SessionID)

	argv := []string{e.gh, "pr", "create", "--title", title, "--body-file", "-", "--base", info.Base, "--head", info.Branch}
	if req.Draft {
		argv = append(argv, "--draft")
	}
	res, err = env.Exec(ctx, environment.ExecSpec{
		Argv:           argv,
		LoginShell:     true,
		Stdin:          []byte(req.Body),
		Env:            pushEnv(),
		SeparateStderr: true,
		MaxOutput:      64 << 10,
		Timeout:        prTimeout,
	})
	if err != nil {
		return event.PullRequestData{}, err
	}
	if res.TimedOut {
		return event.PullRequestData{}, errors.New("gh timed out; the branch is pushed")
	}
	if res.ExitCode == 127 {
		return event.PullRequestData{}, errors.New("the GitHub CLI (gh) is not installed. Install it (" + environment.InstallHint("gh") + "), sign in with `gh auth login`, and try again; the branch is pushed")
	}
	url := prURL(res.Output)
	if res.ExitCode != 0 || url == "" {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Output)
		}
		return event.PullRequestData{}, fmt.Errorf("gh pr create failed (the branch is pushed): %s", msg)
	}
	pr := event.PullRequestData{URL: url, Title: title, Base: info.Base, Head: info.Branch, Draft: req.Draft}
	e.emit(req.SessionID, event.PullRequest, pr)
	return pr, nil
}

var urlLine = regexp.MustCompile(`https://\S+`)

// prURL is the pull request's address in gh's output, its last URL.
func prURL(out string) string {
	m := urlLine.FindAllString(out, -1)
	if len(m) == 0 {
		return ""
	}
	return m[len(m)-1]
}

// pushBlocked says why the push and gh must not run in env, or "": beyond
// what the diff panel refuses, a repository's own config can name programs
// that a push (ssh, credential helpers, a local remote's receive-pack, a
// transport a rewritten URL or protocol setting allows, a proxy or a turned-off
// certificate check that would hand it the user's token) or gh's own git calls
// (fsmonitor) run. Those come from the user's global config only.
func pushBlocked(ctx context.Context, env environment.Environment) string {
	keys, ok := repoConfigKeys(ctx, env, `^(core\.(sshcommand|gitproxy|askpass|fsmonitor)|credential\..*helper|remote\..*\.(receivepack|uploadpack|pushurl|proxy)|url\..*|protocol\..*|http\..*)$`)
	if len(keys) > 0 {
		return fmt.Sprintf("This repository's own git config sets %s, which can run a program or send your token elsewhere when pushing. Push from a terminal instead.", keys[0])
	}
	if !ok {
		return "Freeloop could not read this repository's git config, so it does not push from here."
	}
	return ""
}

// pushEnv is gh's environment for a call that pushes: ghEnv, and no system
// git config.
func pushEnv() map[string]string {
	env := maps.Clone(ghEnv)
	env["GIT_CONFIG_NOSYSTEM"] = "1"
	return env
}
