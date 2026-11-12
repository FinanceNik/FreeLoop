package engine

import "time"

// stopIdle stops the runtime of every session that has sat idle for the time in
// settings (OQ-053): no turn, no waiting approval, no queued message and no
// open terminal for that long. The agent's process or container goes with it,
// and the next message starts them again with the session's history, as after
// a restart of the app. The config watch calls it on every poll.
func (e *Engine) stopIdle(now time.Time) {
	after, on := e.Config().Sessions.IdleStopAfter()
	if !on {
		return
	}
	cutoff := now.Add(-after)
	var due []string
	e.mu.Lock()
	for sid, r := range e.runtimes {
		r.mu.Lock()
		if !r.busy && len(r.queue) == 0 && r.idleSince.Before(cutoff) {
			due = append(due, sid)
		}
		r.mu.Unlock()
	}
	e.mu.Unlock()
	for _, sid := range due {
		if e.holdsWork(sid) {
			continue
		}
		e.stopIdleRuntime(sid, cutoff)
	}
}

// holdsWork says whether something other than a turn keeps a session's runtime:
// an approval waiting for the user, or a terminal still open.
func (e *Engine) holdsWork(sid string) bool {
	for _, r := range e.broker.Pending() {
		if r.SessionID == sid {
			return true
		}
	}
	e.terms.mu.Lock()
	defer e.terms.mu.Unlock()
	for _, t := range e.terms.open {
		if t.info.SessionID != sid {
			continue
		}
		t.mu.Lock()
		exited := t.exited
		t.mu.Unlock()
		if !exited {
			return true
		}
	}
	return false
}

// forgetRuntimeState lets go of what the engine kept for a session whose runtime
// stopped for being idle: its open transcript file, its stuck watch and the
// record of what its model calls last said. Each is made again on use.
func (e *Engine) forgetRuntimeState(sid string) {
	e.store.Release(sid)
	e.stuck.mu.Lock()
	delete(e.stuck.sessions, sid)
	e.stuck.mu.Unlock()
	e.contextMu.Lock()
	for k := range e.contextSeen {
		if len(k) > len(sid) && k[:len(sid)+1] == sid+"\x00" {
			delete(e.contextSeen, k)
		}
	}
	e.contextMu.Unlock()
}

// stopFinished stops the runtime of a session whose work an AFK run took as
// ended: it waits briefly for the turn to let go, since a session shows idle a
// moment before its runtime does.
func (e *Engine) stopFinished(sid string) {
	e.mu.Lock()
	r := e.runtimes[sid]
	e.mu.Unlock()
	if r == nil {
		return
	}
	r.mu.Lock()
	done := r.done
	busy := r.busy
	r.mu.Unlock()
	if busy && done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
	if !e.holdsWork(sid) {
		e.stopIfIdle(sid)
	}
}
