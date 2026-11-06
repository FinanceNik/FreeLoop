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
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// SEC-018: the check says what the broker would do, in a project or a session,
// with the rule, the scope and what else holds.
func TestCheckPolicy(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	e.mu.Lock()
	e.providers.built["p"] = &configuredProvider{cfg: config.Provider{ID: "p", Name: "p", Type: "scripted"}, p: scripted.New("p")}
	e.mu.Unlock()
	globalAllow := permission.Rule{Effect: permission.Allow, Tool: "shell"}
	if err := e.AddRule(globalAllow); err != nil {
		t.Fatal(err)
	}
	if err := e.AddRule(permission.Rule{Effect: permission.Deny, Tool: "shell", Command: "git push *"}); err != nil {
		t.Fatal(err)
	}
	root := gitRepo(t)
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	write(".freeloop/rules.yaml", "rules:\n  - effect: deny\n    tool: shell\n    command: make deploy\n")
	write(".freeloop/hooks.yaml", "version: 1\nhooks:\n  - event: pre_tool_use\n    match: shell\n    command: exit 0\n")

	check := func(q PolicyQuery) PolicyCheck {
		t.Helper()
		if q.SessionID == "" && q.ProjectRoot == "" {
			q.ProjectRoot = root
		}
		c, err := e.CheckPolicy(ctx, q)
		if err != nil {
			t.Fatalf("%+v: %v", q, err)
		}
		return c
	}
	for _, c := range []struct {
		name  string
		q     PolicyQuery
		want  string
		actor permission.Actor
	}{
		{"a global deny", PolicyQuery{Tool: "shell", Command: "git push origin main"}, "denied", permission.ActorRule},
		{"a global deny in bypass", PolicyQuery{Tool: "shell", Command: "git push origin main", Mode: "bypass"}, "denied", permission.ActorRule},
		{"a global allow", PolicyQuery{Tool: "shell", Command: "make deploy"}, "allowed", permission.ActorRule},
		{"a read", PolicyQuery{Tool: "read", Path: "main.go"}, "allowed", permission.ActorPolicy},
		{"a secret", PolicyQuery{Tool: "read", Path: ".env"}, permission.Ask, ""},
		{"an ssh key", PolicyQuery{Tool: "read", Path: "~/.ssh/id_ed25519", Mode: "bypass"}, "denied", permission.ActorProtectedPath},
		{"a git hook in accept edits", PolicyQuery{Tool: "edit", Path: ".git/hooks/pre-commit", Mode: "accept_edits"}, "denied", permission.ActorProtectedPath},
		{"an envrc in accept edits", PolicyQuery{Tool: "edit", Path: ".envrc", Mode: "accept_edits"}, permission.Ask, ""},
		{"an edit in plan", PolicyQuery{Tool: "edit", Path: "main.go", Mode: "plan"}, "denied", permission.ActorMode},
	} {
		got := check(c.q)
		if got.Outcome != c.want || got.Actor != c.actor {
			t.Errorf("%s: %s by %s (%s), want %s by %s", c.name, got.Outcome, got.Actor, got.Reason, c.want, c.actor)
		}
	}
	got := check(PolicyQuery{Tool: "shell", Command: "git push origin main"})
	if got.Rule == nil || got.Rule.Scope != permission.ScopeGlobal || len(got.Overruled) != 1 || got.Overruled[0].Command != "" {
		t.Errorf("the deny's verdict = %+v", got)
	}
	// Untrusted, the project's rules and hooks do not apply.
	if got := check(PolicyQuery{Tool: "shell", Command: "make deploy"}); len(got.Hooks) != 0 {
		t.Errorf("an untrusted project's hook would run: %+v", got.Hooks)
	}

	// Trusted, the project's deny beats the global allow, and its hook would run.
	if _, err := e.SetProjectTrust(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	got = check(PolicyQuery{Tool: "shell", Command: "make deploy"})
	if got.Outcome != "denied" || got.Rule == nil || got.Rule.Scope != permission.ScopeProject || !slices.ContainsFunc(got.Overruled, func(r permission.Rule) bool { return r.Scope == permission.ScopeGlobal && r.Effect == permission.Allow }) {
		t.Errorf("the project's deny = %+v", got)
	}
	if got := check(PolicyQuery{Tool: "shell", Command: "make test"}); len(got.Hooks) != 1 || got.Outcome != "allowed" {
		t.Errorf("the project's hook = %+v", got)
	}

	// In a session, its own rules count, and its mode is the default.
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProviderID: "p", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	read := PolicyQuery{SessionID: ses.ID, Tool: "read", Path: ".env"}
	if got := check(read); got.Outcome != permission.Ask || got.Mode != "ask" || got.Root != ses.ProjectRoot {
		t.Errorf("before the session's rule: %+v", got)
	}
	if err := e.AddSessionRule(ses.ID, permission.Rule{Effect: permission.Allow, Tool: "read_file", Path: ".env"}); err != nil {
		t.Fatal(err)
	}
	if got := check(read); got.Outcome != "allowed" || got.Rule == nil || got.Rule.Scope != permission.ScopeSession {
		t.Errorf("after the session's rule: %+v", got)
	}

	// An external agent: what it decides itself is said, in the mode it applies to.
	cc := PolicyQuery{ProfileID: config.ClaudeCodePresetID, Tool: "shell", Command: "git status"}
	asks := check(cc)
	cc.Mode = "auto"
	auto := check(cc)
	if asks.Harness != config.ClaudeCodePresetID || len(asks.Agent) != 1 || len(auto.Agent) != 2 || !strings.Contains(auto.Agent[1], "auto mode") {
		t.Errorf("Claude Code's notes: ask %q, auto %q", asks.Agent, auto.Agent)
	}

	for _, q := range []PolicyQuery{{Tool: "shell"}, {Tool: "edit"}, {Command: "ls"}, {Tool: "shell", Command: "ls", Mode: "loose"}} {
		q.ProjectRoot = root
		if _, err := e.CheckPolicy(ctx, q); err == nil {
			t.Errorf("%+v was checked", q)
		}
	}
}

func TestPolicyOverview(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	if err := e.AddRule(permission.Rule{Effect: permission.Deny, Tool: "shell", Command: "git push *"}); err != nil {
		t.Fatal(err)
	}
	root := gitRepo(t)
	o, err := e.PolicyOverview(ctx, PolicyQuery{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if o.Trusted || o.Mode != "ask" || o.Harness != "builtin" || len(o.Scopes) != 4 || len(o.Scopes[0].Rules) != 1 {
		t.Errorf("overview = %+v", o)
	}
	if !slices.Contains(o.ActedOnLater, ".envrc") || !slices.Contains(o.NeverWritten, ".git") || !slices.ContainsFunc(o.Protected, func(p string) bool { return strings.HasSuffix(p, ".ssh") }) {
		t.Errorf("protected paths = %v, %v, %v", o.Protected, o.NeverWritten, o.ActedOnLater)
	}
}
