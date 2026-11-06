//go:build unix

package environment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Nothing outlives a crash (NFR-REL-005). Every command, agent, server and terminal
// runs in a process group of its own, which the app stops when its session ends. A
// crash or a kill -9 of the app stops none of them, so each group is written down
// while it runs, with when its first process started, and the next launch stops what
// is still there. The start time tells a leftover from an unrelated process that later
// got the same number: only a group whose leader is the very process recorded is
// stopped.

// ProcessStarted says when process pid started, false if there is none. Set by the app
// for its platform (internal/platform/darwin); without it nothing is recorded, and so
// nothing is ever stopped by mistake.
var ProcessStarted func(pid int) (time.Time, bool)

type ledgerEntry struct {
	Pgid    int       `json:"pgid"`
	Started time.Time `json:"started"`
	Command string    `json:"command"`
}

var ledger struct {
	mu      sync.Mutex
	path    string
	running map[int]ledgerEntry
}

// UseLedger records process groups in path from now on. Call it once, at start, after
// ReapLeftovers.
func UseLedger(path string) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.path, ledger.running = path, map[int]ledgerEntry{}
	writeLedger()
}

// track records the group a started process leads.
func track(pid int, command string) {
	if ProcessStarted == nil {
		return
	}
	started, ok := ProcessStarted(pid)
	if !ok {
		return
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.path == "" {
		return
	}
	// Only the program's name: a command line can carry a secret.
	ledger.running[pid] = ledgerEntry{Pgid: pid, Started: started, Command: filepath.Base(command)}
	writeLedger()
}

// untrack forgets a group once its leader has been waited for.
func untrack(pid int) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.path == "" {
		return
	}
	if _, ok := ledger.running[pid]; !ok {
		return
	}
	delete(ledger.running, pid)
	writeLedger()
}

// writeLedger replaces the file whole; called with ledger.mu held. A write that
// fails leaves the last good file, which errs toward stopping too little.
func writeLedger() {
	list := make([]ledgerEntry, 0, len(ledger.running))
	for _, e := range ledger.running {
		list = append(list, e)
	}
	b, _ := json.Marshal(list)
	tmp := ledger.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, ledger.path)
	}
}

// ReapLeftovers stops the process groups path says were running when the app last
// ended without stopping them, and returns what it stopped. Each is asked to end,
// then killed if still there a moment later.
func ReapLeftovers(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []ledgerEntry
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("environment: %s: %w", filepath.Base(path), err)
	}
	var stopped []int
	var names []string
	for _, e := range list {
		if e.Pgid <= 1 || ProcessStarted == nil {
			continue
		}
		started, ok := ProcessStarted(e.Pgid)
		if !ok || !started.Equal(e.Started) {
			continue // gone, or the number now belongs to another process
		}
		if syscall.Kill(-e.Pgid, syscall.SIGTERM) == nil {
			stopped = append(stopped, e.Pgid)
			names = append(names, e.Command)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for _, pgid := range stopped {
		for time.Now().Before(deadline) && syscall.Kill(-pgid, 0) == nil {
			time.Sleep(50 * time.Millisecond)
		}
		syscall.Kill(-pgid, syscall.SIGKILL)
	}
	return names, nil
}
