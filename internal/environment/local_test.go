package environment

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type denyGuard struct{ read, write string }

func (g denyGuard) CheckRead(p string) error {
	if g.read != "" && strings.HasPrefix(p, g.read) {
		return ErrProtected
	}
	return nil
}

func (g denyGuard) CheckWrite(p string) error {
	if g.write != "" && strings.HasPrefix(p, g.write) {
		return ErrProtected
	}
	return nil
}

func newTest(t *testing.T, g Guard) *Local {
	t.Helper()
	dir := t.TempDir()
	if g == nil {
		g = denyGuard{}
	}
	l, err := NewLocal(LocalOptions{ID: "t", Root: dir, Guard: g, Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestReadWriteRelativeToRoot(t *testing.T) {
	l := newTest(t, nil)
	if err := l.FS().WriteFile("a/b/c.txt", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	b, err := l.FS().ReadFile("a/b/c.txt")
	if err != nil || string(b) != "hi" {
		t.Fatalf("ReadFile = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(l.Root(), "a", "b", "c.txt")); err != nil {
		t.Errorf("file not under root: %v", err)
	}
	entries, err := l.FS().ReadDir("a")
	if err != nil || len(entries) != 1 || entries[0].Path != "a/b" || !entries[0].IsDir {
		t.Errorf("ReadDir = %+v, %v", entries, err)
	}
}

func TestGuardIsAppliedToEveryCall(t *testing.T) {
	l := newTest(t, nil)
	l.fs.guard = denyGuard{read: filepath.Join(l.Root(), "secret"), write: filepath.Join(l.Root(), "ro")}
	os.MkdirAll(filepath.Join(l.Root(), "secret"), 0o755)
	os.WriteFile(filepath.Join(l.Root(), "secret", "k"), []byte("needle"), 0o644)
	os.WriteFile(filepath.Join(l.Root(), "open.txt"), []byte("needle"), 0o644)

	if _, err := l.FS().ReadFile("secret/k"); !errors.Is(err, ErrProtected) {
		t.Errorf("ReadFile: err = %v", err)
	}
	if err := l.FS().WriteFile("ro/x", nil); !errors.Is(err, ErrProtected) {
		t.Errorf("WriteFile: err = %v", err)
	}
	if _, err := l.FS().ReadFile("secret/../secret/k"); !errors.Is(err, ErrProtected) {
		t.Errorf("dot-dot path escaped the guard: %v", err)
	}
	matches, _ := l.FS().Grep(context.Background(), ".", "needle", "", false, 0)
	if len(matches) != 1 || matches[0].Path != "open.txt" {
		t.Errorf("Grep returned guarded files: %+v", matches)
	}
	paths, _ := l.FS().Glob(context.Background(), ".", "**/*", 0)
	for _, p := range paths {
		if strings.HasPrefix(p, "secret") {
			t.Errorf("Glob returned guarded path %s", p)
		}
	}
}

func TestGlobAndGrep(t *testing.T) {
	l := newTest(t, nil)
	for p, c := range map[string]string{
		"main.go":             "package main\nfunc Main() {}\n",
		"pkg/x/x.go":          "package x\n// TODO: fix\n",
		"pkg/x/x_test.go":     "package x\n",
		"README.md":           "todo list\n",
		".git/config":         "TODO",
		"node_modules/m/i.js": "TODO",
		"bin.dat":             "\x00\x01TODO",
	} {
		l.FS().WriteFile(p, []byte(c))
	}
	ctx := context.Background()
	got, err := l.FS().Glob(ctx, ".", "**/*.go", 0)
	if err != nil || strings.Join(got, ",") != "main.go,pkg/x/x.go,pkg/x/x_test.go" {
		t.Errorf("Glob = %v, %v", got, err)
	}
	m, err := l.FS().Grep(ctx, ".", "todo", "", true, 0)
	if err != nil || len(m) != 2 {
		t.Fatalf("Grep = %+v, %v", m, err)
	}
	m, _ = l.FS().Grep(ctx, ".", "TODO", "*.go", false, 0)
	if len(m) != 1 || m[0].Path != "pkg/x/x.go" || m[0].Line != 2 {
		t.Errorf("Grep with include = %+v", m)
	}
}

func TestMatchGlob(t *testing.T) {
	for _, c := range []struct {
		pat, name string
		want      bool
	}{
		{"*.go", "a.go", true},
		{"*.go", "x/a.go", false},
		{"**/*.go", "a.go", true},
		{"**/*.go", "x/y/a.go", true},
		{"src/**", "src/a/b", true},
		{"src/**/test_*.py", "src/test_a.py", true},
		{"src/**/test_*.py", "lib/test_a.py", false},
	} {
		if got := MatchGlob(c.pat, c.name); got != c.want {
			t.Errorf("MatchGlob(%q, %q) = %v", c.pat, c.name, got)
		}
	}
}

func TestExec(t *testing.T) {
	l := newTest(t, nil)
	ctx := context.Background()
	res, err := l.Exec(ctx, ExecSpec{Command: "pwd; echo err >&2; exit 3", Env: map[string]string{"X": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 || !strings.Contains(res.Output, l.Root()) || !strings.Contains(res.Output, "err") {
		t.Errorf("result = %+v", res)
	}
	res, _ = l.Exec(ctx, ExecSpec{Command: "echo $X", Env: map[string]string{"X": "from-env"}})
	if strings.TrimSpace(res.Output) != "from-env" {
		t.Errorf("env not passed: %q", res.Output)
	}
	res, _ = l.Exec(ctx, ExecSpec{Command: "yes | head -c 100000", MaxOutput: 1000})
	if !res.Truncated || len(res.Output) != 1000 {
		t.Errorf("output cap: truncated=%v len=%d", res.Truncated, len(res.Output))
	}
}

func TestExecTimeoutKillsProcessGroup(t *testing.T) {
	l := newTest(t, nil)
	start := time.Now()
	res, err := l.Exec(context.Background(), ExecSpec{Command: "sleep 30 & sleep 30", Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || time.Since(start) > 5*time.Second {
		t.Errorf("timeout not enforced: %+v after %v", res, time.Since(start))
	}
}

func TestExecCancel(t *testing.T) {
	l := newTest(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	_, err := l.Exec(ctx, ExecSpec{Command: "sleep 30"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestSymlinksDoNotEscapeTheGuard(t *testing.T) {
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(outside, "key"), []byte("secret"), 0o600)
	l := newTest(t, nil)
	l.fs.guard = denyGuard{read: outside, write: outside}
	os.Symlink(outside, filepath.Join(l.Root(), "link"))
	if _, err := l.FS().ReadFile("link/key"); !errors.Is(err, ErrProtected) {
		t.Errorf("read through symlink: err = %v", err)
	}
	if err := l.FS().WriteFile("link/new/file", []byte("x")); !errors.Is(err, ErrProtected) {
		t.Errorf("write through symlink: err = %v", err)
	}
}

func TestExecArgv(t *testing.T) {
	l := newTest(t, nil)
	res, err := l.Exec(context.Background(), ExecSpec{Argv: []string{"printf", "%s|%s", "a b", "$HOME"}})
	if err != nil || res.Output != "a b|$HOME" {
		t.Errorf("argv exec = %q, %v; arguments must not pass through a shell", res.Output, err)
	}
}

// Review finding: Resolve returned the link path, so the broker judged a
// symlink inside the project as an inside read.
func TestResolveFollowsSymlinks(t *testing.T) {
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(outside, "notes.txt"), []byte("private"), 0o600)
	l := newTest(t, nil)
	os.Symlink(filepath.Join(outside, "notes.txt"), filepath.Join(l.Root(), "n.txt"))
	os.Symlink(outside, filepath.Join(l.Root(), "up"))
	if got := l.Resolve("n.txt"); got != filepath.Join(outside, "notes.txt") {
		t.Errorf("Resolve(n.txt) = %s", got)
	}
	if got := l.Resolve("up/new/file.go"); got != filepath.Join(outside, "new", "file.go") {
		t.Errorf("Resolve through a linked dir = %s", got)
	}
	if l.Contains("n.txt") || l.Contains("up/x") {
		t.Error("Contains is true for a link that leads outside")
	}
	if !l.Contains("src/a.go") || !l.Contains(strings.ToUpper(l.Root())+"/A") {
		t.Error("Contains is false for paths inside the root")
	}
}

func TestReadRefusesDevicesAndHugeFiles(t *testing.T) {
	l := newTest(t, nil)
	os.Symlink("/dev/zero", filepath.Join(l.Root(), "zero"))
	if _, err := l.FS().ReadFile("zero"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("read of /dev/zero: %v", err)
	}
	big := filepath.Join(l.Root(), "big.bin")
	f, _ := os.Create(big)
	f.Truncate(MaxReadBytes + 1)
	f.Close()
	if _, err := l.FS().ReadFile("big.bin"); err == nil || !strings.Contains(err.Error(), "not read") {
		t.Errorf("read of an oversized file: %v", err)
	}
}

// Review finding: a descendant that leaves the process group kept the pipe
// open and Exec ignored its timeout and cancel.
func TestExecDoesNotHangOnEscapedDescendant(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("perl not installed")
	}
	l := newTest(t, nil)
	start := time.Now()
	_, err := l.Exec(context.Background(), ExecSpec{Command: `perl -e 'fork and exit; setsid; sleep 20' & echo hi`, Timeout: time.Second})
	if d := time.Since(start); d > 6*time.Second {
		t.Fatalf("Exec returned after %v (err %v)", d, err)
	}
}

func TestCommandsGetAMinimalEnvironment(t *testing.T) {
	t.Setenv("FREELOOP_TEST_LEAK", "leaked")
	l := newTest(t, nil)
	l.env = map[string]string{"CONFIGURED": "yes"}
	res, err := l.Exec(context.Background(), ExecSpec{Command: `echo "${FREELOOP_TEST_LEAK:-none} $CONFIGURED $PATH"`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Output, "none yes /") {
		t.Errorf("output = %q; the app's own variables must not leak", res.Output)
	}
}

func TestSpawnPipesAndEnvironment(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FREELOOP_TEST_LEAK", "secret")
	l, err := NewLocal(LocalOptions{Root: root, Guard: denyGuard{}, Shell: []string{"sh", "-c"}, Env: map[string]string{"FROM_ENV": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := l.Spawn(context.Background(), SpawnSpec{Argv: []string{"sh", "-c", `read line; echo "got $line $FROM_ENV $EXTRA ${FREELOOP_TEST_LEAK:-none}"; echo oops >&2`}, Env: map[string]string{"EXTRA": "x"}, LoginShell: true})
	if err != nil {
		t.Fatal(err)
	}
	p.Stdin().Write([]byte("hello\n"))
	out, _ := io.ReadAll(p.Stdout())
	p.Wait()
	if string(out) != "got hello 1 x none\n" || !strings.Contains(p.Stderr(), "oops") {
		t.Errorf("out = %q, stderr = %q", out, p.Stderr())
	}

	ctx, cancel := context.WithCancel(context.Background())
	p, _ = l.Spawn(ctx, SpawnSpec{Argv: []string{"sleep", "30"}})
	cancel()
	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not kill the process")
	}
}
