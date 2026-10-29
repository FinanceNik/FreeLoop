package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
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
