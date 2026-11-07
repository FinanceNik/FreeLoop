package permission

import (
	"os"
	"path/filepath"
	"testing"
)

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

// HRN-019, SEC-005, SEC-017: judged against the project's files, a rewritten command
// is read-only only when what it names stays in the project, unprotected and not
// secret, after following links; and a search that follows links is never.
func TestReadOnlyCommandFollowsTheFiles(t *testing.T) {
	_, _, home, root := setup(t)
	p := NewPolicy(home, filepath.Join(home, "Library", "Application Support", "freeloop"))
	elsewhere := filepath.Join(home, "elsewhere")
	for _, d := range []string{root, elsewhere, filepath.Join(home, ".ssh"), filepath.Join(root, "src")} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("key"), 0o600)
	os.WriteFile(filepath.Join(elsewhere, "notes.txt"), []byte("notes"), 0o644)
	os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=x"), 0o644)
	os.WriteFile(filepath.Join(root, ".env.example"), []byte("TOKEN="), 0o644)
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644)
	os.Symlink(filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(root, "key"))
	os.Symlink(filepath.Join(elsewhere, "notes.txt"), filepath.Join(root, "notes"))
	os.Symlink(elsewhere, filepath.Join(root, "linked"))
	for line, want := range map[string]bool{
		"cat main.go":               true,
		"head -n 5 .env.example":    true,
		"rg TODO src":               true,
		"grep -rn TODO src":         true,
		"git log -n 3":              true,
		"cat key":                   false, // a link into ~/.ssh
		"cat notes":                 false, // a link out of the project
		"ls linked":                 false, // a linked folder out of the project
		"cat .env":                  false, // secret: reading it asks
		"wc -l main.go && cat .env": false,
		"grep -R TODO .":            false, // follows the links it meets
		"grep -Rn TODO src":         false,
		"rg -L TODO":                false,
		"rg --follow TODO":          false,
		"find -L . -name x":         false,
		"git diff --stat":           true,
	} {
		if got := p.ReadOnlyCommand(root, line); got != want {
			t.Errorf("%q: read-only %v, want %v", line, got, want)
		}
	}
}
