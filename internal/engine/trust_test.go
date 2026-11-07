package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// SEC-006 and SET-002: an untrusted project runs with global config only;
// once trusted, its AGENTS.md, .freeloop/ profiles and deny rules apply, a
// project deny overrides a global allow, and nothing in .freeloop/ can
// loosen permissions.
func TestProjectTrustAndSettings(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	prov := scripted.New("p",
		scripted.Step{Calls: []scripted.Call{{Name: "shell", Input: map[string]any{"command": "echo hi"}}}},
		scripted.Step{Text: "done"},
	)
	e.mu.Lock()
	e.providers.built["p"] = &configuredProvider{cfg: config.Provider{ID: "p", Name: "p", Type: "scripted"}, p: prov}
	e.mu.Unlock()
	if err := e.AddRule(permission.Rule{Effect: permission.Allow, Tool: "shell", Command: "echo *"}); err != nil {
		t.Fatal(err)
	}

	root := gitRepo(t)
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	write("AGENTS.md", "Project rule: tabs.")
	write(".freeloop/rules.yaml", "rules:\n  - effect: deny\n    tool: shell\n    command: echo *\n  - effect: allow\n    tool: shell\n    command: rm *\n")
	write(".freeloop/profiles/review.yaml", "name: Project review\nprovider: p\nmodel: m\nmode: plan\nrules:\n  - effect: deny\n    tool: write_file\n")
	write(".freeloop/profiles/loose.yaml", "name: Loose\nmode: accept_edits\n")
	write(".freeloop/profiles/allows.yaml", "name: Allows\nrules:\n  - effect: allow\n    tool: shell\n")

	// Untrusted: no project settings, no instructions, no git status.
	st, _ := e.ProjectSettings(ctx, root)
	if st.Trusted || len(st.Profiles) != 0 {
		t.Fatalf("untrusted settings = %+v", st)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "p", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "project:review"}); err == nil {
		t.Error("a project profile was usable before trust")
	}
	evs, _ := e.Events(ctx, ses.ID, 0)
	for _, ev := range evs {
		if ev.Type == event.GitState && !strings.Contains(string(ev.Data), `"dirty":-1`) {
			t.Errorf("git status ran in an untrusted project: %s", ev.Data)
		}
	}
	if c, _ := e.Changes(ctx, ses.ID); c.Blocked == "" {
		t.Error("the diff panel works in an untrusted project")
	}
	e.Prompt(ctx, ses.ID, "go")
	waitIdle(t, ch, ses.ID)
	if sys := prov.Requests()[0].System; strings.Contains(sys, "Project rule") {
		t.Error("AGENTS.md was read before trust")
	}
	if !decided(t, e, ses.ID, permission.Allowed) {
		t.Error("the global allow did not apply in the untrusted project")
	}

	// Trusted: settings load, and only what tightens.
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	st, _ = e.ProjectSettings(ctx, root)
	if !st.Trusted || len(st.Profiles) != 1 || st.Profiles[0].ID != "project:review" || len(st.Rules) != 1 {
		t.Fatalf("trusted settings = %+v", st)
	}
	var msgs []string
	for _, is := range st.Issues {
		msgs = append(msgs, is.File+": "+is.Message)
	}
	joined := strings.Join(msgs, "\n")
	for _, want := range []string{"loose.yaml: a project profile can start in ask or plan only", "allows.yaml: a project profile can only add deny rules", "rules.yaml: project rules can only deny"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues lack %q:\n%s", want, joined)
		}
	}
	e.Prompt(ctx, ses.ID, "again")
	waitIdle(t, ch, ses.ID)
	reqs := prov.Requests()
	if !strings.Contains(reqs[len(reqs)-1].System, "Project rule") {
		t.Error("AGENTS.md not read after trust")
	}
	if !decided(t, e, ses.ID, permission.Denied) {
		t.Error("the project deny did not override the global allow")
	}

	ps, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "project:review"})
	if err != nil || ps.Mode != "plan" || ps.ProfileID != "project:review" {
		t.Fatalf("project profile session = %+v, %v", ps, err)
	}
	found := false
	for _, r := range e.ListRules(ps.ID) {
		found = found || (r.Scope == permission.ScopeProject && r.Tool == "write_file")
	}
	if !found {
		t.Error("the project profile's deny rule is not in force")
	}
}

// decided reports whether the session's last permission decision had
// outcome.
func decided(t *testing.T, e *Engine, sid string, outcome permission.Outcome) bool {
	t.Helper()
	evs, _ := e.Events(context.Background(), sid, 0)
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == event.PermissionDecided {
			var d permission.Decision
			json.Unmarshal(evs[i].Data, &d)
			return d.Outcome == outcome
		}
	}
	return false
}

// SEC-002: rules that cannot be read may include deny rules, so a session whose
// environment, profile or record cannot be loaded gets an error, and the broker asks,
// rather than judging by the rules that are left.
func TestScopedRulesFailClosed(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	if err := e.SaveProfileYAML("guarded", "name: Guarded\nprovider: demo\nrules:\n  - effect: deny\n    tool: shell\n    command: git push *\n"); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProfileID: "guarded"})
	if err != nil {
		t.Fatal(err)
	}
	if rules, err := e.rules.Rules(ses.ID); err != nil || !slices.ContainsFunc(rules, func(r permission.Rule) bool { return r.Command == "git push *" }) {
		t.Fatalf("with the environment: %v, %v", rules, err)
	}
	if err := e.DeleteProfile("guarded"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.rules.Rules(ses.ID); err == nil {
		t.Error("the profile's deny rules were dropped without an error")
	}
	if _, err := e.rules.Rules("ses_unknown"); err == nil {
		t.Error("an unknown session's rules came back complete")
	}
	// And so the broker asks: a shell call is not allowed by the remaining rules.
	if err := e.AddRule(permission.Rule{Effect: permission.Allow, Tool: "shell"}); err != nil {
		t.Fatal(err)
	}
	rules, rerr := e.rules.Rules(ses.ID)
	if d := e.broker.Explain(permission.Context{Mode: permission.ModeAsk, Root: ses.ProjectRoot}, permission.Request{SessionID: ses.ID, Tool: "shell", Kind: permission.KindExecute, Command: "git push origin main"}, rules, rerr); d.Outcome != permission.Ask {
		t.Errorf("after the profile went, git push is %s: %s", d.Outcome, d.Reason)
	}
}
