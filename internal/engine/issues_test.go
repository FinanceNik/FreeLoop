package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SES-017: a trusted project's open issues come from gh (a fake here),
// and one becomes a first message with its body and comments.
func TestIssues(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	repo := gitRepo(t)
	dir := t.TempDir()
	gh := filepath.Join(dir, "gh")
	os.WriteFile(gh, []byte(`#!/bin/sh
echo "$@" >> '`+dir+`/calls'
case "$1 $2" in
"issue list")
  echo '[{"number":12,"title":"Login fails","url":"https://github.com/acme/app/issues/12","labels":[{"name":"bug"}],"author":{"login":"ann"},"updatedAt":"2026-10-30T10:00:00Z"}]' ;;
"issue view")
  echo '{"number":12,"title":"Login fails","url":"https://github.com/acme/app/issues/12","body":"Steps: open /login, submit.","labels":[{"name":"bug"}],"author":{"login":"ann"},"comments":[{"author":{"login":"bob"},"body":"Same on Safari."}]}' ;;
esac
`), 0o755)
	e.gh = gh
	if _, err := e.Issues(ctx, repo, ""); err == nil || !strings.Contains(err.Error(), "trust") {
		t.Errorf("untrusted project: %v", err)
	}
	e.SetProjectTrust(ctx, repo, true)
	list, err := e.Issues(ctx, repo, "label:bug")
	if err != nil || len(list) != 1 || list[0].Number != 12 || list[0].Labels[0] != "bug" || list[0].Author != "ann" {
		t.Fatalf("issues = %+v, %v", list, err)
	}
	brief, err := e.IssueBrief(ctx, repo, 12)
	if err != nil || brief.SessionTitle != "#12 Login fails" || brief.Tag != "issue-12" {
		t.Fatalf("brief = %+v, %v", brief, err)
	}
	for _, want := range []string{"Work on GitHub issue #12: Login fails", "https://github.com/acme/app/issues/12", "Steps: open /login", "bob wrote:\nSame on Safari."} {
		if !strings.Contains(brief.Text, want) {
			t.Errorf("brief lacks %q:\n%s", want, brief.Text)
		}
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if !strings.Contains(string(calls), "--search label:bug") {
		t.Errorf("calls = %s", calls)
	}
}
