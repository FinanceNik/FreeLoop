//go:build darwin

package environment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
)

// A session's shared SSH connection ends with the session, and so on quitting, not up
// to ControlPersist later: Close asks its master to exit, kills one that cannot hear,
// and removes a socket a killed one left. The master is in the process ledger while it
// runs, though ssh puts it in a group of its own, so the next launch stops it after a
// crash.
func TestSSHSharedConnectionEndsWithTheSession(t *testing.T) {
	ProcessStarted = started
	t.Cleanup(func() { ProcessStarted = nil; UseLedger(filepath.Join(t.TempDir(), "off.json")) })
	ledgerFile := filepath.Join(t.TempDir(), "running.json")
	UseLedger(ledgerFile)
	recorded := func(pid int) bool {
		var list []ledgerEntry
		b, _ := os.ReadFile(ledgerFile)
		json.Unmarshal(b, &list)
		return slices.ContainsFunc(list, func(e ledgerEntry) bool { return e.Pgid == pid })
	}
	// A socket's path has a short limit, which a test's temporary folder can pass.
	controlDir, err := os.MkdirTemp("/tmp", "flssh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(controlDir) })
	sockets := func() []string {
		var out []string
		entries, _ := os.ReadDir(controlDir)
		for _, e := range entries {
			if e.Type() == os.ModeSocket {
				out = append(out, e.Name())
			}
		}
		return out
	}
	gone := func(pid int) bool {
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if _, ok := started(pid); !ok {
				return true
			}
		}
		return false
	}
	r := newRig(t, map[string]string{"a": "a"}, func(o *SSHOptions) { o.ControlDir = controlDir })
	ctx := context.Background()
	master := func() int {
		t.Helper()
		r.exec("true")
		r.env.masterMu.Lock()
		pid := r.env.master
		r.env.masterMu.Unlock()
		if pid == 0 {
			t.Fatal("the shared connection's master was not seen")
		}
		if pgid, _ := syscall.Getpgid(pid); pgid != pid {
			t.Fatalf("the master %d is in group %d, not its own", pid, pgid)
		}
		if !recorded(pid) {
			t.Fatalf("the master %d is not in the ledger", pid)
		}
		if len(sockets()) != 1 {
			t.Fatalf("sockets = %v", sockets())
		}
		return pid
	}

	// Close ends the master and forgets it.
	pid := master()
	r.env.Close(ctx)
	if !gone(pid) {
		t.Fatalf("the master %d runs on after Close", pid)
	}
	if recorded(pid) || len(sockets()) != 0 {
		t.Errorf("after Close: recorded %v, sockets %v", recorded(pid), sockets())
	}

	// A master that was killed leaves its socket, which Close removes.
	pid = master()
	syscall.Kill(pid, syscall.SIGKILL)
	if !gone(pid) {
		t.Fatal("the master did not die")
	}
	r.env.Close(ctx)
	if recorded(pid) || len(sockets()) != 0 {
		t.Errorf("after a killed master: recorded %v, sockets %v", recorded(pid), sockets())
	}

	// One whose socket went cannot be asked to exit, and is stopped by its group.
	pid = master()
	for _, name := range sockets() {
		os.Remove(filepath.Join(controlDir, name))
	}
	r.env.Close(ctx)
	if !gone(pid) || recorded(pid) {
		t.Errorf("a master that could not hear runs on: %d", pid)
	}

	// After a crash, the next launch stops it.
	pid = master()
	if stopped, err := ReapLeftovers(ledgerFile); err != nil || !slices.Contains(stopped, "ssh") {
		t.Errorf("reap = %v, %v", stopped, err)
	}
	if !gone(pid) {
		t.Errorf("the master %d survived the next launch", pid)
	}
	r.env.Close(ctx)
}
