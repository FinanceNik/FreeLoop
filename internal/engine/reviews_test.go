package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// HRN-018: a reviewer gets the session's diff, in plan mode, in the same
// folder, under the session; its reply becomes a message only on request.
func TestReviewBySecondAgent(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	p := scriptedProvider(e, "rev", scripted.Step{Text: "1. change.txt:1: the word is misspelt."})
	e.SetProjectTrust(ctx, repo, true)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "rev", Model: "m", Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.RequestReview(ctx, ReviewRequest{SessionID: ses.ID, ProfileID: "default"}); err == nil || !strings.Contains(err.Error(), "no changes") {
		t.Errorf("nothing to review: %v", err)
	}
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed line\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "change.txt"), []byte("teh word\n"), 0o644)

	r, err := e.RequestReview(ctx, ReviewRequest{SessionID: ses.ID, ProfileID: "default", Label: "Second opinion", Focus: "the wording"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Mode != "plan" || r.ParentID != ses.ID || r.Title != "Review: Second opinion" || !slices.Contains(tagsOf(t, e, r.ID), ReviewTag) {
		t.Fatalf("reviewer = %+v", r)
	}
	waitIdle(t, ch, r.ID)
	req := p.Requests()[0]
	sent := req.Messages[len(req.Messages)-1].Parts[0].Text
	for _, want := range []string{"You are read-only", "the wording", "change.txt", "+changed line"} {
		if !strings.Contains(sent, want) {
			t.Errorf("the reviewer was not told %q:\n%s", want, sent)
		}
	}
	for _, spec := range req.Tools {
		if spec.Name == "write_file" || spec.Name == "edit_file" || spec.Name == "shell" {
			t.Errorf("the reviewer was offered %s", spec.Name)
		}
	}

	// The first session got nothing by itself.
	evs, _ := e.Events(ctx, ses.ID, 0)
	if slices.ContainsFunc(evs, func(ev event.Event) bool { return ev.Type == event.UserMessageChunk }) {
		t.Error("the findings reached the session without being asked for")
	}
	list, _ := e.Reviews(ctx, ses.ID)
	if len(list) != 1 || list[0].Session.ID != r.ID || !strings.Contains(list[0].Reply, "misspelt") {
		t.Fatalf("reviews = %+v", list)
	}
	msg, err := e.ReviewFindings(ctx, r.ID)
	if err != nil || !strings.Contains(msg, "Second opinion") || !strings.Contains(msg, "change.txt:1") {
		t.Errorf("findings = %q, %v", msg, err)
	}
	if _, err := e.ReviewFindings(ctx, ses.ID); err == nil {
		t.Error("findings from a session that is no review")
	}
	if _, err := e.RequestReview(ctx, ReviewRequest{SessionID: r.ID, ProfileID: "default"}); err == nil {
		t.Error("a reviewer was reviewed")
	}
	// It goes with the session.
	if err := e.ArchiveSession(ctx, ses.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.store.GetSession(ctx, r.ID); got.Status != session.StatusArchived {
		t.Errorf("reviewer after the archive: %s", got.Status)
	}
}

func tagsOf(t *testing.T, e *Engine, sid string) []string {
	t.Helper()
	s, err := e.store.GetSession(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	return s.Tags
}
