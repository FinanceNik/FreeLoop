package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// SES-009: a fork has the conversation up to the message it was forked
// before, in a transcript of its own; the model sees that history on the
// fork's first turn; the copied model calls are not counted again.
func TestForkSession(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	p := scripted.New("s", scripted.Step{Text: "one"}, scripted.Step{Text: "two"}, scripted.Step{Text: "three"})
	e.mu.Lock()
	e.providers.built["s"] = &configuredProvider{cfg: config.Provider{ID: "s", Name: "s", Type: "scripted"}, p: p}
	e.mu.Unlock()
	src, err := e.CreateSession(ctx, CreateSessionRequest{Title: "Plan", ProjectRoot: t.TempDir(), ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, src.ID, "first")
	waitIdle(t, ch, src.ID)
	e.Prompt(ctx, src.ID, "second")
	waitIdle(t, ch, src.ID)

	evs, _ := e.Events(ctx, src.ID, 0)
	var at int64
	for _, ev := range evs {
		if ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), `"second"`) {
			at = ev.Seq
		}
	}
	res, err := e.ForkSession(ctx, src.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "second" || res.Session.ID == src.ID || res.Session.Title != "Plan (fork)" {
		t.Fatalf("fork = %+v", res)
	}
	fevs, _ := e.Events(ctx, res.Session.ID, 0)
	all, _ := json.Marshal(fevs)
	if strings.Contains(string(all), `"second"`) || !strings.Contains(string(all), `"first"`) || !strings.Contains(string(all), "Forked from") {
		t.Errorf("the fork's transcript:\n%s", all)
	}
	if got := builtin.Restore(fevs); len(got) != 2 {
		t.Errorf("the fork restores %d messages, want first and its reply", len(got))
	}
	if ses, _ := e.store.GetSession(ctx, res.Session.ID); ses.Status != "idle" {
		t.Errorf("fork status = %s", ses.Status)
	}

	if _, err := e.Prompt(ctx, res.Session.ID, "other"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, res.Session.ID)
	reqs := p.Requests()
	last := reqs[len(reqs)-1].Messages
	if len(last) != 3 || last[0].Parts[0].Text != "first" || last[2].Parts[0].Text != "other" {
		t.Errorf("the fork's first call sent %+v", last)
	}

	days, _ := e.Usage(ctx, 1)
	if len(days) != 1 || days[0].Models[0].Calls != 3 {
		t.Errorf("usage = %+v; want the source's two calls and the fork's one", days)
	}
}

// A fork of a worktree session gets a worktree of its own, branched from
// the source's committed work.
func TestForkWorktreeSession(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	gitIn(t, repo, "config", "user.name", "t")
	gitIn(t, repo, "config", "user.email", "t@t")
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	withScript(e, "s",
		scripted.Step{Calls: []scripted.Call{{Name: "write_file", Input: map[string]any{"path": "a.txt", "content": "from the source\n"}}}},
		scripted.Step{Text: "done"},
		scripted.Step{Text: "and again"},
	)
	e.SetProjectTrust(ctx, repo, true)
	src, err := e.CreateSession(ctx, CreateSessionRequest{Title: "Src", ProjectRoot: repo, ProviderID: "s", Model: "m", Mode: "accept_edits", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, src.ID, "write it")
	waitIdle(t, ch, src.ID)
	e.StageFiles(ctx, src.ID, []string{"a.txt"}, true)
	if _, err := e.Commit(ctx, src.ID, "source work"); err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, src.ID, "more")
	waitIdle(t, ch, src.ID)
	evs, _ := e.Events(ctx, src.ID, 0)
	var at int64
	for _, ev := range evs {
		if ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), `"more"`) {
			at = ev.Seq
		}
	}
	res, err := e.ForkSession(ctx, src.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	f := res.Session
	if f.Worktree == "" || f.Worktree == src.Worktree || f.WorktreeBranch == src.WorktreeBranch {
		t.Fatalf("fork = %+v", f)
	}
	if b, _ := os.ReadFile(filepath.Join(f.Worktree, "a.txt")); string(b) != "from the source\n" {
		t.Errorf("the fork's worktree has a.txt = %q", b)
	}
}

// SES-009, SEC-001: a fork of a session in bypass starts in ask, and keeps
// its own title when the source was titled from its first message.
func TestForkResetsBypassAndKeepsItsTitle(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	cfg := e.Config()
	cfg.Permissions.AllowBypass = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	withScript(e, "s", scripted.Step{Text: "one"}, scripted.Step{Text: "two"})
	root := t.TempDir()
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	src, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetMode(ctx, src.ID, "bypass"); err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, src.ID, "first message here")
	waitIdle(t, ch, src.ID)
	e.Prompt(ctx, src.ID, "second")
	waitIdle(t, ch, src.ID)
	titled, _ := e.store.GetSession(ctx, src.ID)
	evs, _ := e.Events(ctx, src.ID, 0)
	var at int64
	for _, ev := range evs {
		if ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), `"second"`) {
			at = ev.Seq
		}
	}
	res, err := e.ForkSession(ctx, src.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	fork, _ := e.store.GetSession(ctx, res.Session.ID)
	if fork.Mode != "ask" {
		t.Errorf("the fork's mode is %s", fork.Mode)
	}
	if fork.Title != titled.Title+" (fork)" {
		t.Errorf("the fork's title is %q, the source's %q", fork.Title, titled.Title)
	}
}
