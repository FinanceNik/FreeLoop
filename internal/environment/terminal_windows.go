package environment

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// conptyTerminal is a shell on a Windows pseudo console (ConPTY, Windows 10 1809 and
// later), in a job of its own like every command (proc_windows.go).
type conptyTerminal struct {
	hpc     windows.Handle
	in      *os.File // the shell's input: what the user types
	out     *os.File // the shell's screen output
	process windows.Handle
	pid     int
	onClose func()
	done    chan struct{}
	code    int
	err     error
	once    sync.Once
}

func coord(cols, rows int) windows.Coord {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return windows.Coord{X: int16(cols), Y: int16(rows)}
}

// OpenTerminal starts Git Bash as a login shell, or spec.Argv, on a pseudo console in
// the environment's folder. It runs in a job of its own, so Close ends everything it
// started.
func (l *Local) OpenTerminal(ctx context.Context, spec TerminalSpec) (Terminal, error) {
	argv := spec.Argv
	if len(argv) == 0 {
		argv = []string{l.shell[0], "-l", "-i"}
	}
	program, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, fmt.Errorf("environment: start a terminal: %w", startHint(err))
	}
	dir := l.root
	if spec.Cwd != "" {
		dir = l.Resolve(spec.Cwd)
	}
	env := append(l.baseEnv(), terminalEnv()...)
	for k, v := range l.env {
		env = append(env, k+"="+v)
	}
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}

	// Two pipes: one the console reads the user's typing from, one it writes the
	// screen to. The console keeps its own copies of its ends.
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, fmt.Errorf("environment: start a terminal: %w", err)
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, fmt.Errorf("environment: start a terminal: %w", err)
	}
	var hpc windows.Handle
	err = windows.CreatePseudoConsole(coord(spec.Cols, spec.Rows), inR, outW, 0, &hpc)
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)
	t := &conptyTerminal{hpc: hpc, in: os.NewFile(uintptr(inW), "conpty-in"), out: os.NewFile(uintptr(outR), "conpty-out"), done: make(chan struct{})}
	if err != nil {
		t.in.Close()
		t.out.Close()
		return nil, fmt.Errorf("environment: start a terminal: %w", err)
	}
	if err := t.start(program, argv, dir, env); err != nil {
		windows.ClosePseudoConsole(hpc)
		t.in.Close()
		t.out.Close()
		return nil, fmt.Errorf("environment: start a terminal: %w", err)
	}
	go t.wait()
	go func() {
		select {
		case <-ctx.Done():
			t.Close()
		case <-t.done:
		}
	}()
	return t, nil
}

// start runs the program attached to the pseudo console, suspended until it is in
// its job.
func (t *conptyTerminal) start(program string, argv []string, dir string, env []string) error {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return err
	}
	defer attrs.Delete()
	// The attribute's value is the console handle itself, not a pointer to it.
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&t.hpc)), unsafe.Sizeof(t.hpc)); err != nil {
		return err
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{program}, argv[1:]...)))
	if err != nil {
		return err
	}
	app, err := windows.UTF16PtrFromString(program)
	if err != nil {
		return err
	}
	cwd, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	block, err := envBlock(env)
	if err != nil {
		return err
	}
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcess(app, cmdline, nil, nil, false, flags, &block[0], cwd, &si.StartupInfo, &pi); err != nil {
		return err
	}
	defer windows.CloseHandle(pi.Thread)
	t.process, t.pid = pi.Process, int(pi.ProcessId)
	if err := addToNewJob(t.pid, pi.Process); err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Process)
		return err
	}
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		stopGroup(t.pid)
		windows.CloseHandle(pi.Process)
		return err
	}
	return nil
}

// envBlock is env as CreateProcess takes it: each NAME=value ended by a zero, and a
// zero after the last. A later value for a name replaces an earlier one, names
// compared without case, as Windows does.
func envBlock(env []string) ([]uint16, error) {
	last := map[string]int{}
	for i, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		last[strings.ToUpper(k)] = i
	}
	var block []uint16
	for i, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if last[strings.ToUpper(k)] != i {
			continue
		}
		u, err := windows.UTF16FromString(kv)
		if err != nil {
			return nil, err
		}
		block = append(block, u...)
	}
	return append(block, 0), nil
}

// wait records how the shell ended, ends what it left running, as closing a terminal
// window does (NFR-REL-005), and closes the console, which ends its output.
func (t *conptyTerminal) wait() {
	windows.WaitForSingleObject(t.process, windows.INFINITE)
	var code uint32
	if err := windows.GetExitCodeProcess(t.process, &code); err != nil {
		t.err = err
	}
	t.code = int(code)
	windows.CloseHandle(t.process)
	stopGroup(t.pid)
	windows.ClosePseudoConsole(t.hpc)
	close(t.done)
}

// Read returns the screen output, and io.EOF once the console has closed: a pipe
// whose writer is gone reads as the end.
func (t *conptyTerminal) Read(p []byte) (int, error) {
	n, err := t.out.Read(p)
	if err != nil {
		t.out.Close()
	}
	return n, err
}

func (t *conptyTerminal) Write(p []byte) (int, error) { return t.in.Write(p) }

func (t *conptyTerminal) Resize(cols, rows int) error {
	return windows.ResizePseudoConsole(t.hpc, coord(cols, rows))
}

func (t *conptyTerminal) Close() error {
	t.once.Do(func() {
		stopGroup(t.pid)
		select {
		case <-t.done:
		case <-time.After(2 * time.Second):
		}
		t.in.Close()
		if t.onClose != nil {
			t.onClose()
		}
	})
	return nil
}

func (t *conptyTerminal) Wait() (int, error) {
	<-t.done
	return t.code, t.err
}

func (t *conptyTerminal) setOnClose(f func()) { t.onClose = f }
