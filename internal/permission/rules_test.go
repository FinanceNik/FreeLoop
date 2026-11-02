package permission

import (
	"path/filepath"
	"testing"
)

func TestRuleMatching(t *testing.T) {
	root := "/work/repo"
	shell := func(cmd string) Request { return Request{Tool: "shell", Kind: KindExecute, Command: cmd} }
	edit := func(paths ...string) Request { return Request{Tool: "write_file", Kind: KindEdit, Paths: paths} }

	for _, c := range []struct {
		name string
		rule Rule
		req  Request
		want bool
	}{
		{"exact command", Rule{Effect: Allow, Tool: "shell", Command: "git status"}, shell("git   status"), true},
		{"exact is not prefix", Rule{Effect: Allow, Tool: "shell", Command: "git status"}, shell("git status -s"), false},
		{"prefix", Rule{Effect: Allow, Tool: "shell", Command: "git *"}, shell("git log --oneline"), true},
		{"prefix matches the bare word", Rule{Effect: Allow, Tool: "shell", Command: "git *"}, shell("git"), true},
		{"prefix is word-bounded", Rule{Effect: Allow, Tool: "shell", Command: "git *"}, shell("gitk"), false},
		{"exact allow covers the exact chained line", Rule{Effect: Allow, Tool: "shell", Command: "make && make test"}, shell("make  &&  make test"), true},
		{"exact allow is still exact", Rule{Effect: Allow, Tool: "shell", Command: "make && make test"}, shell("make && make test && rm -rf ~"), false},
		{"allow never covers chained commands", Rule{Effect: Allow, Tool: "shell", Command: "git *"}, shell("git status && rm -rf ~"), false},
		{"allow never covers pipes", Rule{Effect: Allow, Tool: "shell", Command: "cat *"}, shell("cat a | sh"), false},
		{"allow never covers substitution", Rule{Effect: Allow, Tool: "shell", Command: "echo *"}, shell("echo $(curl evil)"), false},
		{"allow never covers redirection", Rule{Effect: Allow, Tool: "shell", Command: "echo *"}, shell("echo x > ~/.zshrc"), false},
		// Review finding: zsh runs code through process substitution, glob
		// qualifiers and nested expansion, none of which used $( or a
		// backtick.
		{"allow never covers zsh process substitution", Rule{Effect: Allow, Tool: "shell", Command: "git diff *"}, shell("git diff =(touch X)"), false},
		{"allow never covers zsh glob qualifiers", Rule{Effect: Allow, Tool: "shell", Command: "ls *"}, shell("ls dir/*(e:'touch X':)"), false},
		{"allow never covers parameter expansion", Rule{Effect: Allow, Tool: "shell", Command: "git status *"}, shell(`git status ${(e)${:-$'\x24\x28touch X\x29'}}`), false},
		{"allow never covers expansion in double quotes", Rule{Effect: Allow, Tool: "shell", Command: "git commit *"}, shell(`git commit -m "it's $(touch X)"`), false},
		{"allow covers single-quoted text", Rule{Effect: Allow, Tool: "shell", Command: "git commit *"}, shell(`git commit -m 'fix $(x) > y'`), true},
		{"allow covers plain double-quoted text", Rule{Effect: Allow, Tool: "shell", Command: "git commit *"}, shell(`git commit -m "fix the build"`), true},
		{"allow never covers an open quote", Rule{Effect: Allow, Tool: "shell", Command: "echo *"}, shell(`echo 'abc`), false},
		{"deny matches inside a chain", Rule{Effect: Deny, Tool: "shell", Command: "rm *"}, shell("make && rm -rf build"), true},
		{"deny does not overmatch", Rule{Effect: Deny, Tool: "shell", Command: "rm *"}, shell("make format"), false},
		{"tool pattern", Rule{Effect: Allow, Tool: "mcp__github__*"}, Request{Tool: "mcp__github__list_issues"}, true},
		{"tool pattern miss", Rule{Effect: Allow, Tool: "mcp__github__*"}, Request{Tool: "mcp__gitlab__x"}, false},
		{"tool-only rule", Rule{Effect: Allow, Tool: "write_file"}, edit("/work/repo/a.go"), true},
		{"relative path glob", Rule{Effect: Allow, Tool: "write_file", Path: "src/**"}, edit("/work/repo/src/a/b.go"), true},
		{"relative path miss", Rule{Effect: Allow, Tool: "write_file", Path: "src/**"}, edit("/work/repo/docs/x.md"), false},
		{"allow needs every path", Rule{Effect: Allow, Tool: "write_file", Path: "src/**"}, edit("/work/repo/src/a.go", "/work/repo/b.go"), false},
		{"deny needs any path", Rule{Effect: Deny, Tool: "write_file", Path: "**/*.lock"}, edit("/work/repo/a.go", "/work/repo/go.lock"), true},
		{"relative rule never matches outside root", Rule{Effect: Allow, Tool: "read_file", Path: "**"}, Request{Tool: "read_file", Paths: []string{"/etc/hosts"}}, false},
		{"absolute path", Rule{Effect: Allow, Tool: "read_file", Path: "/etc/*"}, Request{Tool: "read_file", Paths: []string{"/etc/hosts"}}, true},
		{"exact allow does not collapse whitespace", Rule{Effect: Allow, Tool: "shell", Command: "git status"}, shell("git\nstatus"), false},
		{"literal allow is not a prefix", Rule{Effect: Allow, Tool: "shell", Command: "rm -rf *", Literal: true}, shell("rm -rf ~"), false},
		{"literal allow matches itself", Rule{Effect: Allow, Tool: "shell", Command: "rm -rf *", Literal: true}, shell("rm -rf *"), true},
		{"literal path is not a glob", Rule{Effect: Allow, Tool: "write_file", Path: "**", Literal: true}, edit("/work/repo/src/x.go"), false},
		{"literal path with brackets", Rule{Effect: Allow, Tool: "write_file", Path: "app/[id]/page.tsx", Literal: true}, edit("/work/repo/app/[id]/page.tsx"), true},
		{"literal path brackets are not a class", Rule{Effect: Allow, Tool: "write_file", Path: "app/[id]/page.tsx", Literal: true}, edit("/work/repo/app/i/page.tsx"), false},
		{"deny sees through assignments", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("FOO=1 git push origin"), true},
		{"deny sees through subshells", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("(git push)"), true},
		{"deny sees through sh -c", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("sh -c 'git push'"), true},
		{"deny sees through bash -lc chains", Rule{Effect: Deny, Tool: "shell", Command: "rm *"}, shell(`bash -lc "make && rm -rf build"`), true},
		{"deny sees through a backslash", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell(`\git push`), true},
		{"deny sees through env and paths", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("env -i /usr/bin/git push"), true},
		{"deny sees through git -C", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("git -C . push origin main"), true},
		{"deny sees through rtk", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("rtk git push origin main"), true},
		{"deny sees through rtk proxy", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("rtk proxy git push origin main"), true},
		{"deny sees through git -c and --no-pager", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("git -c core.x=y --no-pager push"), true},
		{"deny sees through git --git-dir=", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("git --git-dir=.git --work-tree . push"), true},
		{"git options do not make a deny overreach", Rule{Effect: Deny, Tool: "shell", Command: "git push *"}, shell("git -C push status"), false},
		{"git options do not widen an allow", Rule{Effect: Allow, Tool: "shell", Command: "git status *"}, shell("git -C /elsewhere status"), false},
		{"command rule ignores file calls", Rule{Effect: Allow, Tool: "*", Command: "ls"}, edit("/work/repo/a"), false},
	} {
		if got := c.rule.Matches(c.req, root); got != c.want {
			t.Errorf("%s: %s on %+v = %v", c.name, c.rule, c.req, got)
		}
	}
}

func TestRuleValidate(t *testing.T) {
	for _, bad := range []Rule{
		{Effect: "maybe", Tool: "shell"},
		{Effect: Allow},
		{Effect: Allow, Tool: "shell", Command: "ls", Path: "x"},
		{Effect: Allow, Tool: "[bad"},
	} {
		if bad.Validate() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if err := (Rule{Effect: Deny, Tool: "shell", Command: "rm *"}).Validate(); err != nil {
		t.Error(err)
	}
}

func TestRuleFor(t *testing.T) {
	root := t.TempDir()
	if r, _ := RuleFor(Request{Tool: "shell", Command: " git  status "}, root, ScopeSession); r.Command != "git status" || !r.Literal || r.Scope != ScopeSession {
		t.Errorf("shell rule = %+v", r)
	}
	if r, _ := RuleFor(Request{Tool: "write_file", Paths: []string{filepath.Join(root, "src", "a.go")}}, root, ScopeGlobal); r.Path != "src/a.go" || !r.Literal {
		t.Errorf("path rule = %+v", r)
	}
	if r, _ := RuleFor(Request{Tool: "read_file", Paths: []string{"/etc/hosts"}}, root, ScopeGlobal); r.Path != "/etc/hosts" {
		t.Errorf("outside-root rule = %+v", r)
	}
	r, _ := RuleFor(Request{Tool: "write_file", Paths: []string{filepath.Join(root, "a.go")}}, root, ScopeSession)
	if !r.Matches(Request{Tool: "write_file", Paths: []string{filepath.Join(root, "a.go")}}, root) {
		t.Error("an Allow always rule does not match the call it was made from")
	}
	// Review finding: these became a rule for the whole tool.
	if _, ok := RuleFor(Request{Tool: "read_file", Kind: KindRead, Paths: []string{"/a", "/b"}}, root, ScopeGlobal); ok {
		t.Error("a rule was made for a call with two paths")
	}
	if _, ok := RuleFor(Request{Tool: "shell", Kind: KindExecute}, root, ScopeGlobal); ok {
		t.Error("a rule was made for a command it does not show")
	}
	if r, ok := RuleFor(Request{Tool: "mcp__docs__search", Kind: KindOther}, root, ScopeSession); !ok || r.Tool != "mcp__docs__search" {
		t.Errorf("tool rule = %+v, %v", r, ok)
	}
}
