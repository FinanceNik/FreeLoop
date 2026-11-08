package environment

import (
	"context"
	"io"
)

// The integrated terminal (GUI-012): an interactive shell on a
// pseudo-terminal in the environment, the user's own, typed into by hand.

// TerminalSpec says how to open a terminal.
type TerminalSpec struct {
	// Argv runs instead of the environment's login shell.
	Argv []string
	Cwd  string
	Env  map[string]string
	// Cols and Rows are the initial size.
	Cols, Rows int
}

// Terminal is a running shell on a pseudo-terminal: what is written goes to
// it as typed input, what is read is its screen output. Keep reading until
// EOF: on macOS a shell cannot finish exiting while its output is unread.
type Terminal interface {
	io.ReadWriter
	Resize(cols, rows int) error
	// Close ends the shell and everything it started.
	Close() error
	// Wait returns when the shell has exited, with its exit code.
	Wait() (int, error)
}

// Terminals is an environment that can open terminals.
type Terminals interface {
	OpenTerminal(ctx context.Context, spec TerminalSpec) (Terminal, error)
}

// OpenTerminal opens a shell in the container (bash when it has one) with
// `docker exec -it`, on a pseudo-terminal on the host.
func (d *Docker) OpenTerminal(ctx context.Context, spec TerminalSpec) (Terminal, error) {
	id := newExecID()
	env := map[string]string{"TERM": "xterm-256color"}
	for k, v := range spec.Env {
		env[k] = v
	}
	args, hostEnv := d.execArgs(true, spec.Cwd, env, id)
	// -t after -i: the last argument execArgs adds is the container.
	args = append(append(append([]string{}, args[:len(args)-1]...), "-t"), args[len(args)-1])
	if len(spec.Argv) > 0 {
		args = append(args, spec.Argv...)
	} else {
		args = append(args, "sh", "-c", `if command -v bash >/dev/null 2>&1; then exec bash -l; else exec sh -l; fi`)
	}
	t, err := d.host.OpenTerminal(ctx, TerminalSpec{Argv: d.cli(args...), Env: hostEnv, Cols: spec.Cols, Rows: spec.Rows})
	if err != nil {
		return nil, err
	}
	t.(interface{ setOnClose(func()) }).setOnClose(func() { d.killExec(id) })
	return t, nil
}

// LoginCommand is the argv that runs command in a terminal of env: through
// the user's login shell on this computer, so their PATH applies, or sh in a
// container.
func LoginCommand(env Environment, command string) []string {
	if l, ok := env.(*Local); ok && len(l.shell) > 0 {
		return []string{l.shell[0], "-l", "-c", command}
	}
	return []string{"sh", "-lc", command}
}
