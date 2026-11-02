//go:build darwin

package darwin

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

type fixture struct {
	home, root, outside, scratch string
	env                          *environment.Local
}

type openGuard struct{}

func (openGuard) CheckRead(string) error  { return nil }
func (openGuard) CheckWrite(string) error { return nil }

// sandboxed builds a project in a stand-in home, with a Confinement as the
// engine builds one.
func sandboxed(t *testing.T, change func(*environment.Confinement)) fixture {
	t.Helper()
	if _, err := os.Stat(sandboxExec); err != nil {
		t.Skip(err)
	}
	base := environment.Canonical(t.TempDir())
	f := fixture{home: filepath.Join(base, "home"), scratch: filepath.Join(base, "scratch")}
	f.root = filepath.Join(f.home, "Documents", "project")
	f.outside = filepath.Join(f.home, "outside")
	for _, d := range []string{f.root, f.outside, f.scratch, filepath.Join(f.home, ".ssh"), filepath.Join(f.home, "toolchain"), filepath.Join(f.home, "Documents", "taxes")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(f.home, ".ssh", "id"), []byte("private"), 0o600)
	os.WriteFile(filepath.Join(f.home, "toolchain", "tool"), []byte("tool"), 0o644)
	os.WriteFile(filepath.Join(f.home, "Documents", "taxes", "2026"), []byte("taxes"), 0o644)
	os.WriteFile(filepath.Join(f.outside, "victim"), []byte("victim"), 0o644)
	if out, err := exec.Command("git", "init", "-q", f.root).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	c := environment.Confinement{
		Home: f.home, Write: []string{f.root}, Scratch: []string{f.scratch},
		Protected: []string{
			filepath.Join(f.root, ".git", "config"), filepath.Join(f.root, ".git", "hooks"), filepath.Join(f.root, ".git", "commondir"),
			filepath.Join(f.root, ".freeloop"), filepath.Join(f.root, ".claude"),
		},
		Read:     []string{filepath.Join(f.home, "toolchain")},
		NoAccess: []string{filepath.Join(f.home, ".ssh")},
		Network:  environment.NetworkLocalhost,
	}
	if change != nil {
		change(&c)
	}
	env, err := environment.NewLocal(environment.LocalOptions{
		Root: f.root, Guard: openGuard{}, Shell: []string{"/bin/sh", "-c"},
		Env:     map[string]string{"HOME": f.home, "TMPDIR": f.scratch},
		Sandbox: &environment.SandboxOptions{Confiner: Seatbelt{}, Base: c, Probe: filepath.Join(f.outside, "probe")},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.env = env
	return f
}

func (f fixture) run(t *testing.T, command string) (string, int) {
	t.Helper()
	res, err := f.env.Exec(context.Background(), environment.ExecSpec{Command: command, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return res.Output, res.ExitCode
}

// ENV-009: what a confined command can do.
func TestSeatbeltAllows(t *testing.T) {
	f := sandboxed(t, nil)
	if err := f.env.CheckSandbox(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.outside, "probe")); err == nil {
		t.Error("the self-test left its probe")
	}
	for _, command := range []string{
		"echo x > made.txt && test -s made.txt",
		"mkdir -p a/b && echo x > a/b/c && rm a/b/c",
		"echo x > \"$TMPDIR/t\"",
		"echo x > /private/tmp/.freeloop-sandbox-test-$$ 2>/dev/null; true",
		"git status --short",
		"git add -A && git -c user.name=t -c user.email=t@t commit -qm one && git log --oneline | wc -l",
		"cat \"$HOME/toolchain/tool\"",
		"pwd",
		"sh -c 'exit 0'",
		"(sleep 5 & p=$!; kill $p) && echo killed its own child",
	} {
		if out, code := f.run(t, command); code != 0 {
			t.Errorf("%s: exit %d: %s", command, code, out)
		}
	}
	if _, code := f.run(t, "sh -c 'exit 7'"); code != 7 {
		t.Errorf("exit code = %d", code)
	}
}

// ENV-009: what it cannot. Each of these worked before the profile had the rule
// that stops it, or is a way out someone would try.
func TestSeatbeltRefuses(t *testing.T) {
	f := sandboxed(t, nil)
	victim := filepath.Join(f.outside, "victim")
	for _, command := range []string{
		"touch " + filepath.Join(f.outside, "w"),
		"echo x >> " + victim,
		"touch " + filepath.Join(f.home, "w"),
		"touch /usr/local/.freeloop-sandbox-test",
		"echo x >> .git/config",
		"echo x >> .GIT/CONFIG",
		"git config core.fsmonitor x",
		"touch .git/hooks/pre-commit",
		"echo x > .git/commondir",
		"mkdir .freeloop",
		"mkdir .FREELOOP",
		"mkdir .claude",
		"ln " + victim + " hard.txt && echo x >> hard.txt",
		"ln -s " + victim + " soft.txt && echo x >> soft.txt",
		"ln -s " + f.outside + " dir && touch dir/w",
		"cat " + filepath.Join(f.home, ".ssh", "id"),
		"cat " + filepath.Join(f.home, ".SSH", "ID"),
		"ln -s " + filepath.Join(f.home, ".ssh") + " keys && cat keys/id",
		"cat " + filepath.Join(f.home, "Documents", "taxes", "2026"),
		"ls " + filepath.Join(f.home, "Documents"),
		"ls " + f.home + "/../home/outside",
		"curl -sS -m 5 -o /dev/null https://example.com",
		"python3 -I -c 'import socket; socket.gethostbyname(\"example.com\")'",
		"python3 -I -c 'import socket; socket.socket().connect((\"1.1.1.1\", 53))'",
		"ssh-add -l",
		"open -g -b com.apple.calculator",
		"osascript -e 'tell application \"Finder\" to activate'",
		"pbpaste",
		"launchctl submit -l com.freeloop.sandboxtest -- /usr/bin/touch " + filepath.Join(f.outside, "launchd"),
		"security find-generic-password -s freeloop-test -w",
		"sandbox-exec -p '(version 1)(allow default)' touch " + filepath.Join(f.outside, "nested"),
	} {
		if out, code := f.run(t, command); code == 0 {
			t.Errorf("%s: was allowed: %s", command, out)
		}
	}
	if b, _ := os.ReadFile(victim); string(b) != "victim" {
		t.Errorf("the outside file became %q", b)
	}
	exec.Command("launchctl", "remove", "com.freeloop.sandboxtest").Run()
	entries, _ := os.ReadDir(f.outside)
	if len(entries) != 1 {
		t.Errorf("the outside folder holds %v", entries)
	}
	if out, err := exec.Command("git", "-C", f.root, "config", "--get", "core.fsmonitor").Output(); err == nil {
		t.Errorf("core.fsmonitor was set to %s", out)
	}
}

// Other programs are out of reach: their processes, and the command lines where
// keys sometimes are. Unconfined, the same commands see and signal them.
func TestSeatbeltOtherProcesses(t *testing.T) {
	f := sandboxed(t, nil)
	other := exec.Command("/bin/sh", "-c", "sleep 30; : sk-not-for-the-sandbox")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		other.Process.Kill()
		other.Wait()
	}()
	pid := other.Process.Pid
	list := fmt.Sprintf("ps -ww -o args= -p %d; ps -ww -ax -o args=", pid)
	if out, err := exec.Command("/bin/sh", "-c", list).CombinedOutput(); err != nil || !strings.Contains(string(out), "sk-not-for-the-sandbox") {
		t.Fatalf("the control cannot see the command line: %v: %.200s", err, out)
	}
	if out, code := f.run(t, fmt.Sprintf("kill -0 %d", pid)); code == 0 {
		t.Errorf("signalled another process: %s", out)
	}
	if out, _ := f.run(t, list); strings.Contains(out, "sk-not-for-the-sandbox") {
		t.Errorf("read another process's command line: %.200s", out)
	}
}

// OQ-028: localhost by default, and nothing with NetworkNone.
func TestSeatbeltLocalhost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	connect := fmt.Sprintf(`python3 -I -c 'import socket; socket.create_connection(("127.0.0.1", %d), 5)'`, ln.Addr().(*net.TCPAddr).Port)
	serve := `python3 -I -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); s.listen(1); c=socket.create_connection(s.getsockname(), 5)'`
	unix := `python3 -I -c 'import socket,os; p=os.environ["TMPDIR"]+"/s"; s=socket.socket(socket.AF_UNIX); s.bind(p); s.listen(1); socket.socket(socket.AF_UNIX).connect(p); os.unlink(p)'`

	f := sandboxed(t, nil)
	for _, command := range []string{connect, serve, unix} {
		if out, code := f.run(t, command); code != 0 {
			t.Errorf("%s: exit %d: %s", command, code, out)
		}
	}
	f = sandboxed(t, func(c *environment.Confinement) { c.Network = environment.NetworkNone })
	for _, command := range []string{connect, serve} {
		if out, code := f.run(t, command); code == 0 {
			t.Errorf("%s: allowed with no network: %s", command, out)
		}
	}
	if out, code := f.run(t, unix); code != 0 {
		t.Errorf("a program's own Unix socket: exit %d: %s", code, out)
	}
}

// Plan mode: it can look at the project, and writes only temp files.
func TestSeatbeltReadOnly(t *testing.T) {
	f := sandboxed(t, nil)
	run := func(command string) (string, int) {
		res, err := f.env.Exec(context.Background(), environment.ExecSpec{Command: command, ReadOnly: true, Timeout: 30 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		return res.Output, res.ExitCode
	}
	if out, code := run("git status --short; echo x > \"$TMPDIR/t\"; ls -a"); code != 0 || !strings.Contains(out, ".git") {
		t.Errorf("reading: exit %d: %s", code, out)
	}
	for _, command := range []string{"touch made.txt", "git add -A", "rm -rf .git"} {
		if out, code := run(command); code == 0 {
			t.Errorf("%s: was allowed: %s", command, out)
		}
	}
	if _, err := os.Stat(filepath.Join(f.root, ".git", "HEAD")); err != nil {
		t.Error(err)
	}
}

// A file the skill store holds stays readable although the data folder is not.
func TestSeatbeltReadAlways(t *testing.T) {
	f := sandboxed(t, nil)
	data := filepath.Join(f.home, "data")
	os.MkdirAll(filepath.Join(data, "skills", "x"), 0o755)
	os.WriteFile(filepath.Join(data, "skills", "x", "run.sh"), []byte("ok"), 0o644)
	os.WriteFile(filepath.Join(data, "config.yaml"), []byte("private"), 0o644)
	c := environment.Confinement{Home: f.home, Write: []string{f.root}, NoAccess: []string{data}, ReadAlways: []string{filepath.Join(data, "skills")}}
	env, err := environment.NewLocal(environment.LocalOptions{Root: f.root, Guard: openGuard{}, Shell: []string{"/bin/sh", "-c"},
		Sandbox: &environment.SandboxOptions{Confiner: Seatbelt{}, Base: c}})
	if err != nil {
		t.Fatal(err)
	}
	res, _ := env.Exec(context.Background(), environment.ExecSpec{Command: "cat " + filepath.Join(data, "skills", "x", "run.sh")})
	if res.ExitCode != 0 || res.Output != "ok" {
		t.Errorf("the skill: exit %d: %s", res.ExitCode, res.Output)
	}
	res, _ = env.Exec(context.Background(), environment.ExecSpec{Command: "cat " + filepath.Join(data, "config.yaml")})
	if res.ExitCode == 0 {
		t.Error("the data folder was readable")
	}
}

// A wildcard deny rule holds for the shell too.
func TestSeatbeltPatterns(t *testing.T) {
	f := sandboxed(t, nil)
	os.WriteFile(filepath.Join(f.root, "secret.env"), []byte("hidden"), 0o644)
	os.WriteFile(filepath.Join(f.root, "open.txt"), []byte("open"), 0o644)
	c := environment.Confinement{Home: f.home, Write: []string{f.root}, NoAccessPatterns: []string{"^" + f.root + `/[^/]*\.env$`}}
	env, _ := environment.NewLocal(environment.LocalOptions{Root: f.root, Guard: openGuard{}, Shell: []string{"/bin/sh", "-c"},
		Sandbox: &environment.SandboxOptions{Confiner: Seatbelt{}, Base: c}})
	if res, _ := env.Exec(context.Background(), environment.ExecSpec{Command: "cat secret.env"}); res.ExitCode == 0 {
		t.Error("a file a deny rule covers was read")
	}
	if res, _ := env.Exec(context.Background(), environment.ExecSpec{Command: "cat open.txt"}); res.ExitCode != 0 {
		t.Errorf("another file: %s", res.Output)
	}
}

// Protecting .git/config means nothing if .git can be moved aside, edited and
// moved back (found by a security review, 2026-11-02).
func TestSeatbeltNoMove(t *testing.T) {
	var gitDir string
	f := sandboxed(t, func(c *environment.Confinement) {
		gitDir = filepath.Join(c.Write[0], ".git")
		c.NoMove = []string{gitDir}
	})
	for _, command := range []string{
		"mv .git .g2",
		"mv .git/config .git/config2",
		"python3 -I -c 'import os; os.rename(\".git\", \".g2\")'",
	} {
		if out, code := f.run(t, command); code == 0 {
			t.Errorf("%s succeeded: %s", command, out)
		}
	}
	if out, code := f.run(t, `echo ref > .git/ORIG_HEAD && mkdir -p sub && mv sub sub2`); code != 0 {
		t.Errorf("ordinary writes were refused: %s", out)
	}
	if b, err := os.ReadFile(filepath.Join(gitDir, "HEAD")); err != nil || len(b) == 0 {
		t.Errorf(".git was changed: %v", err)
	}
	if !strings.Contains(mustProfile(t, environment.Confinement{Home: "/h", Write: []string{"/h/p"}, NoMovePatterns: []string{"^/h/p/x$"}}), `(deny file-write-unlink (regex #"^/h/p/x$"))`) {
		t.Error("a pattern was left out")
	}
}

// A Unix socket is a way into a program outside the sandbox, whatever the
// network setting (found by a security review, 2026-11-02).
func TestSeatbeltUnixSockets(t *testing.T) {
	// Outside the stand-in home and project, as the app's and Docker's are.
	dir, err := os.MkdirTemp("/private/tmp", "flsock")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Write([]byte("hello"))
			c.Close()
		}
	}()
	connect := `python3 -I -c 'import socket,sys; s=socket.socket(socket.AF_UNIX); s.connect(sys.argv[1]); print(s.recv(5).decode())' `
	// A relative path, since a stand-in home's is longer than a socket's may be.
	own := "import socket; s=socket.socket(socket.AF_UNIX); s.bind(\"o\"); s.listen(); c=socket.socket(socket.AF_UNIX); c.connect(\"o\"); print(\"own ok\")"
	for _, network := range []environment.Network{environment.NetworkAll, environment.NetworkLocalhost, environment.NetworkNone} {
		t.Run(string(network), func(t *testing.T) {
			f := sandboxed(t, func(c *environment.Confinement) {
				c.Scratch = append(c.Scratch, "/private/tmp")
			})
			exec := func(command string) (string, int) {
				res, err := f.env.Exec(context.Background(), environment.ExecSpec{Command: command, Network: network, Timeout: 20 * time.Second})
				if err != nil {
					t.Fatal(err)
				}
				return res.Output, res.ExitCode
			}
			if out, code := exec(connect + sock); code == 0 {
				t.Errorf("reached a socket in /private/tmp: %s", out)
			}
			if out, code := exec(`python3 -I -c '` + own + `'`); code != 0 || !strings.Contains(out, "own ok") {
				t.Errorf("a program's own socket failed: %d: %s", code, out)
			}
		})
	}
}

func mustProfile(t *testing.T, c environment.Confinement) string {
	t.Helper()
	p, err := SeatbeltProfile(c)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSeatbeltFailsClosed(t *testing.T) {
	if _, err := SeatbeltProfile(environment.Confinement{}); err == nil {
		t.Error("an empty confinement gave a profile")
	}
	c := environment.Confinement{Home: "/h", Write: []string{"/h/p"}, NoAccessPatterns: []string{`a"b`}}
	if _, err := SeatbeltProfile(c); err == nil {
		t.Error("a pattern with a quote gave a profile")
	}
	if got := quote(`a"b\c`); got != `"a\"b\\c"` {
		t.Errorf("quote = %s", got)
	}
}

// The user's own commands get the network, and the file rules still hold. It
// needs the internet, so it runs only when asked:
// FREELOOP_SANDBOX_NET=1 go test ./internal/platform/darwin -run NetworkAll
func TestSeatbeltNetworkAll(t *testing.T) {
	if os.Getenv("FREELOOP_SANDBOX_NET") == "" {
		t.Skip("set FREELOOP_SANDBOX_NET=1; this reaches the internet")
	}
	f := sandboxed(t, nil)
	for _, command := range []string{
		"curl -sS -m 20 -o /dev/null https://registry.npmjs.org/react/latest",
		`python3 -I -c 'import socket; print(socket.gethostbyname("example.com"))'`,
		"npm_config_cache=$TMPDIR/npm npm view react version",
	} {
		res, err := f.env.Exec(context.Background(), environment.ExecSpec{Command: command, Network: environment.NetworkAll, Timeout: 60 * time.Second})
		if err != nil || res.ExitCode != 0 {
			t.Errorf("%s: exit %d: %v: %s", command, res.ExitCode, err, res.Output)
		}
	}
	res, _ := f.env.Exec(context.Background(), environment.ExecSpec{Command: "touch " + filepath.Join(f.outside, "w"), Network: environment.NetworkAll})
	if res.ExitCode == 0 {
		t.Error("the network setting opened the file rules")
	}
	res, _ = f.env.Exec(context.Background(), environment.ExecSpec{Command: "pbpaste", Network: environment.NetworkAll})
	if res.ExitCode == 0 {
		t.Error("the network setting opened the clipboard")
	}
}

// ENV-010, on the real sandbox and internet: the usual tools reach a listed host
// through the proxy, and nothing reaches any other, with or without the proxy.
// FREELOOP_SANDBOX_NET=1 go test ./internal/platform/darwin -run Hosts
func TestSeatbeltHosts(t *testing.T) {
	if os.Getenv("FREELOOP_SANDBOX_NET") == "" {
		t.Skip("set FREELOOP_SANDBOX_NET=1; this reaches the internet")
	}
	var blocked []string
	egress, err := netguard.StartEgress([]string{"registry.npmjs.org", "github.com", "proxy.golang.org", "sum.golang.org", "pypi.org"}, func(h string) { blocked = append(blocked, h) })
	if err != nil {
		t.Fatal(err)
	}
	defer egress.Close()
	f := sandboxed(t, func(c *environment.Confinement) { c.Certificates = true })
	env := environment.ProxyEnv(egress.URL())
	run := func(command string) (string, int) {
		res, err := f.env.Exec(context.Background(), environment.ExecSpec{Command: command, Env: env, Timeout: 90 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		return res.Output, res.ExitCode
	}
	for _, command := range []string{
		"curl -sS -m 30 -o /dev/null https://registry.npmjs.org/react/latest",
		"npm_config_cache=$TMPDIR/npm npm view react version",
		`python3 -I -c 'import urllib.request as u; print(u.urlopen("https://pypi.org/simple/pip/", timeout=30).status)'`,
		"git ls-remote https://github.com/git/git.git HEAD",
		`node -e 'fetch("https://registry.npmjs.org/react/latest").then(r=>{if(!r.ok)process.exit(1);console.log("ok")}).catch(e=>{console.error(e);process.exit(1)})'`,
		"GOFLAGS='-mod=mod -modcacherw' GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/gc go mod download -json golang.org/x/text@v0.3.0",
	} {
		if out, code := run(command); code != 0 {
			t.Errorf("%s: exit %d: %.300s", command, code, out)
		}
	}
	for _, command := range []string{
		"curl -sS -m 15 -o /dev/null https://example.com",
		"curl -sS -m 15 --noproxy '*' -o /dev/null https://registry.npmjs.org/react/latest",
		`env -u HTTPS_PROXY -u https_proxy -u ALL_PROXY -u all_proxy curl -sS -m 15 -o /dev/null https://registry.npmjs.org/react/latest`,
		`python3 -I -c 'import socket; socket.create_connection(("registry.npmjs.org", 443), 10)'`,
	} {
		if out, code := run(command); code == 0 {
			t.Errorf("%s: was allowed: %s", command, out)
		}
	}
	if len(blocked) != 1 || blocked[0] != "example.com" {
		t.Errorf("blocked = %v", blocked)
	}
}

// Opening a project's file must not run it (security review, 2026-11-02).
func TestOpenInEditorNeverRuns(t *testing.T) {
	argv := EditorArgv("/p/README.md")
	if argv[0] != "/usr/bin/open" || argv[1] != "-t" || argv[2] != "/p/README.md" || len(argv) != 3 {
		t.Errorf("argv = %v", argv)
	}
}
