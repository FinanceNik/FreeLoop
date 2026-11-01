package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// SES-018: the checks of a session's pull request come from gh (a fake
// here); failures become one message with each failing job's log.
func TestPullRequestChecks(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "checks", "ask")
	e.SetProjectTrust(ctx, ses.ProjectRoot, true)
	if _, err := e.PullRequestChecks(ctx, ses.ID); err == nil || !strings.Contains(err.Error(), "no pull request") {
		t.Errorf("without a pull request: %v", err)
	}
	e.emit(ses.ID, event.PullRequest, event.PullRequestData{URL: "https://github.com/acme/app/pull/7", Title: "t", Base: "main", Head: "f"})

	dir := t.TempDir()
	gh := filepath.Join(dir, "gh")
	os.WriteFile(gh, []byte(`#!/bin/sh
echo "$@" >> '`+dir+`/calls'
case "$1 $2" in
"pr checks")
  echo '[{"name":"test","state":"FAILURE","bucket":"fail","link":"https://github.com/acme/app/actions/runs/11/job/22","workflow":"CI"},{"name":"lint","state":"SUCCESS","bucket":"pass","link":"https://x","workflow":"CI"},{"name":"build","state":"IN_PROGRESS","bucket":"pending","link":"https://y","workflow":"CI"}]'
  exit 1 ;;
"run view")
  echo "test	Run tests	--- FAIL: TestLogin"
  echo "test	Run tests	login_test.go:12: want 200, got 500" ;;
esac
`), 0o755)
	e.gh = gh

	info, err := e.PullRequestChecks(ctx, ses.ID)
	if err != nil || len(info.Checks) != 3 || info.Failed != 1 || info.Passed != 1 || info.Pending != 1 {
		t.Fatalf("checks = %+v, %v", info, err)
	}
	msg, err := e.CheckFailures(ctx, ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://github.com/acme/app/pull/7", "## CI / test (failure)", "actions/runs/11/job/22", "want 200, got 500"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "lint") {
		t.Error("a passing check is in the message")
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if !strings.Contains(string(calls), "run view --job 22 --log-failed -R acme/app") {
		t.Errorf("gh calls:\n%s", calls)
	}

	netguard.SetLocalOnly(true)
	defer netguard.SetLocalOnly(false)
	if _, err := e.PullRequestChecks(ctx, ses.ID); err == nil {
		t.Error("checked while local-only")
	}
}
