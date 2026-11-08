package environment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
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

// GroupMembers lists a process group's processes, set like ProcessStarted. With it, a
// group whose first process has ended, and whose other processes run on (a command
// that put something in the background), is stopped too.
var GroupMembers func(pgid int) []int

type ledgerEntry struct {
	Pgid    int       `json:"pgid"`
	Started time.Time `json:"started"`
	Command string    `json:"command"`
	// Lingering is when the group's first process was seen to have ended with others
	// of the group still running. Only a process that started between Started and
	// then can be one of them, so a later group with the same number is never taken
	// for this one.
	Lingering time.Time `json:"lingering,omitzero"`
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

// track records the group a started process leads, and returns when that process
// started, which untrack and linger take, so a later process with the same number
// is never mistaken for it.
func track(pid int, command string) time.Time {
	if ProcessStarted == nil {
		return time.Time{}
	}
	started, ok := ProcessStarted(pid)
	if !ok {
		return time.Time{}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.path == "" {
		return started
	}
	// Only the program's name: a command line can carry a secret.
	ledger.running[pid] = ledgerEntry{Pgid: pid, Started: started, Command: filepath.Base(command)}
	writeLedger()
	return started
}

// untrack forgets a group once it has ended.
func untrack(pid int, started time.Time) {
	releaseGroup(pid)
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if e, ok := ledger.running[pid]; ok && e.Started.Equal(started) && ledger.path != "" {
		delete(ledger.running, pid)
		writeLedger()
	}
}

// linger marks a group whose first process has ended while others run on.
func linger(pid int, started time.Time) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if e, ok := ledger.running[pid]; ok && e.Started.Equal(started) && ledger.path != "" {
		e.Lingering = time.Now()
		ledger.running[pid] = e
		writeLedger()
	}
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

// leftover reports whether group e.Pgid is still the one recorded: its first process
// is the very one, or, for a lingering group, one of its processes started between
// the group's start and when it was seen lingering. A number given to another process
// since matches neither.
func leftover(e ledgerEntry) bool {
	if started, ok := ProcessStarted(e.Pgid); ok {
		return started.Equal(e.Started)
	}
	if e.Lingering.IsZero() || GroupMembers == nil {
		return false
	}
	for _, pid := range GroupMembers(e.Pgid) {
		if at, ok := ProcessStarted(pid); ok && !at.Before(e.Started) && !at.After(e.Lingering) {
			return true
		}
	}
	return false
}
