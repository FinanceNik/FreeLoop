package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// agentsAlive counts the processes running this test binary as a reference agent,
// which is how the engine starts it.
func agentsAlive(t *testing.T) int {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ps", "-A", "-o", "pid=,command=").Output()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == exe && f[0] != fmt.Sprint(os.Getpid()) {
			n++
		}
	}
	return n
}

// OQ-053, run by scripts/memcheck.sh (NFR-PERF-004) with FREELOOP_MEMCHECK=1 and not
// as a test: ten sessions on the reference agent each hold a process until the idle
// time passes, then none does. Takes about a minute, the least time a setting allows.
func TestMemcheckIdleStop(t *testing.T) {
	if os.Getenv("FREELOOP_MEMCHECK") != "1" {
		t.Skip("run by scripts/memcheck.sh")
	}
	ctx := context.Background()
	e, first := referenceAgentSession(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	e.mu.Lock()
	e.cfg.Sessions.IdleStop = "1m"
	e.mu.Unlock()
	sids := []string{first.ID}
	for len(sids) < 10 {
		ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: first.ProjectRoot, ProfileID: "ref", Mode: "accept_edits"})
		if err != nil {
			t.Fatal(err)
		}
		sids = append(sids, ses.ID)
	}
	for _, sid := range sids {
		if _, err := e.Prompt(ctx, sid, "go"); err != nil {
			t.Fatal(err)
		}
	}
	stop := make(chan struct{})
	approveAll(e, stop)
	left := map[string]bool{}
	for _, sid := range sids {
		left[sid] = true
	}
	for deadline := time.After(60 * time.Second); len(left) > 0; {
		select {
		case ev := <-ch:
			if ev.Type == event.TurnEnded {
				delete(left, ev.SID)
			}
		case <-deadline:
			t.Fatalf("turns still running: %d", len(left))
		}
	}
	close(stop)
	before := agentsAlive(t)
	fmt.Printf("reference agent processes alive with 10 sessions idle for 0 s: %d\n", before)
	started := time.Now()
	for time.Since(started) < 90*time.Second && agentsAlive(t) > 0 {
		time.Sleep(time.Second)
	}
	after := agentsAlive(t)
	fmt.Printf("reference agent processes alive after the idle time (1 min, %d s waited): %d\n", int(time.Since(started).Seconds()), after)
	if before != 10 || after != 0 {
		t.Fatalf("alive before %d (want 10) and after %d (want 0)", before, after)
	}
}
