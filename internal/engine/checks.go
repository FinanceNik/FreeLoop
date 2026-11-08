package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// Pull request checks (SES-018). Once a session has opened a pull request,
// the Diff tab shows its CI checks through the GitHub CLI, which signs in
// on its own; the app never reads its token. Failing GitHub Actions jobs
// can be turned into one message for the agent with the end of their
// logs. Like opening the pull request, this is the user's own action from
// the GUI and runs on this computer.

// Check is one CI check of a pull request.
type Check struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Bucket   string `json:"bucket"` // pass, fail, pending, skipping or cancel
	Link     string `json:"link"`
	Workflow string `json:"workflow"`
}

// ChecksInfo is a pull request's checks.
type ChecksInfo struct {
	URL     string  `json:"url"`
	Checks  []Check `json:"checks"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	Pending int     `json:"pending"`
}

// checksTimeout bounds one gh call.
const checksTimeout = 45 * time.Second

// logTail is how much of a failed job's log a report quotes.
const logTail = 3000

var ghEnv = map[string]string{"GH_PROMPT_DISABLED": "1", "GH_NO_UPDATE_NOTIFIER": "1", "NO_COLOR": "1", "GIT_TERMINAL_PROMPT": "0"}

// runGH runs the GitHub CLI through the login shell, so it is found where
// the user installed it.
func (e *Engine) runGH(ctx context.Context, env environment.Environment, args ...string) (environment.ExecResult, error) {
	res, err := env.Exec(ctx, environment.ExecSpec{
		Argv: append([]string{e.gh}, args...), LoginShell: true, Env: ghEnv,
		SeparateStderr: true, MaxOutput: 512 << 10, Timeout: checksTimeout,
	})
	if err != nil {
		return res, err
	}
	if res.ExitCode == 127 {
		return res, errors.New("the GitHub CLI (gh) is not installed; install it (" + environment.InstallHint("gh") + ") and sign in with `gh auth login`")
	}
	if res.TimedOut {
		return res, errors.New("gh timed out")
	}
	return res, nil
}

// prEnv returns where a session's gh commands run and its last pull
// request's URL.
func (e *Engine) prEnv(ctx context.Context, sid string) (environment.Environment, string, error) {
	if netguard.LocalOnly() {
		return nil, "", errors.New("the app is set to connect to nothing but this computer")
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, "", err
	}
	pr := e.lastPullRequest(ctx, sid)
	if pr == nil {
		return nil, "", errors.New("no pull request was opened from this session")
	}
	if !e.trusted(ses.ProjectID) {
		return nil, "", errors.New(untrustedDiff)
	}
	env, err := e.localEnv(ses.WorkRoot(), nil)
	if err != nil {
		return nil, "", err
	}
	if why := pushBlocked(ctx, env); why != "" {
		return nil, "", errors.New(why)
	}
	return env, pr.URL, nil
}

// PullRequestChecks returns the checks of the session's last pull request.
func (e *Engine) PullRequestChecks(ctx context.Context, sid string) (ChecksInfo, error) {
	env, url, err := e.prEnv(ctx, sid)
	if err != nil {
		return ChecksInfo{}, err
	}
	res, err := e.runGH(ctx, env, "pr", "checks", url, "--json", "name,state,bucket,link,workflow")
	if err != nil {
		return ChecksInfo{}, err
	}
	info := ChecksInfo{URL: url, Checks: []Check{}}
	out := strings.TrimSpace(res.Output)
	if out == "" {
		// gh says so on standard error when a pull request has no checks.
		if strings.Contains(res.Stderr, "no checks") {
			return info, nil
		}
		return ChecksInfo{}, fmt.Errorf("gh pr checks failed: %s", strings.TrimSpace(res.Stderr))
	}
	// gh exits non-zero while checks fail or run, and still prints them.
	if err := json.Unmarshal([]byte(out), &info.Checks); err != nil {
		return ChecksInfo{}, fmt.Errorf("gh pr checks printed something else: %.200s", out)
	}
	for _, c := range info.Checks {
		switch c.Bucket {
		case "pass":
			info.Passed++
		case "fail", "cancel":
			info.Failed++
		case "pending":
			info.Pending++
		}
	}
	return info, nil
}

var (
	prRepo  = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)/pull/\d+`)
	jobLink = regexp.MustCompile(`/actions/runs/\d+/job/(\d+)`)
)

// CheckFailures writes one message for the agent about the session's
// failing checks: each one's name and link and, for a GitHub Actions job,
// the end of its failed steps' log.
func (e *Engine) CheckFailures(ctx context.Context, sid string) (string, error) {
	info, err := e.PullRequestChecks(ctx, sid)
	if err != nil {
		return "", err
	}
	if info.Failed == 0 {
		return "", errors.New("no check has failed")
	}
	env, _, err := e.prEnv(ctx, sid)
	if err != nil {
		return "", err
	}
	repo := ""
	if m := prRepo.FindStringSubmatch(info.URL); m != nil {
		repo = m[1]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "These checks failed on the pull request %s. Find the cause and fix it.\n", info.URL)
	for _, c := range info.Checks {
		if c.Bucket != "fail" && c.Bucket != "cancel" {
			continue
		}
		name := c.Name
		if c.Workflow != "" && c.Workflow != c.Name {
			name = c.Workflow + " / " + c.Name
		}
		fmt.Fprintf(&b, "\n## %s (%s)\n%s\n", name, strings.ToLower(c.State), c.Link)
		m := jobLink.FindStringSubmatch(c.Link)
		if m == nil || repo == "" {
			continue
		}
		res, err := e.runGH(ctx, env, "run", "view", "--job", m[1], "--log-failed", "-R", repo)
		if err != nil || res.ExitCode != 0 || strings.TrimSpace(res.Output) == "" {
			b.WriteString("(The log could not be read.)\n")
			continue
		}
		b.WriteString(fence("", tail(res.Output, logTail)))
	}
	return b.String(), nil
}
