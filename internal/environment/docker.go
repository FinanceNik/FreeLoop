package environment

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// KindDocker is a container on this machine (ENV-005).
const KindDocker Kind = "docker"

// DockerOptions configures a Docker environment.
type DockerOptions struct {
	ID string
	// Container names the session's container; it is created on Start
	// unless it exists.
	Container string
	// Root is the project folder on the host. It is bind-mounted at the
	// same path inside the container, so paths mean the same on both sides.
	Root string
	// Image runs the container; Dockerfile and BuildContext, when set,
	// build it first (from a devcontainer.json "build").
	Image        string
	Dockerfile   string
	BuildContext string
	// Env reaches every command and agent; values are passed through the
	// docker CLI's own environment, never on its command line.
	Env map[string]string
	// Setup runs once after the container is created; Health before every
	// session start (ENV-007).
	Setup  string
	Health string
	Guard  Guard
}

// Docker runs commands and agents in a container and reads and writes the
// bind-mounted project on the host, under the same path policy as a local
// environment. Inside the container only the project and the image are
// visible: an approved command cannot reach the rest of the user's files.
type Docker struct {
	opts   DockerOptions
	docker string
	host   *Local // the host side: files, and the docker CLI's processes
}

// dockerDirs are where the docker CLI is looked for when it is not on the
// app's PATH, as for an app started from Finder.
var dockerDirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/Applications/Docker.app/Contents/Resources/bin"}

// FindDocker returns the docker CLI's path.
func FindDocker() (string, error) {
	if p, err := exec.LookPath("docker"); err == nil {
		return p, nil
	}
	for _, d := range dockerDirs {
		p := filepath.Join(d, "docker")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("the docker command is not installed; install Docker Desktop, or Colima and the docker CLI (brew install colima docker)")
}

// NewDocker returns a Docker environment; Start creates or starts its
// container.
func NewDocker(opts DockerOptions) (*Docker, error) {
	if opts.Container == "" || (opts.Image == "" && opts.Dockerfile == "") {
		return nil, errors.New("environment: a Docker environment needs a container name and an image or Dockerfile")
	}
	docker, err := FindDocker()
	if err != nil {
		return nil, err
	}
	host, err := NewLocal(LocalOptions{ID: opts.ID, Root: opts.Root, Guard: opts.Guard})
	if err != nil {
		return nil, err
	}
	return &Docker{opts: opts, docker: docker, host: host}, nil
}

func (d *Docker) ID() string                  { return d.opts.ID }
func (d *Docker) Kind() Kind                  { return KindDocker }
func (d *Docker) Root() string                { return d.host.Root() }
func (d *Docker) Resolve(p string) string     { return d.host.Resolve(p) }
func (d *Docker) Contains(p string) bool      { return d.host.Contains(p) }
func (d *Docker) FS() FileSystem              { return d.host.FS() }
func (d *Docker) Container() string           { return d.opts.Container }
func (d *Docker) cli(args ...string) []string { return append([]string{d.docker}, args...) }

// run runs the docker CLI on the host.
func (d *Docker) run(ctx context.Context, timeout time.Duration, env map[string]string, args ...string) (ExecResult, error) {
	res, err := d.host.Exec(ctx, ExecSpec{Argv: d.cli(args...), Env: env, Timeout: timeout, MaxOutput: 64 << 10})
	if err == nil && res.ExitCode != 0 {
		err = fmt.Errorf("docker %s: %s", args[0], lastLines(res.Output, 4))
	}
	return res, err
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// State is the container's status: running, exited, created, or "" when
// it does not exist.
func (d *Docker) State(ctx context.Context) string {
	res, err := d.host.Exec(ctx, ExecSpec{Argv: d.cli("inspect", "-f", "{{.State.Status}}", d.opts.Container), Timeout: 20 * time.Second})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(res.Output)
}

// Start creates the container on first use (building the image when it
// comes from a Dockerfile, then running the setup command), starts it when
// it is stopped, and runs the health check. It returns what it did, for
// the transcript.
func (d *Docker) Start(ctx context.Context) (string, error) {
	var did []string
	switch d.State(ctx) {
	case "running":
	case "":
		image := d.opts.Image
		if d.opts.Dockerfile != "" {
			image = "freeloop-" + d.opts.Container
			buildCtx := d.opts.BuildContext
			if buildCtx == "" {
				buildCtx = filepath.Dir(d.opts.Dockerfile)
			}
			if _, err := d.run(ctx, MaxTimeout, nil, "build", "-q", "-t", image, "-f", d.opts.Dockerfile, buildCtx); err != nil {
				return "", err
			}
			did = append(did, "built the image")
		}
		root := d.Root()
		if _, err := d.run(ctx, MaxTimeout, nil, "run", "-d", "--init", "--name", d.opts.Container,
			"--label", "app=freeloop", "-v", root+":"+root, "-w", root, image, "sleep", "infinity"); err != nil {
			return "", err
		}
		did = append(did, "created the container from "+image)
		if d.opts.Setup != "" {
			res, err := d.Exec(ctx, ExecSpec{Command: d.opts.Setup, Timeout: MaxTimeout, MaxOutput: 64 << 10})
			if err != nil || res.ExitCode != 0 {
				return "", fmt.Errorf("the setup command failed: %s", lastLines(res.Output, 6))
			}
			did = append(did, "ran the setup command")
		}
	default:
		if _, err := d.run(ctx, 2*time.Minute, nil, "start", d.opts.Container); err != nil {
			return "", err
		}
		did = append(did, "started the container")
	}
	// A bind mount of a folder Docker's VM does not share comes up empty.
	if res, err := d.Exec(ctx, ExecSpec{Argv: []string{"ls", "-A", d.Root()}, Timeout: time.Minute}); err != nil || res.ExitCode != 0 || strings.TrimSpace(res.Output) == "" {
		if entries, _ := os.ReadDir(d.Root()); len(entries) > 0 {
			return strings.Join(did, ", "), fmt.Errorf("the container cannot see %s; Docker's VM shares only some folders (Colima shares your home folder by default)", d.Root())
		}
	}
	if d.opts.Health != "" {
		res, err := d.Exec(ctx, ExecSpec{Command: d.opts.Health, Timeout: 2 * time.Minute})
		if err != nil || res.ExitCode != 0 {
			return strings.Join(did, ", "), fmt.Errorf("the environment is unhealthy: %s", lastLines(res.Output, 6))
		}
	}
	return strings.Join(did, ", "), nil
}

// Stop stops the container; Remove deletes it.
func (d *Docker) Stop(ctx context.Context) error {
	_, err := d.run(ctx, time.Minute, nil, "stop", "-t", "2", d.opts.Container)
	return err
}

func (d *Docker) Remove(ctx context.Context) error {
	_, err := d.run(ctx, time.Minute, nil, "rm", "-f", d.opts.Container)
	return err
}

// execArgs is the docker exec command line: variables by name only, their
// values in the CLI's own environment.
func (d *Docker) execArgs(interactive bool, cwd string, env map[string]string) ([]string, map[string]string) {
	args := []string{"exec"}
	if interactive {
		args = append(args, "-i")
	}
	if cwd == "" {
		cwd = d.Root()
	} else {
		cwd = d.Resolve(cwd)
	}
	args = append(args, "-w", cwd)
	all := map[string]string{}
	maps.Copy(all, d.opts.Env)
	maps.Copy(all, env)
	for _, k := range slices.Sorted(maps.Keys(all)) {
		args = append(args, "-e", k)
	}
	return append(args, d.opts.Container), all
}

// Exec runs a command inside the container. A shell command runs with sh
// -lc, under timeout -s KILL so the deadline holds inside the container
// too, where killing the docker CLI would not reach.
func (d *Docker) Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	args, env := d.execArgs(false, spec.Cwd, spec.Env)
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	timeout = min(timeout, MaxTimeout)
	if len(spec.Argv) > 0 {
		args = append(args, spec.Argv...)
	} else {
		secs := fmt.Sprint(int(timeout.Seconds()))
		args = append(args, "sh", "-c", `if command -v timeout >/dev/null 2>&1; then exec timeout -s KILL "$1" sh -lc "$0"; else exec sh -lc "$0"; fi`, spec.Command, secs)
	}
	return d.host.Exec(ctx, ExecSpec{Argv: d.cli(args...), Env: env, Timeout: timeout + 5*time.Second, MaxOutput: spec.MaxOutput})
}

// Spawn starts a long-running process inside the container with stdio
// pipes, an ACP agent for example (ENV-006).
func (d *Docker) Spawn(ctx context.Context, spec SpawnSpec) (Process, error) {
	args, env := d.execArgs(true, spec.Cwd, spec.Env)
	if spec.LoginShell {
		args = append(args, "sh", "-lc", `exec "$0" "$@"`)
	}
	return d.host.Spawn(ctx, SpawnSpec{Argv: d.cli(append(args, spec.Argv...)...), Env: env})
}

// RemoveContainer deletes a container by name, when Docker is installed;
// a session's container goes when the session is deleted.
func RemoveContainer(ctx context.Context, name string) error {
	docker, err := FindDocker()
	if err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, docker, "rm", "-f", name)
	cmd.Env = baseEnv()
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "No such container") {
		return fmt.Errorf("docker rm: %s", lastLines(string(out), 2))
	}
	return nil
}
