package permission

import (
	"errors"
	"path/filepath"
	"testing"
)

// Explain must say what Request does, for every kind of decision (SEC-018).
func TestExplainAgreesWithRequest(t *testing.T) {
	b, _, home, root := setup(t)
	rules := &memRules{}
	b.SetRules(rules, rules)
	at := func(p ...string) []string { return []string{filepath.Join(append([]string{root}, p...)...)} }
	shell := func(cmd string) Request {
		return Request{SessionID: "s", Tool: "shell", Kind: KindExecute, Command: cmd}
	}
	edit := func(p ...string) Request {
		return Request{SessionID: "s", Tool: "edit_file", Kind: KindEdit, Paths: at(p...)}
	}
	read := func(p ...string) Request {
		return Request{SessionID: "s", Tool: "read_file", Kind: KindRead, Paths: at(p...)}
	}
	pushDeny := Rule{Effect: Deny, Tool: "shell", Command: "git push *", Scope: ScopeGlobal, Source: "rules.yaml"}
	shellAllow := Rule{Effect: Allow, Tool: "shell", Scope: ScopeProfile, Source: "work.yaml"}

	for _, c := range []struct {
		name  string
		mode  Mode
		rules []Rule
		err   error
		req   Request
		want  string
		actor Actor
	}{
		{"read inside", ModeAsk, nil, nil, read("a.go"), "allowed", ActorPolicy},
		{"read a secret", ModeAsk, nil, nil, read(".env"), Ask, ""},
		{"read ssh key", ModeBypass, nil, nil, Request{SessionID: "s", Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(home, ".ssh", "id_ed25519")}}, "denied", ActorProtectedPath},
		{"shell asks", ModeAsk, nil, nil, shell("make test"), Ask, ""},
		{"deny beats allow", ModeAsk, []Rule{shellAllow, pushDeny}, nil, shell("git push origin main"), "denied", ActorRule},
		{"deny holds in bypass", ModeBypass, []Rule{pushDeny}, nil, shell("git push origin main"), "denied", ActorRule},
		{"allow rule", ModeAsk, []Rule{shellAllow, pushDeny}, nil, shell("make test"), "allowed", ActorRule},
		{"plan refuses writes", ModePlan, []Rule{{Effect: Allow, Tool: "edit_file", Scope: ScopeGlobal}}, nil, edit("a.go"), "denied", ActorMode},
		{"accept edits", ModeAcceptEdits, nil, nil, edit("a.go"), "allowed", ActorMode},
		{"accept edits on a git hook", ModeAcceptEdits, nil, nil, edit(".git", "hooks", "pre-commit"), "denied", ActorProtectedPath},
		{"accept edits on envrc", ModeAcceptEdits, nil, nil, edit(".envrc"), Ask, ""},
		{"bypass", ModeBypass, nil, nil, shell("rm -rf build"), "allowed", ActorMode},
		{"unreadable rules", ModeAsk, []Rule{shellAllow}, errors.New("broken"), shell("make test"), Ask, ""},
		{"sandboxed", ModeAsk, nil, nil, Request{SessionID: "s", Tool: "shell", Kind: KindExecute, Command: "make", Contained: ContainedProject}, "allowed", ActorSandbox},
	} {
		rules.global, rules.err = c.rules, c.err
		pc := Context{Mode: c.mode, Root: root}
		v := b.Explain(pc, c.req, c.rules, c.err)
		d := decide(t, b, pc, c.req, "reject_once")
		did := string(d.Outcome)
		if d.Actor == ActorUser {
			did = Ask
		}
		if v.Outcome != did || (did != Ask && v.Actor != d.Actor) {
			t.Errorf("%s: Explain says %s by %s, Request did %s by %s", c.name, v.Outcome, v.Actor, d.Outcome, d.Actor)
		}
		if v.Outcome != c.want || v.Actor != c.actor {
			t.Errorf("%s: got %s by %s (%s), want %s by %s", c.name, v.Outcome, v.Actor, v.Reason, c.want, c.actor)
		}
		if v.Reason == "" {
			t.Errorf("%s: no reason", c.name)
		}
	}
}

func TestExplainNamesWhatItOverruled(t *testing.T) {
	b, rec, _, root := setup(t)
	allow := Rule{Effect: Allow, Tool: "shell", Scope: ScopeProfile}
	deny := Rule{Effect: Deny, Tool: "shell", Command: "git push *", Scope: ScopeGlobal}
	v := b.Explain(Context{Mode: ModeAsk, Root: root}, Request{Tool: "shell", Kind: KindExecute, Command: "git push"}, []Rule{allow, deny}, nil)
	if v.Rule == nil || *v.Rule != deny || len(v.Overruled) != 1 || v.Overruled[0] != allow {
		t.Errorf("verdict = %+v", v)
	}
	if len(rec.events) != 0 || len(b.Pending()) != 0 {
		t.Errorf("Explain recorded %v or queued %d", rec.events, len(b.Pending()))
	}
}
