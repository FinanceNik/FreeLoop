package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// HRN-016: one task on two profiles makes two worktree sessions with the
// same message; the comparison shows each run's changes and reply, and
// keeping one archives the other.
func TestCompare(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	gitIn(t, repo, "config", "user.name", "t")
	gitIn(t, repo, "config", "user.email", "t@t")
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	for id, content := range map[string]string{"one": "a\n", "two": "a\nb\nc\n"} {
		p := scripted.New(id,
			scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": id + ".txt", "content": content}}}},
			scripted.Step{Text: "done by " + id})
		e.mu.Lock()
		e.providers.built[id] = &configuredProvider{cfg: config.Provider{ID: id, Name: id, Type: "scripted"}, p: p}
		e.mu.Unlock()
	}
	req := CompareRequest{ProjectRoot: repo, Mode: "accept_edits", Prompt: "write your file", Profiles: []CompareProfile{
		{ProfileID: "default", ProviderID: "one", Model: "m", Label: "one"},
	}}
	if _, err := e.CreateCompare(ctx, req); err == nil {
		t.Error("a comparison of one profile was made")
	}
	req.Profiles = append(req.Profiles, CompareProfile{ProfileID: "default", ProviderID: "two", Model: "m", Label: "two"})
	if _, err := e.CreateCompare(ctx, req); err == nil || !strings.Contains(err.Error(), "trust") {
		t.Errorf("untrusted project: %v", err)
	}
	e.SetProjectTrust(ctx, repo, true)
	tag, err := e.CreateCompare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	runs, _ := e.Compare(ctx, tag)
	if len(runs) != 2 {
		t.Fatalf("runs = %d", len(runs))
	}
	waitIdle(t, ch, runs[0].Session.ID, runs[1].Session.ID)
	runs, err = e.Compare(ctx, tag)
	if err != nil {
		t.Fatal(err)
	}
	one, two := runs[0], runs[1]
	if one.Session.Title != "write your file · one" || one.Session.Worktree == two.Session.Worktree || one.Session.Worktree == "" {
		t.Fatalf("sessions = %+v / %+v", one.Session, two.Session)
	}
	if one.Files != 1 || two.Files != 1 || one.Added != 1 || two.Added != 3 || one.LastReply != "done by one" || two.LastReply != "done by two" || one.Tokens == 0 {
		t.Errorf("runs = %+v / %+v", one, two)
	}
	if err := e.KeepCompare(ctx, tag, one.Session.ID); err != nil {
		t.Fatal(err)
	}
	runs, _ = e.Compare(ctx, tag)
	if runs[0].Session.Status == session.StatusArchived || runs[1].Session.Status != session.StatusArchived {
		t.Errorf("after keeping one: %s / %s", runs[0].Session.Status, runs[1].Session.Status)
	}
}
