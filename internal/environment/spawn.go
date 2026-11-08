package environment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// SpawnSpec starts a long-running process that talks over its stdin and
// stdout: an MCP server or an ACP agent (MCP-001, HRN-005). Like Exec it
// gets only the base environment plus the environment's variables and Env
// (SEC-008), and runs in its own process group.
type SpawnSpec struct {
	Argv []string
	Cwd  string
	Env  map[string]string
	// LoginShell runs Argv through the environment's shell as
	// `exec "$0" "$@"`, so commands such as npx resolve on the user's login
	// PATH while the arguments stay exactly as given.
	LoginShell bool
}

// Process is a spawned process.
type Process interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	// Stderr returns the last bytes the process wrote to stderr.
	Stderr() string
	// Wait blocks until the process exits.
	Wait() error
	// Kill ends the process and everything it started.
	Kill()
	Pid() int
}

// Spawner is implemented by environments that can start processes with
// pipes; the local environment is one.
type Spawner interface {
	Spawn(ctx context.Context, spec SpawnSpec) (Process, error)
}

// stderrWait bounds waiting for a process that outlives what it was doing.
const stderrWait = time.Second

// StderrAfterExit returns what a process wrote to stderr, for saying why it stopped.
// Seeing its stdout close is not enough to read that yet, since stderr is copied
// separately and is complete only once the process has been waited for.
func StderrAfterExit(p Process) string {
	done := make(chan struct{})
	go func() {
		p.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(stderrWait):
	}
	return p.Stderr()
}

type proc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *tail
	err    error
	done   chan struct{}
}

func (p *proc) Stdin() io.WriteCloser { return p.stdin }
func (p *proc) Stdout() io.ReadCloser { return p.stdout }
func (p *proc) Stderr() string        { return p.stderr.String() }
func (p *proc) Pid() int              { return p.cmd.Process.Pid }
func (p *proc) Kill()                 { killProcessGroup(p.cmd) }
func (p *proc) Wait() error {
	<-p.done
	return p.err
}

// Spawn starts spec. Cancelling ctx kills the process group.
func (l *Local) Spawn(ctx context.Context, spec SpawnSpec) (Process, error) {
	if len(spec.Argv) == 0 || spec.Argv[0] == "" {
		return nil, errors.New("environment: no command to start")
	}
	argv := l.viaShellIfBatch(spec.Argv)
	if spec.LoginShell {
		argv = append(append(append([]string{}, l.shell...), `exec "$0" "$@"`), spec.Argv...)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = l.root
	if spec.Cwd != "" {
		cmd.Dir = l.Resolve(spec.Cwd)
	}
	cmd.Env = l.baseEnv()
	for k, v := range l.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	setProcessGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p := &proc{cmd: cmd, stdin: stdin, stdout: stdout, stderr: &tail{limit: 4096}, done: make(chan struct{})}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("environment: start %s: %w", spec.Argv[0], startHint(err))
	}
	if err := joinGroup(cmd); err != nil {
		killProcessGroup(cmd)
		cmd.Wait()
		return nil, fmt.Errorf("environment: start %s: %w", spec.Argv[0], err)
	}
	started := track(cmd.Process.Pid, cmd.Path)
	go func() {
		p.err = cmd.Wait()
		// An agent or server that ended leaves nothing of its own running behind it
		// (NFR-REL-005).
		stopGroup(cmd.Process.Pid)
		untrack(cmd.Process.Pid, started)
		close(p.done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			killProcessGroup(cmd)
		case <-p.done:
		}
	}()
	return p, nil
}

// tail keeps the last limit bytes written to it.
type tail struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if over := len(t.buf) - t.limit; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(b), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
