package environment

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// screen collects everything a terminal prints, reading until it ends.
type screen struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func watch(term Terminal) *screen {
	s := &screen{}
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := term.Read(b)
			s.mu.Lock()
			s.buf.Write(b[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *screen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// waitFor waits until the screen shows want.
func (s *screen) waitFor(t *testing.T, want string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if strings.Contains(s.String(), want) {
			return
		}
	}
	t.Fatalf("no %q in the terminal's output:\n%s", want, s.String())
}

// GUI-012: a terminal is an interactive shell in the environment's folder,
// with the environment's variables, a size it can change, and an exit code.
func TestLocalTerminal(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLocal(LocalOptions{Root: dir, Guard: denyGuard{}, Shell: []string{"/bin/sh", "-c"}, Env: map[string]string{"FREELOOP_TERM_TEST": "from-the-env"}})
	if err != nil {
		t.Fatal(err)
	}
	term, err := l.OpenTerminal(context.Background(), TerminalSpec{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	out := watch(term)
	io.WriteString(term, "echo \"[$(pwd)] $FREELOOP_TERM_TEST $(stty size) $TERM\"\n")
	out.waitFor(t, "] from-the-env 30 100 xterm-256color")
	if !strings.Contains(out.String(), "["+l.Root()+"]") {
		t.Errorf("not in the folder:\n%s", out.String())
	}
	if err := term.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	io.WriteString(term, "stty size; exit 3\n")
	out.waitFor(t, "40 120")
	if code, err := term.Wait(); err != nil || code != 3 {
		t.Errorf("exit = %d, %v", code, err)
	}
	term.Close()
}

// Close ends the shell and what it started, even when the shell ignores
// the hangup.
func TestTerminalClose(t *testing.T) {
	l, err := NewLocal(LocalOptions{Root: t.TempDir(), Guard: denyGuard{}, Shell: []string{"/bin/sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	term, err := l.OpenTerminal(context.Background(), TerminalSpec{Argv: []string{"/bin/sh", "-c", "trap '' HUP; echo ready; sleep 60"}})
	if err != nil {
		t.Fatal(err)
	}
	watch(term).waitFor(t, "ready")
	start := time.Now()
	term.Close()
	if _, err := term.Wait(); err != nil && !strings.Contains(err.Error(), "signal") {
		t.Errorf("wait: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("Close took too long")
	}
}
