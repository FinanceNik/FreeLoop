package permission

import (
	"path/filepath"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

func shellReq(cmd string) Request { return Request{Tool: "shell", Kind: KindExecute, Command: cmd} }

// A deny rule compares the words the shell will run, not the way they are spelled: every
// word is read with its quotes and escapes removed, and zsh's own prefixes are wrappers.
// Reproduced on 2026-11-11: with `deny git push *` and `allow git *`, `git "push" origin
// main` was allowed by the rule in ask mode (SEC-003).
func TestShellDenyRulesSeeThroughQuotedWordsAndZshPrefixes(t *testing.T) {
	r := Rule{Effect: Deny, Tool: "shell", Command: "git push *"}
	for _, line := range []string{
		`git push origin main`, `git "push" origin main`, `git 'push' origin`, `git pu""sh origin`, `git p\ush origin`,
		`git "p"'u'sh origin`, `"git" "push" origin`, `git -C . "push"`, `git -c a=b --no-pager 'push' -f`,
		`noglob git push origin`, `nocorrect git push origin`, `=git push origin`, `command git push origin`,
		`builtin command git push`, `env A=1 git "push"`, `sudo -u root git "push"`,
		`eval "git push origin"`, `eval git push origin`, `eval 'git "push" origin'`,
		`sh -c 'git "push" origin'`, `echo hi && git "push"`, `(git "push")`, `{ git "push"; }`,
	} {
		if !r.Matches(shellReq(line), "/p") {
			t.Errorf("%q got past the rule", line)
		}
	}
	for _, line := range []string{`git pull`, `git "pull" origin`, `echo git push`, `git status`, `echo "git push"`, `git log --grep push`} {
		if r.Matches(shellReq(line), "/p") {
			t.Errorf("%q matched the rule", line)
		}
	}
}

// Flag letters are read as a set, in any order and split or joined, and rm's long names
// are its short ones; a command with more flags than the rule names is still the command.
func TestShellDenyRulesReadFlagsAsASet(t *testing.T) {
	r := Rule{Effect: Deny, Tool: "shell", Command: "rm -rf *"}
	for _, line := range []string{
		`rm -rf x`, `rm -fr x`, `rm -r -f x`, `rm -f -r x`, `rm -Rf x`, `rm --recursive --force x`, `rm --force -r x`,
		`rm -rfv x`, `rm -v -rf x`, `rm -r -v -f x`, `sudo rm -fr /`, `rm "-fr" x`, `rm -rf`,
	} {
		if !r.Matches(shellReq(line), "/p") {
			t.Errorf("%q got past the rule", line)
		}
	}
	for _, line := range []string{`rm -r x`, `rm -f x`, `rm x`, `rm --recursive x`, `git rm -rf x`, `echo rm -rf x`} {
		if r.Matches(shellReq(line), "/p") {
			t.Errorf("%q matched the rule", line)
		}
	}
	// A rule that names no flags is unchanged, and so is an exact rule.
	if !(Rule{Effect: Deny, Tool: "shell", Command: "git push -f *"}).Matches(shellReq(`git push -f origin`), "/p") {
		t.Error("git push -f")
	}
	if (Rule{Effect: Deny, Tool: "shell", Command: "git push --force-with-lease"}).Matches(shellReq(`git push --force`), "/p") {
		t.Error("--force-with-lease is not --force")
	}
}

// A command whose program, or the subcommand after it, is built when it runs cannot be
// compared with a deny rule. It is never allowed by a rule that allows commands, and
// bypass mode refuses it while a shell deny rule exists; with none it runs as before.
func TestACommandBuiltAtRunTimeIsNotAllowedUnseenWhileADenyRuleExists(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	root := filepath.Join(home, "repo")
	deny := Rule{Effect: Deny, Tool: "shell", Command: "git push *", Scope: ScopeGlobal}
	allowShell := Rule{Effect: Allow, Tool: "shell", Scope: ScopeGlobal}
	for _, line := range []string{`a=git; $a push origin`, `git "$sub" origin`, "$(echo git) push", "`echo git` push", `g=git; "$g" push`, `git $'push' origin`, `${cmd} push`, `g*t push origin`, `/usr/bin/gi[t] push`, `/usr/bin/g?t push`} {
		b.SetRules(fixedRules{deny, allowShell}, nil)
		if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, shellReq(line)); decided {
			t.Errorf("%q in ask mode with allow shell was decided: %+v", line, d)
		}
		if d, decided := b.evaluate(Context{Mode: ModeAcceptEdits, Root: root}, shellReq(line)); decided {
			t.Errorf("%q in accept-edits mode with allow shell was decided: %+v", line, d)
		}
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); !decided || d.Outcome != Denied {
			t.Errorf("%q in bypass mode: %+v, %v", line, d, decided)
		}
		// With no shell deny rule nothing needs to be compared.
		b.SetRules(fixedRules{allowShell}, nil)
		if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, shellReq(line)); !decided || d.Outcome != Allowed {
			t.Errorf("%q with no deny rule: %+v, %v", line, d, decided)
		}
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); !decided || d.Outcome != Allowed {
			t.Errorf("%q in bypass with no deny rule: %+v, %v", line, d, decided)
		}
	}
	// Variables and expansions in the arguments are everyday and still run.
	b.SetRules(fixedRules{deny, allowShell}, nil)
	for _, line := range []string{`echo $HOME`, `git commit -m "$msg"`, `ls $(pwd)`, `FOO=$(date) make`, `git status > out.txt`, `cat < in.txt | wc -l`, `[ -f go.mod ] && echo ok`, `[[ -n $x ]] && echo ok`, `ls *.go`, `echo ?`} {
		if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, shellReq(line)); !decided || d.Outcome != Allowed {
			t.Errorf("%q: %+v, %v", line, d, decided)
		}
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); !decided || d.Outcome != Allowed {
			t.Errorf("%q in bypass: %+v, %v", line, d, decided)
		}
	}
}

// A deny rule on one of the two ways to write a file holds for the other, and for an
// external agent's write, which is judged as an edit: the model chooses the tool, the
// user's rule is about the path. Reproduced on 2026-11-11: `deny write_file(secrets/**)`
// did not stop edit_file (SEC-003). An allow rule stays as narrow as it was written.
func TestAPathDenyRuleOnAWriteToolCoversTheOther(t *testing.T) {
	root := "/p"
	req := func(tool string) Request {
		return Request{Tool: tool, Kind: KindEdit, Paths: []string{"/p/secrets/key.txt"}}
	}
	for _, c := range []struct{ rule, tool string }{
		{"write_file", "edit_file"}, {"edit_file", "write_file"}, {"write_file", "write_file"}, {"edit_file", "edit_file"},
		{"write_*", "edit_file"}, {"*", "edit_file"},
	} {
		r := Rule{Effect: Deny, Tool: c.rule, Path: "secrets/**"}
		if !r.Matches(req(c.tool), root) {
			t.Errorf("deny %s(secrets/**) does not hold for %s", c.rule, c.tool)
		}
	}
	for _, c := range []struct{ rule, tool string }{
		{"read_file", "edit_file"}, {"grep", "write_file"}, {"shell", "edit_file"}, {"write_file", "read_file"}, {"write_file", "grep"},
	} {
		r := Rule{Effect: Deny, Tool: c.rule, Path: "secrets/**"}
		if r.Matches(Request{Tool: c.tool, Kind: KindEdit, Paths: []string{"/p/secrets/key.txt"}}, root) {
			t.Errorf("deny %s(secrets/**) holds for %s", c.rule, c.tool)
		}
	}
	// An allow rule is only for the tool it names.
	allow := Rule{Effect: Allow, Tool: "write_file", Path: "src/**"}
	if allow.Matches(Request{Tool: "edit_file", Kind: KindEdit, Paths: []string{"/p/src/a.go"}}, root) {
		t.Error("allow write_file(src/**) allows edit_file")
	}
	// And a deny rule with a command is unchanged.
	if (Rule{Effect: Deny, Tool: "write_file", Command: "x"}).Matches(Request{Tool: "edit_file", Command: "x"}, root) {
		t.Error("a command rule on write_file matched edit_file")
	}
}

// Through the broker: a deny rule on write_file stops an external agent's edit, even in
// bypass mode, and `freeloop check` says so.
func TestAWriteDenyRuleStopsAnExternalAgentsEdit(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	root := filepath.Join(home, "repo")
	b.SetRules(fixedRules{{Effect: Deny, Tool: "write_file", Path: "secrets/**", Scope: ScopeGlobal}}, nil)
	for _, mode := range []Mode{ModeAsk, ModeAcceptEdits, ModeBypass} {
		d, decided := b.evaluate(Context{Mode: mode, Root: root}, Request{Tool: "edit_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "secrets", "k.txt")}})
		if !decided || d.Outcome != Denied || d.Actor != ActorRule {
			t.Errorf("%s: %+v, %v", mode, d, decided)
		}
	}
	if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, Request{Tool: "edit_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "src", "a.go")}}); !decided || d.Outcome != Allowed {
		t.Errorf("an edit elsewhere: %+v, %v", d, decided)
	}
}
