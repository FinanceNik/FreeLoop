package environment

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// Review finding: a link whose target did not exist was judged as the link
// itself, inside the project, and the write followed it out.
func TestWriteRefusesDanglingSymlinks(t *testing.T) {
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	l := newTest(t, nil)
	os.Symlink(filepath.Join(outside, "hook"), filepath.Join(l.Root(), "notes.md"))
	os.Symlink(filepath.Join(outside, "gone", "dir"), filepath.Join(l.Root(), "folder"))
	if err := l.FS().WriteFile("notes.md", []byte("x")); err == nil {
		t.Error("wrote through a dangling symlink")
	}
	if err := l.FS().WriteFile("folder/file", []byte("x")); err == nil {
		t.Error("wrote under a dangling directory symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "hook")); err == nil {
		t.Error("the file outside the project was created")
	}
	if _, err := os.Stat(filepath.Join(outside, "gone")); err == nil {
		t.Error("a folder outside the project was created")
	}
	// A link to a file inside the project is written where it points.
	os.WriteFile(filepath.Join(l.Root(), "real.md"), []byte("old"), 0o600)
	os.Symlink("real.md", filepath.Join(l.Root(), "alias.md"))
	if err := l.FS().WriteFile("alias.md", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(l.Root(), "real.md")); string(b) != "new" {
		t.Errorf("real.md = %q", b)
	}
	if fi, _ := os.Lstat(filepath.Join(l.Root(), "alias.md")); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced by a file")
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

// Hooks (EXT-005) get the event on stdin and report a block on stderr.
func TestExecStdinAndSeparateStderr(t *testing.T) {
	l := newTest(t, nil)
	res, err := l.Exec(context.Background(), ExecSpec{Command: `read line; echo "got $line"; echo "why" >&2; exit 2`, Stdin: []byte("hello\n"), SeparateStderr: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "got hello\n" || res.Stderr != "why\n" || res.ExitCode != 2 {
		t.Errorf("res = %+v", res)
	}
	res, _ = l.Exec(context.Background(), ExecSpec{Command: `echo out; echo err >&2`})
	if res.Output != "out\nerr\n" || res.Stderr != "" {
		t.Errorf("interleaved = %+v", res)
	}
}

// A folder swapped for a link after the path was checked must not lead around
// the guard (security review, 2026-11-02).
func TestFileToolsDoNotFollowALinkAfterTheCheck(t *testing.T) {
	base, _ := filepath.EvalSymlinks(t.TempDir())
	root, secret := filepath.Join(base, "project"), filepath.Join(base, "secret")
	for _, d := range []string{filepath.Join(root, "d"), secret} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(secret, "id"), []byte("PRIVATE"), 0o600)
	os.WriteFile(filepath.Join(root, "d", "id"), []byte("public"), 0o644)
	guard := rootOnlyGuard{root}
	l, err := NewLocal(LocalOptions{Root: root, Guard: guard})
	if err != nil {
		t.Fatal(err)
	}
	swap := func() {
		afterCheck = func() {}
		os.Rename(filepath.Join(root, "d"), filepath.Join(root, "d-aside"))
		os.Symlink(secret, filepath.Join(root, "d"))
	}
	restore := func() {
		afterCheck = func() {}
		os.Remove(filepath.Join(root, "d"))
		os.Rename(filepath.Join(root, "d-aside"), filepath.Join(root, "d"))
	}
	t.Cleanup(func() { afterCheck = func() {} })

	afterCheck = swap
	b, err := l.FS().ReadFile("d/id")
	if err == nil || string(b) == "PRIVATE" {
		t.Errorf("read through a swapped folder: %q, %v", b, err)
	}
	restore()

	afterCheck = swap
	if err := l.FS().WriteFile("d/planted", []byte("x")); err == nil {
		t.Error("wrote through a swapped folder")
	}
	if _, err := os.Stat(filepath.Join(secret, "planted")); err == nil {
		t.Error("a write landed outside the project")
	}
	restore()

	// Ordinary use is unchanged, a link inside the project included.
	if err := os.Symlink(filepath.Join(root, "d", "id"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if b, err := l.FS().ReadFile("link"); err != nil || string(b) != "public" {
		t.Errorf("reading a link in the project: %q, %v", b, err)
	}
	if err := l.FS().WriteFile("link", []byte("edited")); err != nil {
		t.Errorf("writing through a link in the project: %v", err)
	}
	if err := l.FS().WriteFile("new/deep/file", []byte("x")); err != nil {
		t.Errorf("a new file in new folders: %v", err)
	}
}

func environmentCanonical(p string) string { return Canonical(p) }

type rootOnlyGuard struct{ root string }

func (g rootOnlyGuard) CheckRead(p string) error {
	if !Within(p, g.root) {
		return ErrProtected
	}
	return nil
}
func (g rootOnlyGuard) CheckWrite(p string) error { return g.CheckRead(p) }

// An environment can keep the user's SSH agent from what it runs: commands and agents
// alike. By default they get it, for git over SSH.
func TestTheSSHAgentCanBeKeptOut(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	for _, keep := range []bool{false, true} {
		l, err := NewLocal(LocalOptions{Root: t.TempDir(), Guard: denyGuard{}, Shell: []string{"sh", "-c"}, NoSSHAgent: keep})
		if err != nil {
			t.Fatal(err)
		}
		want := "/tmp/agent.sock"
		if keep {
			want = "none"
		}
		res, err := l.Exec(context.Background(), ExecSpec{Command: `echo "${SSH_AUTH_SOCK:-none}"`})
		if err != nil || strings.TrimSpace(res.Output) != want {
			t.Errorf("NoSSHAgent %v: a command sees %q, %v; want %q", keep, res.Output, err, want)
		}
		p, err := l.Spawn(context.Background(), SpawnSpec{Argv: []string{"sh", "-c", `echo "${SSH_AUTH_SOCK:-none}"`}})
		if err != nil {
			t.Fatal(err)
		}
		out, _ := io.ReadAll(p.Stdout())
		p.Wait()
		if strings.TrimSpace(string(out)) != want {
			t.Errorf("NoSSHAgent %v: an agent sees %q; want %q", keep, out, want)
		}
	}
}

// A search can leave out the files whose names say they hold secrets and say which it
// left out, so what it returns is what the user's yes to a search covered (SEC-017).
func TestGrepCanLeaveOutSecretFiles(t *testing.T) {
	l := newTest(t, nil)
	for p, c := range map[string]string{
		".env": "TOKEN=1\n", ".env.example": "TOKEN=\n", "main.go": "// TOKEN\n",
		"config/secrets.yaml": "TOKEN: 1\n", "keys/deploy.pem": "TOKEN\n", ".aws/config": "TOKEN\n",
	} {
		l.FS().WriteFile(p, []byte(c))
	}
	ctx := context.Background()
	all, err := l.FS().Grep(ctx, ".", "TOKEN", "", false, 0)
	if err != nil || len(all) != 6 {
		t.Fatalf("Grep without the option = %+v, %v", all, err)
	}
	var left []string
	got, err := l.FS().Grep(ctx, ".", "TOKEN", "", false, 0, SkipSecrets(&left))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, m := range got {
		paths = append(paths, m.Path)
	}
	sort.Strings(paths)
	if strings.Join(paths, ",") != ".env.example,main.go" {
		t.Errorf("matches = %v", paths)
	}
	sort.Strings(left)
	if strings.Join(left, ",") != ".aws/config,.env,config/secrets.yaml,keys/deploy.pem" {
		t.Errorf("left out = %v", left)
	}
	// A file that does not match the pattern is not reported as left out.
	left = nil
	l.FS().Grep(ctx, ".", "nothing-matches", "", false, 0, SkipSecrets(&left))
	if len(left) != 4 {
		t.Errorf("files left out are counted as they are met, matching or not: %v", left)
	}
	// A file's name is no secret: a listing still shows it.
	if names, _ := l.FS().Glob(ctx, ".", "**/*", 0); !strings.Contains(strings.Join(names, ","), ".env,") {
		t.Errorf("Glob = %v", names)
	}
	// The limit counts what is returned, not what was left out.
	got, _ = l.FS().Grep(ctx, ".", "TOKEN", "", false, 2, SkipSecrets(new([]string)))
	if len(got) != 2 {
		t.Errorf("with a limit of 2: %+v", got)
	}
}
