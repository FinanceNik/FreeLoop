package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// fakeGH writes a gh stand-in that records its arguments and stdin in dir
// and prints a pull request URL, or fails with exit code code.
func fakeGH(t *testing.T, dir string, code int) string {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + dir + "/args'\ncat > '" + dir + "/body'\n"
	if code == 0 {
		script += "echo 'Creating pull request for feature into main in acme/app'\necho https://github.com/acme/app/pull/7\n"
	} else {
		script += "echo 'To get started with GitHub CLI, please run:  gh auth login' >&2\nexit " + string(rune('0'+code)) + "\n"
	}
	p := filepath.Join(dir, "gh")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// SES-011: a session on a feature branch pushes it to its remote (a local
// bare repository here) and opens a pull request through gh (a fake here)
// with the drafted, edited title and description; the URL lands in the
// transcript. On the base branch, without commits, without gh or when gh
// fails, the panel says why.
func TestPullRequests(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	gitIn(t, tmp, "init", "-q", "--bare", "-b", "main", remote)
	seed := gitRepo(t)
	gitIn(t, seed, "push", "-q", remote, "main")
	repo := filepath.Join(tmp, "app")
	gitIn(t, tmp, "clone", "-q", remote, repo)

	e := openTest(t, t.TempDir())
	defer e.Close()
	withScript(e, "msg", scripted.Step{Text: "# Add the feature\n\nAdds feature.txt.\n\nTested by hand."})
	if _, err := e.SetProjectTrust(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "msg", Model: "scripted", Title: "Feature work"})
	if err != nil {
		t.Fatal(err)
	}

	info, err := e.PullRequestInfo(ctx, ses.ID)
	if err != nil || info.Branch != "main" || info.Base != "main" || info.Remote != "origin" || !strings.Contains(info.Blocked, "on main") {
		t.Fatalf("on the base branch: %+v, %v", info, err)
	}
	gitIn(t, repo, "checkout", "-q", "-b", "feature")
	if info, _ = e.PullRequestInfo(ctx, ses.ID); !strings.Contains(info.Blocked, "no commits") {
		t.Fatalf("without commits: %+v", info)
	}
	os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("new\n"), 0o644)
	gitIn(t, repo, "add", "feature.txt")
	gitIn(t, repo, "commit", "-q", "-m", "Add the feature")
	info, err = e.PullRequestInfo(ctx, ses.ID)
	if err != nil || info.Blocked != "" || info.Branch != "feature" || info.Title != "Add the feature" || len(info.Commits) != 1 || info.Opened != nil {
		t.Fatalf("ready: %+v, %v", info, err)
	}

	text, err := e.SuggestPullRequest(ctx, ses.ID)
	if err != nil || text.Title != "Add the feature" || !strings.HasPrefix(text.Body, "Adds feature.txt.") {
		t.Fatalf("suggested = %+v, %v", text, err)
	}

	// gh missing or failing: the branch is pushed, the error says why.
	e.gh = filepath.Join(tmp, "no-such-gh")
	if _, err := e.CreatePullRequest(ctx, CreatePullRequestRequest{SessionID: ses.ID, Title: "x"}); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("without gh: %v", err)
	}
	if got := gitIn(t, remote, "rev-parse", "feature"); got != gitIn(t, repo, "rev-parse", "HEAD") {
		t.Errorf("the branch was not pushed: %s", got)
	}
	ghDir := t.TempDir()
	e.gh = fakeGH(t, ghDir, 1)
	if _, err := e.CreatePullRequest(ctx, CreatePullRequestRequest{SessionID: ses.ID, Title: "x"}); err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("failing gh: %v", err)
	}
	if _, err := e.CreatePullRequest(ctx, CreatePullRequestRequest{SessionID: ses.ID, Title: " "}); err == nil {
		t.Error("an empty title was accepted")
	}
	if _, err := e.CreatePullRequest(ctx, CreatePullRequestRequest{SessionID: ses.ID, Title: "x", Base: "--force"}); err == nil {
		t.Error("an option was accepted as the base branch")
	}

	e.gh = fakeGH(t, ghDir, 0)
	body := "Adds feature.txt.\n\n`$(touch pwned)` stays text."
	pr, err := e.CreatePullRequest(ctx, CreatePullRequestRequest{SessionID: ses.ID, Title: "Add the feature", Body: body, Draft: true})
	if err != nil || pr.URL != "https://github.com/acme/app/pull/7" || pr.Base != "main" || pr.Head != "feature" || !pr.Draft {
		t.Fatalf("created = %+v, %v", pr, err)
	}
	args, _ := os.ReadFile(filepath.Join(ghDir, "args"))
	if got := strings.Fields(string(args)); strings.Join(got, " ") != "pr create --title Add the feature --body-file - --base main --head feature --draft" {
		t.Errorf("gh args = %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(ghDir, "body")); string(b) != body {
		t.Errorf("gh body = %q", b)
	}
	if _, err := os.Stat(filepath.Join(repo, "pwned")); err == nil {
		t.Error("the body ran as a command")
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	var recorded event.PullRequestData
	for _, ev := range evs {
		if ev.Type == event.PullRequest {
			json.Unmarshal(ev.Data, &recorded)
		}
	}
	if recorded != pr {
		t.Errorf("transcript = %+v", recorded)
	}
	if info, _ = e.PullRequestInfo(ctx, ses.ID); info.Opened == nil || info.Opened.URL != pr.URL {
		t.Errorf("opened = %+v", info.Opened)
	}
	if md, _ := e.RenderExport(ctx, ses.ID, ExportMarkdown); !strings.Contains(md, pr.URL) {
		t.Error("the export leaves out the pull request")
	}
}
