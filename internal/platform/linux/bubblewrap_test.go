//go:build linux

package linux

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// The sandbox's guarantees, tried from inside it (ENV-009, OQ-037). They need
// bubblewrap and user namespaces, which a container gives only when privileged:
//
//	make linux-test-sandbox
//
// sets FREELOOP_SANDBOX_TEST=1 and runs these there.
func sandboxed(t *testing.T) (home, project string, run func(c environment.Confinement, script string) (string, error)) {
	t.Helper()
	if os.Getenv("FREELOOP_SANDBOX_TEST") != "1" {
		t.Skip("set FREELOOP_SANDBOX_TEST=1 where bubblewrap can run (make linux-test-sandbox)")
	}
	base, _ := filepath.EvalSymlinks(t.TempDir())
	home = filepath.Join(base, "home")
	project = filepath.Join(home, "work", "project")
	for _, d := range []string{filepath.Join(home, ".ssh"), filepath.Join(project, ".git", "hooks"), filepath.Join(project, "src"), filepath.Join(home, "toolchain"), filepath.Join(home, ".cache", "proj")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(home, ".ssh", "id"), []byte("PRIVATE KEY"), 0o600)
	os.WriteFile(filepath.Join(home, "toolchain", "tool"), []byte("tool"), 0o644)
	os.WriteFile(filepath.Join(project, ".git", "config"), []byte("[core]"), 0o644)
	os.WriteFile(filepath.Join(project, "src", "main.go"), []byte("package main"), 0o644)
	run = func(c environment.Confinement, script string) (string, error) {
		argv, err := Bubblewrap{}.Confine(c, []string{"/bin/sh", "-c", script})
		if err != nil {
			return "", err
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = project
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	return home, project, run
}

func base(home, project string) environment.Confinement {
	return environment.Confinement{
		Home: home, Write: []string{project}, Scratch: []string{filepath.Join(home, ".cache", "proj")},
		Read: []string{filepath.Join(home, "toolchain")}, Network: environment.NetworkNone,
	}
}

func TestTheSandboxKeepsFilesInTheirPlace(t *testing.T) {
	home, project, run := sandboxed(t)
	c := base(home, project)
	c.Protected = []string{filepath.Join(project, ".git", "config"), filepath.Join(project, ".git", "hooks")}
	c.NoMove = []string{filepath.Join(project, ".git")}
	for _, step := range []struct {
		name, script string
		ok           bool
	}{
		{"the project is written", "echo x > src/new && cat src/new", true},
		{"the project's caches are written", "echo x > " + filepath.Join(home, ".cache", "proj", "c"), true},
		{"a listed toolchain is read", "cat " + filepath.Join(home, "toolchain", "tool"), true},
		{"git can write its objects", "mkdir -p .git/objects && echo x > .git/objects/o", true},
		{"the home folder is not read", "cat " + filepath.Join(home, ".ssh", "id"), false},
		{"the home folder is not listed", "ls " + filepath.Join(home, ".ssh"), false},
		{"nothing is written to the home folder", "echo x > " + filepath.Join(home, "planted"), false},
		{"nothing is written to the system", "echo x > /etc/planted", false},
		{"a protected file is not written", "echo x >> .git/config", false},
		{"a protected folder is not written", "echo x > .git/hooks/pre-commit", false},
		{"a folder that must stay cannot be moved", "mv .git .git-aside", false},
		{"a folder that must stay cannot be removed", "rm -rf .git", false},
	} {
		out, err := run(c, step.script)
		if (err == nil) != step.ok {
			t.Errorf("%s: err = %v, output %q", step.name, err, out)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(project, ".git", "config")); string(b) != "[core]" {
		t.Errorf(".git/config = %q", b)
	}
	if _, err := os.Stat(filepath.Join(home, "planted")); err == nil {
		t.Error("a file was planted in the home folder")
	}

	ro := c
	ro.ReadOnly = true
	if out, err := run(ro, "echo x > src/plan"); err == nil {
		t.Errorf("plan mode wrote the project: %q", out)
	}
}

func TestTheSandboxHidesWhatADenyRuleCovers(t *testing.T) {
	home, project, run := sandboxed(t)
	os.WriteFile(filepath.Join(project, ".env"), []byte("TOKEN=secret"), 0o600)
	os.MkdirAll(filepath.Join(project, "private", "keep"), 0o755)
	os.WriteFile(filepath.Join(project, "private", "keep", "skill.sh"), []byte("ok"), 0o644)
	os.WriteFile(filepath.Join(project, "private", "data"), []byte("hidden"), 0o644)
	os.WriteFile(filepath.Join(project, "src", "a.key"), []byte("k"), 0o644)
	c := base(home, project)
	c.NoAccess = []string{filepath.Join(project, ".env"), filepath.Join(project, "private")}
	c.ReadAlways = []string{filepath.Join(project, "private", "keep")}
	c.ProtectedPatterns = []string{`^` + strings.ReplaceAll(project, ".", `\.`) + `/src/[^/]*\.key$`}
	for _, step := range []struct {
		name, script string
		ok           bool
	}{
		{"a denied file is not read", "cat .env | grep -q secret", false},
		{"a denied folder is not read", "cat private/data", false},
		{"what is kept readable in it is", "cat private/keep/skill.sh", true},
		{"a wildcard's match is not written", "echo x >> src/a.key", false},
	} {
		out, err := run(c, step.script)
		if (err == nil) != step.ok {
			t.Errorf("%s: err = %v, output %q", step.name, err, out)
		}
	}
}

func TestTheSandboxIsolatesProcessesAndSockets(t *testing.T) {
	home, project, run := sandboxed(t)
	outside := exec.Command("sleep", "30")
	outside.Start()
	defer outside.Process.Kill()
	c := base(home, project)
	if out, err := run(c, "kill -0 "+strconv.Itoa(outside.Process.Pid)+" 2>/dev/null && echo seen || echo hidden"); out != "hidden" {
		t.Errorf("another program is visible: %q, %v", out, err)
	}
	// A socket outside the project, as an SSH agent's or Docker's.
	sock := filepath.Join(home, "agent.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if out, err := run(c, "test -S "+sock+" && echo reachable || echo gone"); out != "gone" {
		t.Errorf("a socket in the home folder is reachable: %q, %v", out, err)
	}
	// A socket in the project stays usable: a dev server's.
	inner := filepath.Join(project, "dev.sock")
	l2, err := net.Listen("unix", inner)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if out, err := run(c, "test -S "+inner+" && echo reachable"); out != "reachable" {
		t.Errorf("a socket in the project is gone: %q, %v", out, err)
	}
}

func TestTheSandboxsNetwork(t *testing.T) {
	home, project, run := sandboxed(t)
	for _, n := range []environment.Network{environment.NetworkNone, environment.NetworkLocalhost} {
		c := base(home, project)
		c.Network = n
		// /proc/net/dev is the namespace's own; /sys shows the host's.
		if out, _ := run(c, "awk 'NR>2{print $1}' /proc/net/dev"); out != "lo:" {
			t.Errorf("%s: interfaces %q, want only lo", n, out)
		}
	}
	c := base(home, project)
	c.Network = environment.NetworkAll
	if out, _ := run(c, "awk 'NR>2{print $1}' /proc/net/dev"); out == "lo:" {
		t.Errorf("all: only lo")
	}
}

func TestASandboxNeedsAProject(t *testing.T) {
	if _, err := BubblewrapArgs(environment.Confinement{Home: "/home/x"}); err == nil {
		t.Error("a sandbox without a project was made")
	}
}
