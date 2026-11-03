package permission

import "testing"

func TestReadOnlyCommand(t *testing.T) {
	for line, want := range map[string]bool{
		"git status": true, "git log -1": true, "git diff --stat": true, "ls -la": true, "cat a.txt": true, "grep -rn foo src": true,
		"find . -name x": true, "wc -l a.txt": true, "git status && git diff": true, `grep "foo bar" a.txt`: true,

		"git push": false, "git commit -m x": false, "git": false, "git -c core.pager=evil log": false,
		"git diff --output=x": false, "git diff --ext-diff": false, "git log --output x": false,
		"find . -delete": false, "find . -exec rm {} +": false, "rg --pre evil x": false, "tail -f log": false,
		"rm a": false, "npm test": false, "tsc": false, "./ls": false, "sh -c ls": false,
		"cat /etc/passwd": false, "cat ../secret": false, "cat a/../../secret": false, "cat ~/.ssh/id": false, "ls $HOME": false,
		"cat a; rm b": false, "cat a | sh": false, "cat a > b": false, "cat `x`": false, "cat $(x)": false, "cat a\nrm b": false,
		"cat a || rm b": false, "git status && rm x": false, "": false,

		// What the shell makes of quotes is what is judged.
		`rg --pr"e=./x.sh" foo`: false, `rg "--pre=./x.sh" foo`: false, `rg --pre'=./x.sh' foo`: false,
		`cat .""./.""./.ssh/id_rsa`: false, `cat "../secret"`: false, `"rm" a`: false, `r"m" a`: false, `cat "a`: false,
		`git "push"`: false, `git sta"tus" --out"put=x"`: false, `cat "/etc/passwd"`: false,
		`grep "a && b" f`: true, `grep 'it' "src/a b.txt"`: true, `c"a"t a.txt`: true, `grep "a" f && git "status"`: true,

		// A path as an option's value is still a path.
		"git blame --contents=/etc/passwd a": false, "rg --file=/etc/passwd x": false, "wc --files0-from=/etc/x": false,
		"grep -f/etc/passwd x": false, "grep -rf/etc/passwd x": false, "grep -f../secret x": false, "rg --file=../secret x": false,
		"git log --format=%h/%an": true, "git log main..feature/x": true, "git diff main..HEAD": true, "git diff HEAD~1": false, "git log -- src/a.go": true,

		// Git takes a unique start of a long option, so does this.
		"git diff --outp=x": false, "git log --ext-di": false, "git show --textc": false,
		"git log --oneline": true, "git diff --name-only": true, "git diff --": true,
	} {
		if got := ReadOnlyCommand(line); got != want {
			t.Errorf("ReadOnlyCommand(%q) = %v", line, got)
		}
	}
}
