package permission

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// Findings of the security review of 2026-11-02.

func TestFoldedSpellingsAreTheSameName(t *testing.T) {
	for _, tc := range []struct {
		p, dir string
		want   bool
	}{
		{"/h/.ssh/id", "/h/.ssh", true},
		{"/h/.SSH/id", "/h/.ssh", true},
		{"/h/.ſsh/id", "/h/.ssh", true}, // long s
		{"/h/café/x", "/h/café", true},
		{"/h/Ky/x", "/h/ky", true}, // Kelvin sign
		{"/h/.sshx/id", "/h/.ssh", false},
	} {
		if got := within(tc.p, tc.dir); got != tc.want {
			t.Errorf("within(%q, %q) = %v", tc.p, tc.dir, got)
		}
	}
}

func TestADenyRuleHoldsForOtherSpellings(t *testing.T) {
	root := environment.Canonical(t.TempDir())
	r := Rule{Effect: Deny, Tool: "read_file", Path: "secrets/**"}
	for _, name := range []string{"secrets/key", "SECRETS/key", "secretſ/key"} {
		req := Request{Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(root, name)}}
		if !r.Matches(req, root) {
			t.Errorf("%s got past the rule", name)
		}
	}
}

func TestWritesThatPlantExecution(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	p := NewPolicy(home, filepath.Join(home, "data"))
	for _, rel := range []string{".git/config", "sub/.git/config", "sub/.git/hooks/pre-commit", ".freeloop/hooks.yaml", "sub/.freeloop/hooks.yaml", "sub/.GIT/config"} {
		if p.writeRefusal(root, filepath.Join(root, rel)) == "" {
			t.Errorf("%s is writable", rel)
		}
	}
	for _, rel := range []string{"a.go", "sub/a.go", ".gitignore", "x.git/y", ".github/workflows/ci.yml"} {
		if why := p.writeRefusal(root, filepath.Join(root, rel)); why != "" {
			t.Errorf("%s refused: %s", rel, why)
		}
	}
	for _, rel := range []string{".envrc", "sub/.envrc", ".vscode/tasks.json", ".mcp.json", ".husky/pre-commit", ".claude/settings.json", ".gemini/settings.json", ".codex/config.toml", ".pre-commit-config.yaml", ".cursor/mcp.json"} {
		if !p.actedOn(root, []string{filepath.Join(root, rel)}) {
			t.Errorf("%s is not treated as acted on later", rel)
		}
	}
	if p.actedOn(root, []string{filepath.Join(root, "src/a.go"), filepath.Join(root, "docs/.vscode-notes")}) {
		t.Error("an ordinary edit counted as acted on later")
	}
}

func TestVendorSignInsAreNeverRead(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	p := NewPolicy(home, filepath.Join(home, "data"))
	for _, rel := range []string{".codex/auth.json", ".gemini/oauth_creds.json", ".claude/.credentials.json", ".claude.json", ".netrc", ".npmrc", ".config/gh/hosts.yml", ".docker/config.json", ".kube/config", ".git-credentials"} {
		if p.readRefusal(filepath.Join(home, rel)) == "" {
			t.Errorf("%s is readable", rel)
		}
	}
	if p.readRefusal(filepath.Join(home, ".claude/settings.json")) != "" {
		t.Error("an agent's plain settings were closed too")
	}
}

func TestAllowAlwaysNeverWidensForAnAgentsTool(t *testing.T) {
	for _, req := range []Request{
		{Tool: "acp:other", Kind: KindOther},
		{Tool: "acp:fetch", Kind: KindFetch},
		{Tool: "edit_file", Kind: KindEdit},
		{Tool: "read_file", Kind: KindRead},
		{Tool: "grep", Kind: KindSearch},
	} {
		if r, ok := RuleFor(req, "/p", ScopeGlobal); ok {
			t.Errorf("%s made the rule %+v", req.Tool, r)
		}
	}
	if _, ok := RuleFor(Request{Tool: "mcp__s__t", Kind: KindOther}, "/p", ScopeSession); !ok {
		t.Error("an MCP tool has a stable name and can be allowed always")
	}
}

func TestShellDenyRulesSeeThroughWrappers(t *testing.T) {
	r := Rule{Effect: Deny, Tool: "shell", Command: "rm *"}
	for _, line := range []string{
		"rm x", "env -u F rm x", "env -C d rm x", "exec -a n rm x", "nice -n 5 rm x", "sudo -u root rm x", "timeout 5 rm x",
		"bash -x -c 'rm x'", "bash --norc -c 'rm x'", "! rm x", "if true; then rm x; fi", "for f in a; do rm x; done",
		"echo $(rm x)", "echo `rm x`", "'rm' x", "r\\m x", "\"rm\" x", "xargs rm x", "/bin/rm x",
	} {
		if !r.Matches(Request{Tool: "shell", Kind: KindExecute, Command: line}, "/p") {
			t.Errorf("%q got past the rule", line)
		}
	}
	for _, line := range []string{"echo rm", "git rm x", "npm run format", "cat rm.txt"} {
		if r.Matches(Request{Tool: "shell", Kind: KindExecute, Command: line}, "/p") {
			t.Errorf("%q matched the rule", line)
		}
	}
}

func TestBypassDoesNotLeaveTheSandbox(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	pc := Context{Mode: ModeBypass, Root: filepath.Join(home, "repo")}
	if d, decided := b.evaluate(pc, Request{Tool: "shell", Kind: KindExecute, Command: "x", LeavesSandbox: true}); decided {
		t.Errorf("leaving the sandbox was decided without asking: %+v", d)
	}
	if d, decided := b.evaluate(pc, Request{Tool: "shell", Kind: KindExecute, Command: "x"}); !decided || d.Outcome != Allowed {
		t.Errorf("bypass otherwise: %+v, %v", d, decided)
	}
}

func TestASandboxAsksAboutExpansionsWhenARuleCouldBeHidden(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	rules := fixedRules{{Effect: Deny, Tool: "shell", Command: "rm *", Scope: ScopeGlobal}}
	b.SetRules(rules, nil)
	pc := Context{Mode: ModeAsk, Root: filepath.Join(home, "repo")}
	for line, wantDecided := range map[string]bool{"ls -la": true, `eval "$(cat run.sh)"`: false, "a=rm; $a x": false} {
		_, decided := b.evaluate(pc, Request{Tool: "shell", Kind: KindExecute, Command: line, Contained: ContainedProject})
		if decided != wantDecided {
			t.Errorf("%q: decided = %v", line, decided)
		}
	}
	b.SetRules(fixedRules{}, nil)
	if _, decided := b.evaluate(pc, Request{Tool: "shell", Kind: KindExecute, Command: "a=x; $a", Contained: ContainedProject}); !decided {
		t.Error("with no shell deny rule a sandboxed command needs no approval")
	}
}

type fixedRules []Rule

func (f fixedRules) Rules(string) ([]Rule, error) { return f, nil }
