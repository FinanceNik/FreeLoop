//go:build darwin

package environment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// started is darwin.ProcessStart, which this package cannot import in its tests.
func started(pid int) (time.Time, bool) {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(k.Proc.P_pid) != pid {
		return time.Time{}, false
	}
	return time.Unix(int64(k.Proc.P_starttime.Sec), int64(k.Proc.P_starttime.Usec)*1000), true
}

// NFR-REL-005: a process group a crash left running is stopped at the next start; one
// that ended is forgotten; and a process that only got the same number is left alone.
func TestLeftoversAreStopped(t *testing.T) {
	ProcessStarted = started
	t.Cleanup(func() { ProcessStarted = nil; UseLedger(filepath.Join(t.TempDir(), "off.json")) })
	ledgerFile := filepath.Join(t.TempDir(), "running.json")
	UseLedger(ledgerFile)
	l, err := NewLocal(LocalOptions{Root: t.TempDir(), Guard: denyGuard{}, Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	recorded := func() []ledgerEntry {
		var list []ledgerEntry
		b, _ := os.ReadFile(ledgerFile)
		json.Unmarshal(b, &list)
		return list
	}

	// One that ends by itself leaves nothing behind.
	if _, err := l.Exec(context.Background(), ExecSpec{Command: "true"}); err != nil {
		t.Fatal(err)
	}
	if got := recorded(); len(got) != 0 {
		t.Fatalf("an ended command is still recorded: %+v", got)
	}

	// One left running, as by a crash: the app stops nothing, then starts again.
	left, err := l.Spawn(context.Background(), SpawnSpec{Argv: []string{"sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := l.Spawn(context.Background(), SpawnSpec{Argv: []string{"sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Kill(-other.(*proc).cmd.Process.Pid, syscall.SIGKILL) })
	list := recorded()
	if len(list) != 2 {
		t.Fatalf("recorded %+v", list)
	}
	// The second's number, as if it now belonged to a process started later.
	for i := range list {
		if list[i].Pgid == other.(*proc).cmd.Process.Pid {
			list[i].Started = list[i].Started.Add(-time.Hour)
		}
	}
	b, _ := json.Marshal(list)
	os.WriteFile(ledgerFile, b, 0o600)

	stopped, err := ReapLeftovers(ledgerFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(stopped) != 1 || stopped[0] != "sleep" {
		t.Errorf("stopped %q", stopped)
	}
	select {
	case <-left.(*proc).done:
	case <-time.After(5 * time.Second):
		t.Error("the leftover is still running")
	}
	if syscall.Kill(other.(*proc).cmd.Process.Pid, 0) != nil {
		t.Error("a process that only had the recorded number was stopped")
	}
	if names, err := ReapLeftovers(filepath.Join(t.TempDir(), "none.json")); err != nil || names != nil {
		t.Errorf("no file: %q, %v", names, err)
	}
}
