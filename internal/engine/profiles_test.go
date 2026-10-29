package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

func toolNames(req provider.ChatRequest) []string {
	var out []string
	for _, t := range req.Tools {
		out = append(out, t.Name)
	}
	return out
}

// HRN-003: two profiles that differ in model and tool set run side by side,
// each session getting its own model, tools, prompt and rules.
func TestProfilesRunSideBySide(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	prov := scripted.New("p", scripted.Step{Text: "hello"})
	e.mu.Lock()
	e.providers.built["p"] = &configuredProvider{cfg: config.Provider{ID: "p", Name: "p", Type: "scripted"}, p: prov}
	e.mu.Unlock()

	if _, err := e.SaveProfile(config.Profile{
		ID: "reader", Name: "Reader", Provider: "p", Model: "small", Mode: "plan",
		SystemPrompt: "Answer like a librarian.", Tools: []string{"read_file", "grep"}, Instructions: "none",
		Rules: []permission.Rule{{Effect: permission.Deny, Tool: "shell", Command: "git push *"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SaveProfile(config.Profile{ID: "writer", Name: "Writer", Provider: "p", Model: "large", Effort: "max"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SaveProfile(config.Profile{ID: "bad", Name: "Bad", Tools: []string{"teleport"}}); err == nil {
		t.Error("saved a profile with an unknown tool")
	}
	if got := len(e.ListProfiles()); got != 3 { // default, reader, writer
		t.Fatalf("profiles = %d", got)
	}

	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Project rule: tabs."), 0o644)
	e.SetProjectTrust(ctx, root, true)
	r, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "reader"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Model != "small" || r.Mode != "plan" || r.ProfileID != "reader" || w.Model != "large" || w.Mode != "ask" || w.ProfileID != "writer" {
		t.Fatalf("sessions = %+v / %+v", r, w)
	}
	e.Prompt(ctx, r.ID, "hi")
	e.Prompt(ctx, w.ID, "hi")
	waitIdle(t, ch, r.ID, w.ID)

	var reader, writer provider.ChatRequest
	for _, req := range prov.Requests() {
		switch req.Model {
		case "small":
			reader = req
		case "large":
			writer = req
		}
	}
	// Plan mode offers the read-only tools of the profile plus present_plan.
	if got := toolNames(reader); !slices.Equal(got, []string{"read_file", "grep", "present_plan"}) {
		t.Errorf("reader tools = %v", got)
	}
	if !strings.Contains(reader.System, "librarian") || strings.Contains(reader.System, "Project rule") {
		t.Errorf("reader system prompt lacks the profile prompt or has the project's instructions:\n%s", reader.System)
	}
	if len(toolNames(writer)) < 6 || writer.Effort != "max" || !strings.Contains(writer.System, "Project rule") {
		t.Errorf("writer tools = %v, effort %q", toolNames(writer), writer.Effort)
	}

	rules := e.ListRules(r.ID)
	if !slices.ContainsFunc(rules, func(x permission.Rule) bool { return x.Scope == permission.ScopeProfile && x.Command == "git push *" }) {
		t.Errorf("reader rules = %+v", rules)
	}
	if slices.ContainsFunc(e.ListRules(w.ID), func(x permission.Rule) bool { return x.Scope == permission.ScopeProfile }) {
		t.Error("writer sees the reader's profile rules")
	}

	// The project's default profile is used when none is named.
	p, _ := e.AddProject(ctx, root)
	if err := e.SetProjectProfile(ctx, p.ID, "writer"); err != nil {
		t.Fatal(err)
	}
	if s, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root}); err != nil || s.ProfileID != "writer" || s.Model != "large" {
		t.Errorf("default-profile session = %+v, %v", s, err)
	}
}

// GUI-004: a profile's slash commands are advertised to the UI and a prompt
// that starts with one sends the command's prompt, with the rest appended;
// the transcript keeps what the user typed.
func TestProfileSlashCommands(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	prov := scripted.New("p", scripted.Step{Text: "ok"})
	e.mu.Lock()
	e.providers.built["p"] = &configuredProvider{cfg: config.Provider{ID: "p", Name: "p", Type: "scripted"}, p: prov}
	e.mu.Unlock()
	if _, err := e.SaveProfile(config.Profile{ID: "rev", Name: "Rev", Provider: "p", Model: "m",
		Commands: []config.Command{{Name: "review", Description: "Review changes", Prompt: "Review the uncommitted changes."}}}); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProfileID: "rev"})
	if err != nil {
		t.Fatal(err)
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	if last := evs[len(evs)-1]; last.Type != event.AvailableCommandsUpdate || !strings.Contains(string(last.Data), `"name":"review"`) {
		t.Fatalf("last event = %s %s", last.Type, last.Data)
	}
	e.Prompt(ctx, ses.ID, "/review  focus on auth")
	waitIdle(t, ch, ses.ID)
	reqs := prov.Requests()
	got := reqs[len(reqs)-1].Messages[0].Parts[0].Text
	if got != "Review the uncommitted changes.\n\nfocus on auth" {
		t.Errorf("model got %q", got)
	}
	evs, _ = e.Events(ctx, ses.ID, 0)
	if !slices.ContainsFunc(evs, func(ev event.Event) bool {
		return ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), "/review  focus on auth")
	}) {
		t.Error("the transcript lost what the user typed")
	}
}
