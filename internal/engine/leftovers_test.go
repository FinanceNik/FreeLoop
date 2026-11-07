//go:build darwin

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/platform/darwin"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// NFR-REL-005: what a crash left running in a data folder is stopped when the engine
// opens that folder again, before anything of its own starts.
func TestOpenStopsWhatACrashLeftRunning(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { environment.ProcessStarted = nil })
	open := func() *Engine {
		e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), ProcessStarted: darwin.ProcessStart, WatchInterval: -1, ScheduleInterval: -1})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	e := open()
	env, err := e.localEnv(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	left, err := env.Spawn(context.Background(), environment.SpawnSpec{Argv: []string{"sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	// A crash: the engine is not closed and stops nothing; a new one opens the folder.
	e.store.Close()
	e2 := open()
	defer e2.Close()
	done := make(chan error, 1)
	go func() { done <- left.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("the command the crash left running is still running")
	}
}
