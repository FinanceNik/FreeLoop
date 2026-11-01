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
