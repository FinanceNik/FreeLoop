package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

func TestParseStatus(t *testing.T) {
	st := parseStatus("# branch.oid 1234abcd\n# branch.head feature/x\n# branch.upstream origin/feature/x\n1 .M N... 100644 100644 100644 a b go.mod\n? new.txt\n")
	if !st.Repo || st.Head != "1234abcd" || st.Branch != "feature/x" || st.Dirty != 2 {
		t.Errorf("status = %+v", st)
	}
	if st := parseStatus("# branch.oid (initial)\n# branch.head (detached)\n"); st.Head != "" || st.Branch != "" || st.Dirty != 0 {
		t.Errorf("initial/detached = %+v", st)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	run("add", ".")
	run("commit", "-q", "-m", "first")
	run("remote", "add", "origin", "https://example.com/r.git")
	return dir
}

func TestGitAgainstARealRepo(t *testing.T) {
	dir := gitRepo(t)
	home := t.TempDir()
	env, err := environment.NewLocal(environment.LocalOptions{Root: dir, Guard: permission.NewPolicy(home, filepath.Join(home, "d")).Guard(dir)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st := gitState(ctx, env)
	if !st.Repo || st.Branch != "main" || len(st.Head) != 40 || st.Dirty != 0 {
		t.Errorf("clean repo = %+v", st)
	}
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
	if st := gitState(ctx, env); st.Dirty != 1 {
		t.Errorf("dirty = %d, want 1", st.Dirty)
	}
	first, remote := fingerprint(ctx, env)
	if first != st.Head || remote != "https://example.com/r.git" {
		t.Errorf("fingerprint = %q, %q", first, remote)
	}

	plain := t.TempDir()
	penv, _ := environment.NewLocal(environment.LocalOptions{Root: plain, Guard: permission.NewPolicy(home, filepath.Join(home, "d")).Guard(plain)})
	if st := gitState(ctx, penv); st.Repo {
		t.Errorf("not a repo = %+v", st)
	}
}
