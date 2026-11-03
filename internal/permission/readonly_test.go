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
	} {
		if got := ReadOnlyCommand(line); got != want {
			t.Errorf("ReadOnlyCommand(%q) = %v", line, got)
		}
	}
}
