package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func TestRecentUsageFollowsTheTranscript(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	s, err := Open(Options{Dir: t.TempDir(), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ses, err := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "builtin", ProviderID: "p", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	use := func(at time.Time, input int64) {
		t.Helper()
		now = at
		b, _ := json.Marshal(event.UsageData{InputTokens: input, OutputTokens: 1})
		if _, err := s.Append(ctx, ses.ID, event.UsageUpdate, b); err != nil {
			t.Fatal(err)
		}
	}
	start := now
	use(start.Add(time.Minute), 10)
	use(start.Add(2*time.Minute), 20)
	use(start.Add(11*time.Minute), 5)
	got, err := s.RecentUsage(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Input != 30 || got[0].Calls != 2 || got[1].Input != 5 || got[0].ProviderID != "p" || got[0].Model != "m" {
		t.Fatalf("buckets: %+v", got)
	}
	// Asking again adds nothing; what happened since is added.
	use(start.Add(12*time.Minute), 7)
	got, err = s.RecentUsage(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Input != 30 || got[1].Input != 12 {
		t.Fatalf("buckets after more: %+v", got)
	}
	// Old buckets go.
	now = start.Add(100 * time.Hour)
	use(now, 1)
	got, err = s.RecentUsage(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Input != 1 {
		t.Fatalf("buckets after a long time: %+v", got)
	}
}

// PRV-008: an hour's cache writes are kept apart, calls over a model's long-prompt
// size count under their own name, and an adjustment adds tokens and no call.
func TestUsageKeepsWhatPricesDependOn(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	ses, err := s.CreateSession(ctx, session.Session{Title: "a", HarnessID: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []event.UsageData{
		{InputTokens: 2, CacheWriteTokens: 50, CacheWrite1hTokens: 30, Model: "claude-haiku-5-5"},
		{InputTokens: 2, CacheReadTokens: 100_000, Model: "claude-haiku-5-5"},
		{OutputTokens: 7, CacheReadTokens: 200_000, Model: "claude-haiku-5-5", Adjustment: true},
	} {
		b, _ := json.Marshal(d)
		if _, err := s.Append(ctx, ses.ID, event.UsageUpdate, b); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := s.RecentUsage(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Fatalf("recent: %+v", recent)
	}
	base, long := recent[0], recent[1]
	if base.Model != "claude-haiku-5-5" || base.CacheWrite != 50 || base.CacheWrite1h != 30 || base.Output != 7 || base.Calls != 1 {
		t.Errorf("base: %+v", base)
	}
	if long.Model != "claude-haiku-5-5 (long prompt)" || long.CacheRead != 100_000 || long.Calls != 1 {
		t.Errorf("long: %+v", long)
	}
	s.Close()
	s, err = Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.indexed.Wait()
	day, err := s.UsageSince(ctx, "2026-11-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(day) != 2 || day[0].CacheWrite1hTokens != 30 || day[0].Calls != 1 || day[1].Model != "claude-haiku-5-5 (long prompt)" || day[1].Calls != 1 {
		t.Errorf("per day: %+v", day)
	}
}

// A database of an earlier build gets the columns without a higher schema version,
// so that build can still open it, and what it writes without them still works.
func TestAddedColumnsLeaveTheSchemaVersionAlone(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range addedColumns {
		if _, err := s.db.Exec(`ALTER TABLE ` + c.table + ` DROP COLUMN ` + c.column); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	for range 2 { // and again, with them there
		s, err = Open(Options{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		var version int
		if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != len(migrations) {
			t.Errorf("schema version %d, %v; want %d", version, err, len(migrations))
		}
		if _, err := s.db.Exec(`INSERT INTO usage (day, session_id, provider_id, model, input, output, cache_read, cache_write, calls)
			VALUES ('2026-11-03', 's', 'p', 'm', 1, 1, 1, 1, 1) ON CONFLICT DO NOTHING`); err != nil {
			t.Errorf("an earlier build's write: %v", err)
		}
		if _, err := s.UsageSince(context.Background(), "2026-11-01"); err != nil {
			t.Errorf("reading usage: %v", err)
		}
		s.Close()
	}
}
