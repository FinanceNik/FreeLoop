//go:build unix

package environment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type ptyTerminal struct {
	f       *os.File
	cmd     *exec.Cmd
	onClose func()
	done    chan struct{}
	code    int
	err     error
	once    sync.Once
}

func size(cols, rows int) *pty.Winsize {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}
}

// OpenTerminal starts the user's login shell, or spec.Argv, on a
// pseudo-terminal in the environment's folder. It runs in a session of its
// own, so Close ends everything it started.
func (l *Local) OpenTerminal(ctx context.Context, spec TerminalSpec) (Terminal, error) {
	argv := spec.Argv
	if len(argv) == 0 {
		argv = []string{l.shell[0], "-l"}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = l.root
	if spec.Cwd != "" {
		cmd.Dir = l.Resolve(spec.Cwd)
	}
	cmd.Env = append(l.baseEnv(), "TERM=xterm-256color", "COLORTERM=truecolor")
	for k, v := range l.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// pty.Start makes the shell a session leader with the terminal as its
	// controlling terminal, which is its own process group too.
	f, err := pty.StartWithSize(cmd, size(spec.Cols, spec.Rows))
	if err != nil {
		return nil, fmt.Errorf("environment: start a terminal: %w", err)
	}
	t := &ptyTerminal{f: f, cmd: cmd, done: make(chan struct{})}
	started := track(cmd.Process.Pid, cmd.Path)
	go func() {
		err := cmd.Wait()
		// A terminal tab's shell that ended takes its jobs with it, as closing a
		// terminal window does (NFR-REL-005).
		stopGroup(cmd.Process.Pid)
		untrack(cmd.Process.Pid, started)
		var exitErr *exec.ExitError
		switch {
		case err == nil:
		case errors.As(err, &exitErr):
			t.code = exitErr.ExitCode()
		default:
			t.err = err
		}
		close(t.done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			t.Close()
		case <-t.done:
		}
	}()
	return t, nil
}

func (t *ptyTerminal) Read(p []byte) (int, error) {
	n, err := t.f.Read(p)
	// The terminal's master reports EIO once the shell is gone.
	if err != nil && errors.Is(err, syscall.EIO) {
		err = io.EOF
	}
	return n, err
}

func (t *ptyTerminal) Write(p []byte) (int, error) { return t.f.Write(p) }

func (t *ptyTerminal) Resize(cols, rows int) error { return pty.Setsize(t.f, size(cols, rows)) }

func (t *ptyTerminal) Close() error {
	t.once.Do(func() {
		// Hang up, as closing a terminal window does, then make sure.
		// Closing our side first matters: on macOS a process cannot finish
		// exiting while output it wrote to the terminal is unread.
		if t.cmd.Process != nil {
			syscall.Kill(-t.cmd.Process.Pid, syscall.SIGHUP)
		}
		t.f.Close()
		select {
		case <-t.done:
		case <-time.After(2 * time.Second):
			if t.cmd.Process != nil {
				syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
			}
			<-t.done
		}
		if t.onClose != nil {
			t.onClose()
		}
	})
	return nil
}

func (t *ptyTerminal) Wait() (int, error) {
	<-t.done
	return t.code, t.err
}

func (t *ptyTerminal) setOnClose(f func()) { t.onClose = f }
