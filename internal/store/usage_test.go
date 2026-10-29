package store

import (
	"context"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// PRV-008: model calls' tokens add up per day and model; a call that does
// not name its model counts for the session's model at the time; old
// transcripts are counted on open; a deleted session's usage stays.
func TestUsagePerDay(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	s, err := Open(Options{Dir: dir, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin", ProviderID: "p", Model: "m1"})
	b, _ := s.CreateSession(ctx, session.Session{Title: "b", HarnessID: "acp", ProviderID: "", Model: ""})
	use := func(sid string, in, out int64, provider, model string) {
		s.Append(ctx, sid, event.UsageUpdate, raw(event.UsageData{InputTokens: in, OutputTokens: out, CacheReadTokens: 1, ProviderID: provider, Model: model}))
	}
	use(a.ID, 100, 10, "p", "m1")
	use(a.ID, 200, 20, "p", "m1")
	s.Append(ctx, a.ID, event.ModelChanged, raw(event.ModelChangedData{ProviderID: "q", Model: "m2"}))
	use(a.ID, 1000, 0, "", "") // an older event without its model
	use(b.ID, 7, 7, "", "")
	now = now.Add(24 * time.Hour)
	use(a.ID, 5, 5, "p", "m1")
	s.Append(ctx, a.ID, event.UsageUpdate, raw(event.UsageData{}))
	s.Close()

	// Forget the usage, as a database from before it would be.
	s, _ = Open(Options{Dir: dir})
	s.db.Exec(`DELETE FROM usage`)
	s.db.Exec(`DELETE FROM search`)
	s.db.Exec(`UPDATE sessions SET indexed_seq = 0`)
	s.Close()
	s, _ = Open(Options{Dir: dir})
	defer s.Close()
	s.indexed.Wait()
	if err := s.DeleteSession(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.UsageSince(ctx, "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	want := []Usage{
		{Day: "2026-10-02", ProviderID: "p", Model: "m1", InputTokens: 5, OutputTokens: 5, CacheReadTokens: 1, Calls: 1},
		{Day: "2026-10-01", ProviderID: "", Model: "", InputTokens: 7, OutputTokens: 7, CacheReadTokens: 1, Calls: 1},
		{Day: "2026-10-01", ProviderID: "p", Model: "m1", InputTokens: 300, OutputTokens: 30, CacheReadTokens: 2, Calls: 2},
		{Day: "2026-10-01", ProviderID: "q", Model: "m2", InputTokens: 1000, CacheReadTokens: 1, Calls: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("usage = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("usage[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if later, _ := s.UsageSince(ctx, "2026-10-02"); len(later) != 1 {
		t.Errorf("since the 2nd = %+v", later)
	}
}
