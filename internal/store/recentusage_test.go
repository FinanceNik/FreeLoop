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
