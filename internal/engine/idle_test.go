package engine

import (
	"context"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

func hasRuntime(e *Engine, sid string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.runtimes[sid]
	return ok
}

// putIdleRuntime installs a runtime with no agent behind it that has been idle
// since long before any limit.
func putIdleRuntime(e *Engine, sid string) *runtime {
	r := &runtime{sid: sid, host: &environment.Local{}, idleSince: time.Now().Add(-24 * time.Hour)}
	e.mu.Lock()
	e.runtimes[sid] = r
	e.mu.Unlock()
	return r
}

// OQ-053: a session idle past the time loses its agent, and a message to it starts
// one again whose turn runs.
func TestAnIdleSessionIsStoppedAndStartsAgain(t *testing.T) {
	ctx := context.Background()
	e, ses := referenceAgentSession(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	if _, err := e.Prompt(ctx, ses.ID, "one"); err != nil {
		t.Fatal(err)
	}
	stopEarlyIn(t, e, ch, ses.ID)
	e.mu.Lock()
	first := e.runtimes[ses.ID]
	e.mu.Unlock()
	if first == nil {
		t.Fatal("no runtime after a turn")
	}
	// The turn ends a moment before its runtime lets go of it.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		first.mu.Lock()
		busy := first.busy
		first.mu.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the runtime stays busy")
		}
	}
	// Not yet.
	e.stopIdle(time.Now().Add(29 * time.Minute))
	if !hasRuntime(e, ses.ID) {
		t.Fatal("stopped before the time")
	}
	e.stopIdle(time.Now().Add(31 * time.Minute))
	if hasRuntime(e, ses.ID) {
		t.Fatal("still running after the time")
	}
	if !first.stopped {
		t.Error("the runtime was not stopped")
	}
	if _, err := e.Prompt(ctx, ses.ID, "two"); err != nil {
		t.Fatal(err)
	}
	stopEarlyIn(t, e, ch, ses.ID)
	e.mu.Lock()
	again := e.runtimes[ses.ID]
	e.mu.Unlock()
	if again == nil || again == first {
		t.Error("the agent was not started again")
	}
}

// OQ-053: the time is a setting, and "never" a choice.
func TestTheIdleTimeIsASetting(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	putIdleRuntime(e, "a")
	e.mu.Lock()
	e.cfg.Sessions.IdleStop = "never"
	e.mu.Unlock()
	e.stopIdle(time.Now())
	if !hasRuntime(e, "a") {
		t.Fatal("stopped with idleStop never")
	}
	e.mu.Lock()
	e.cfg.Sessions.IdleStop = "2h"
	e.mu.Unlock()
	e.runtimes["a"].idleSince = time.Now().Add(-time.Hour)
	e.stopIdle(time.Now())
	if !hasRuntime(e, "a") {
		t.Fatal("stopped after an hour with idleStop 2h")
	}
	e.stopIdle(time.Now().Add(2 * time.Hour))
	if hasRuntime(e, "a") {
		t.Fatal("kept past idleStop 2h")
	}
}

// OQ-053: a turn running, a queued message, a waiting approval and an open
// terminal each keep the session's agent.
func TestASessionWithWorkIsNeverStopped(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "work", "ask")
	later := time.Now().Add(time.Hour)

	r := putIdleRuntime(e, ses.ID)
	r.mu.Lock()
	r.busy = true
	r.mu.Unlock()
	e.stopIdle(later)
	if !hasRuntime(e, ses.ID) {
		t.Fatal("stopped with a turn running")
	}
	r.mu.Lock()
	r.busy = false
	r.queue = []queued{{text: "next"}}
	r.mu.Unlock()
	e.stopIdle(later)
	if !hasRuntime(e, ses.ID) {
		t.Fatal("stopped with a message queued")
	}
	r.mu.Lock()
	r.queue = nil
	r.mu.Unlock()

	asked := make(chan permission.Decision, 1)
	go func() {
		asked <- e.broker.Request(ctx, permission.Context{Mode: permission.ModeAsk, Root: ses.ProjectRoot}, permission.Request{SessionID: ses.ID, Tool: "shell", Kind: permission.KindExecute, Command: "make test"})
	}()
	for deadline := time.Now().Add(5 * time.Second); len(e.broker.Pending()) == 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("no request is waiting")
		}
	}
	e.stopIdle(later)
	if !hasRuntime(e, ses.ID) {
		t.Fatal("stopped with an approval waiting")
	}
	e.broker.CancelSession(ses.ID, "test")
	<-asked

	term := &terminal{info: TerminalInfo{ID: "term_1", SessionID: ses.ID}}
	e.terms.mu.Lock()
	e.terms.open = map[string]*terminal{"term_1": term}
	e.terms.mu.Unlock()
	e.stopIdle(later)
	if !hasRuntime(e, ses.ID) {
		t.Fatal("stopped with a terminal open")
	}
	term.mu.Lock()
	term.exited = true
	term.mu.Unlock()
	e.stopIdle(later)
	if hasRuntime(e, ses.ID) {
		t.Fatal("kept for a terminal that ended")
	}
	e.terms.mu.Lock()
	e.terms.open = nil // the stand-in has no shell to close
	e.terms.mu.Unlock()
}

// OQ-053: what a stopped session held goes with it.
func TestAStoppedSessionLetsGoOfWhatItHeld(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "held", "ask")
	if _, err := e.store.Append(ctx, ses.ID, event.Notice, []byte(`{"message":"x"}`)); err != nil {
		t.Fatal(err)
	}
	putIdleRuntime(e, ses.ID)
	e.stuck.mu.Lock()
	e.stuck.state(ses.ID)
	e.stuck.mu.Unlock()
	e.contextMu.Lock()
	e.contextSeen = map[string][32]byte{ses.ID + "\x00turn": {1}, "other\x00turn": {2}}
	e.contextMu.Unlock()
	e.stopIdle(time.Now().Add(time.Hour))
	e.stuck.mu.Lock()
	_, stuck := e.stuck.sessions[ses.ID]
	e.stuck.mu.Unlock()
	e.contextMu.Lock()
	seen := len(e.contextSeen)
	_, other := e.contextSeen["other\x00turn"]
	e.contextMu.Unlock()
	if stuck || seen != 1 || !other {
		t.Errorf("stuck %v, contextSeen %d (other kept %v)", stuck, seen, other)
	}
	if e.store.IsOpen(ses.ID) {
		t.Error("the transcript file is still open")
	}
}
