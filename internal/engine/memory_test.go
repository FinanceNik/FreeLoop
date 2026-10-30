package engine

import (
	"context"
	goruntime "runtime"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// NFR-PERF-004, the engine's share: ten sessions that each ran a turn and
// went idle keep little heap and no goroutines per session. The whole app,
// window included, is measured by scripts/memcheck.sh (make memcheck).
func TestIdleSessionsMemory(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	withScript(e, "p", scripted.Step{Text: "Done."})
	ch, cancel := e.Subscribe()
	defer cancel()

	measure := func() (uint64, int) {
		var m goruntime.MemStats
		for range 3 {
			goruntime.GC()
		}
		goruntime.ReadMemStats(&m)
		return m.HeapInuse, goruntime.NumGoroutine()
	}
	heap0, gor0 := measure()
	var ids []string
	for range 10 {
		ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "p", Model: "m"})
		if err != nil {
			t.Fatal(err)
		}
		e.Prompt(ctx, ses.ID, "hello")
		ids = append(ids, ses.ID)
	}
	waitIdle(t, ch, ids...)
	// Background work after a turn (git state, the search index) settles.
	time.Sleep(500 * time.Millisecond)
	heap1, gor1 := measure()

	grown := int64(heap1) - int64(heap0)
	t.Logf("ten idle sessions: heap %+.1f MB, goroutines %+d", float64(grown)/(1<<20), gor1-gor0)
	if grown > 32<<20 {
		t.Errorf("ten idle sessions keep %.1f MB of heap; the budget is 32 MB", float64(grown)/(1<<20))
	}
	if gor1-gor0 > 4*len(ids) {
		t.Errorf("ten idle sessions keep %d goroutines", gor1-gor0)
	}
}
