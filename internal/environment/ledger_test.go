//go:build darwin

package environment

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// members is darwin.GroupMembers, by asking ps, which is enough for a test.
func members(pgid int) []int {
	out, _ := exec.Command("ps", "-g", strconv.Itoa(pgid), "-o", "pid=").Output()
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if n, err := strconv.Atoi(f); err == nil {
			pids = append(pids, n)
		}
	}
	return pids
}

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

// NFR-REL-005: what a command puts in the background runs on until the session stops,
// then stops; an agent that ends takes its children with it; and after a crash, a
// group whose first process had ended is still stopped at the next start.
func TestBackgroundedProcessesStop(t *testing.T) {
	ProcessStarted, GroupMembers = started, members
	t.Cleanup(func() { ProcessStarted, GroupMembers = nil, nil; UseLedger(filepath.Join(t.TempDir(), "off.json")) })
	ledgerFile := filepath.Join(t.TempDir(), "running.json")
	UseLedger(ledgerFile)
	l, err := NewLocal(LocalOptions{Root: t.TempDir(), Guard: denyGuard{}, Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	pidOf := func(out string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(out))
		return n
	}
	gone := func(pid int) bool {
		for range 100 {
			if syscall.Kill(pid, 0) != nil {
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
		return false
	}

	// A dev server an agent starts: it outlives the command, not the session.
	res, err := l.Exec(context.Background(), ExecSpec{Command: "sleep 60 >/dev/null 2>&1 & echo $!"})
	if err != nil {
		t.Fatal(err)
	}
	server := pidOf(res.Output)
	t.Cleanup(func() { syscall.Kill(server, syscall.SIGKILL) })
	if server == 0 || syscall.Kill(server, 0) != nil {
		t.Fatalf("the background process did not start: %q", res.Output)
	}
	l.StopLingering()
	if !gone(server) {
		t.Error("the background process outlived its session")
	}

	// An agent that exits: its children go with it.
	p, err := l.Spawn(context.Background(), SpawnSpec{Argv: []string{"sh", "-c", "sleep 60 >/dev/null 2>&1 & echo $!"}})
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 32)
	n, _ := p.Stdout().Read(b)
	child := pidOf(string(b[:n]))
	t.Cleanup(func() { syscall.Kill(child, syscall.SIGKILL) })
	p.Wait()
	if !gone(child) {
		t.Error("an exited agent's child ran on")
	}

	// A crash with something lingering: the next start stops it.
	res, err = l.Exec(context.Background(), ExecSpec{Command: "sleep 60 >/dev/null 2>&1 & echo $!"})
	if err != nil {
		t.Fatal(err)
	}
	orphan := pidOf(res.Output)
	t.Cleanup(func() { syscall.Kill(orphan, syscall.SIGKILL) })
	stopped, err := ReapLeftovers(ledgerFile)
	if err != nil || len(stopped) != 1 {
		t.Errorf("stopped %q, %v", stopped, err)
	}
	if !gone(orphan) {
		t.Error("what a crash left lingering ran on")
	}
}
