package environment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// dockerOrSkip returns a project folder Docker can bind-mount, or skips
// when no Docker daemon runs here (as on CI).
func dockerOrSkip(t *testing.T) string {
	t.Helper()
	docker, err := FindDocker()
	if err != nil {
		t.Skip(err)
	}
	host, _ := NewLocal(LocalOptions{Root: t.TempDir(), Guard: denyGuard{}})
	if res, err := host.Exec(context.Background(), ExecSpec{Argv: []string{docker, "info", "--format", "{{.ServerVersion}}"}, Timeout: 20 * time.Second}); err != nil || res.ExitCode != 0 {
		t.Skip("no Docker daemon is running")
	}
	// Colima and Docker Desktop share the home folder by default, not
	// the system temp folder.
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skip(err)
	}
	dir, err := os.MkdirTemp(cache, "freeloop-docker-test-")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// ENV-005, ENV-007, OQ-014: commands and agents run in the container, the
// project is shared at the same path, secrets reach commands without being
// on a command line, setup and health run, and the host stays out of sight.
func TestDockerEnvironment(t *testing.T) {
	root := dockerOrSkip(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello from the host\n"), 0o644)
	var b [3]byte
	rand.Read(b[:])
	d, err := NewDocker(DockerOptions{
		ID: "docker", Container: "freeloop-test-" + hex.EncodeToString(b[:]), Root: root, Image: "alpine:3.20",
		Env: map[string]string{"SECRET": "s3cr3t-value"}, Setup: "touch /tmp/setup-done", Health: "test -f /tmp/setup-done", Guard: denyGuard{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Remove(ctx)
	did, err := d.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(did, "created the container") || !strings.Contains(did, "setup") {
		t.Errorf("start did %q", did)
	}

	res, err := d.Exec(ctx, ExecSpec{Command: "uname -s; cat a.txt; echo $SECRET; pwd"})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("exec = %+v, %v", res, err)
	}
	want := "Linux\nhello from the host\ns3cr3t-value\n" + d.Root() + "\n"
	if res.Output != want {
		t.Errorf("output = %q, want %q", res.Output, want)
	}
	args, _ := d.execArgs(false, "", nil, "x")
	if slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "s3cr3t") }) {
		t.Errorf("a secret value is on the docker command line: %v", args)
	}

	// Files written through the environment are the container's files.
	if err := d.FS().WriteFile(filepath.Join(d.Root(), "b.txt"), []byte("from the agent\n")); err != nil {
		t.Fatal(err)
	}
	if res, _ := d.Exec(ctx, ExecSpec{Command: "cat b.txt"}); res.Output != "from the agent\n" {
		t.Errorf("b.txt in the container = %q", res.Output)
	}
	// The rest of the host is not there: a file next to the project is
	// invisible (the mount only creates the empty folders above it).
	sibling := root + "-sibling.txt"
	os.WriteFile(sibling, []byte("x"), 0o600)
	defer os.Remove(sibling)
	if res, _ := d.Exec(ctx, ExecSpec{Command: "test -e " + sibling + " && echo visible || echo hidden"}); strings.TrimSpace(res.Output) != "hidden" {
		t.Errorf("a host file next to the project is %s in the container", res.Output)
	}

	// The deadline holds inside the container.
	start := time.Now()
	res, _ = d.Exec(ctx, ExecSpec{Command: "sleep 30", Timeout: 2 * time.Second})
	if time.Since(start) > 15*time.Second || res.ExitCode == 0 {
		t.Errorf("sleep 30 with a 2 s timeout took %s, exit %d", time.Since(start), res.ExitCode)
	}

	// Agents talk over stdio from inside the container (ENV-006).
	p, err := d.Spawn(ctx, SpawnSpec{Argv: []string{"sh", "-c", `read line; echo "got $line $SECRET"`}})
	if err != nil {
		t.Fatal(err)
	}
	p.Stdin().Write([]byte("ping\n"))
	out, _ := io.ReadAll(p.Stdout())
	p.Wait()
	if string(out) != "got ping s3cr3t-value\n" {
		t.Errorf("spawned process said %q", out)
	}

	// A stopped container starts again without running setup twice.
	if err := d.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if did, err := d.Start(ctx); err != nil || did != "started the container" {
		t.Errorf("restart did %q, %v", did, err)
	}
}

// A Dockerfile or build context outside the project is refused, symlinks
// resolved, before anything runs.
func TestDockerBuildPathsStayInside(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "ctx"))
	for _, o := range []DockerOptions{
		{Dockerfile: filepath.Join(outside, "Dockerfile"), BuildContext: root},
		{Dockerfile: filepath.Join(root, "Dockerfile"), BuildContext: filepath.Join(root, "ctx")},
	} {
		o.Container, o.Root, o.Guard = "c", root, denyGuard{}
		os.WriteFile(filepath.Join(root, "Dockerfile"), nil, 0o644)
		os.WriteFile(filepath.Join(outside, "Dockerfile"), nil, 0o644)
		if _, err := NewDocker(o); err == nil || !strings.Contains(err.Error(), "outside the project") {
			t.Errorf("%+v: err = %v", o, err)
		}
	}
	if hostSensitive("SECRET") || !hostSensitive("DOCKER_HOST") || !hostSensitive("DYLD_INSERT_LIBRARIES") || !hostSensitive("PATH") {
		t.Error("hostSensitive")
	}
}

// What the app reads and runs on the host is read-only in the container,
// and a project file's variables reach commands without reaching the host
// docker CLI's environment.
func TestDockerProtectsHostConfig(t *testing.T) {
	root := dockerOrSkip(t)
	ctx := context.Background()
	var b [3]byte
	rand.Read(b[:])
	d, err := NewDocker(DockerOptions{
		ID: "docker", Container: "freeloop-test-" + hex.EncodeToString(b[:]), Root: root, Image: "alpine:3.20", Guard: denyGuard{},
		ReadOnly: []string{filepath.Join(root, ".freeloop") + "/"},
		PlainEnv: map[string]string{"FROM_PROJECT": "plain", "DOCKER_HOST": "tcp://evil:1"},
		Env:      map[string]string{"SECRET": "s3cr3t"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Remove(ctx)
	if _, err := d.Start(ctx); err != nil {
		t.Fatal(err)
	}
	res, _ := d.Exec(ctx, ExecSpec{Command: "touch .freeloop/hooks.yaml 2>&1; echo \"$FROM_PROJECT $SECRET\""})
	if !strings.Contains(res.Output, "Read-only") || !strings.Contains(res.Output, "plain s3cr3t") {
		t.Errorf("output = %q", res.Output)
	}
	if _, err := os.Stat(filepath.Join(root, ".freeloop", "hooks.yaml")); err == nil {
		t.Error("the container wrote .freeloop/hooks.yaml")
	}
	args, env := d.execArgs(false, "", nil, "x")
	if _, ok := env["DOCKER_HOST"]; ok || !slices.Contains(args, "DOCKER_HOST=tcp://evil:1") || !slices.Contains(args, "SECRET") {
		t.Errorf("args = %v, CLI env = %v", args, env)
	}
}

// Stopping a command ends it inside the container too, children included,
// not just the docker CLI on the host.
func TestDockerCancelEndsTheCommand(t *testing.T) {
	root := dockerOrSkip(t)
	ctx := context.Background()
	var b [3]byte
	rand.Read(b[:])
	d, err := NewDocker(DockerOptions{ID: "docker", Container: "freeloop-test-" + hex.EncodeToString(b[:]), Root: root, Image: "alpine:3.20", Guard: denyGuard{}})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Remove(ctx)
	if _, err := d.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	d.Exec(cctx, ExecSpec{Command: "sleep 301 & sleep 302; wait", Timeout: time.Minute})
	res, _ := d.Exec(ctx, ExecSpec{Command: `for p in /proc/[0-9]*; do tr '\000' ' ' < $p/cmdline 2>/dev/null; echo; done`})
	if strings.Contains(res.Output, "sleep 30") {
		t.Errorf("the command still runs in the container:\n%s", res.Output)
	}
}

// Review finding: docker follows a symlink in a mount's source, so a cloned
// repository's .freeloop link could mount a folder of the Mac.
func TestSymlinkOnPath(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "real", "hooks"), 0o755)
	os.Symlink(filepath.Join(root, "real"), filepath.Join(root, ".git"))
	os.Symlink("../../.ssh", filepath.Join(root, ".freeloop"))
	bases := []string{root}
	if got := symlinkOnPath(filepath.Join(root, ".freeloop"), bases); got != filepath.Join(root, ".freeloop") {
		t.Errorf(".freeloop: %q", got)
	}
	if got := symlinkOnPath(filepath.Join(root, ".git", "hooks"), bases); got != filepath.Join(root, ".git") {
		t.Errorf(".git/hooks: %q", got)
	}
	for _, p := range []string{filepath.Join(root, "real", "hooks"), filepath.Join(root, "missing", "x"), "/elsewhere/x"} {
		if got := symlinkOnPath(p, bases); got != "" {
			t.Errorf("%s: %q", p, got)
		}
	}
}
