package engine

import (
	"context"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Engine.Events takes a limit and never returns more than it (NFR-PERF-002, issue 041);
// a reader that follows a page's last seq gets every event once.
func TestEventsNeverReturnMoreThanTheLimit(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "paged", "")
	for i := 0; i < 700; i++ {
		if _, err := e.AppendEvent(ctx, AppendEventRequest{SessionID: ses.ID, Type: event.Notice, Data: []byte(`{"message":"n"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := e.Events(ctx, ses.ID, 0, 0)
	if err != nil || len(all) < 700 {
		t.Fatalf("%d events, %v", len(all), err)
	}
	for _, limit := range []int{1, 100, 257, 5000} {
		got, err := e.Events(ctx, ses.ID, 0, limit)
		if err != nil || len(got) > limit || len(got) != min(limit, len(all)) {
			t.Errorf("limit %d gave %d events, %v", limit, len(got), err)
		}
	}
	var paged []event.Event
	for after := int64(0); ; {
		page, err := e.Events(ctx, ses.ID, after, 300)
		if err != nil || len(page) > 300 {
			t.Fatalf("a page of %d, %v", len(page), err)
		}
		if len(page) == 0 {
			break
		}
		paged = append(paged, page...)
		after = page[len(page)-1].Seq
	}
	if len(paged) != len(all) {
		t.Errorf("pages held %d events, the whole read %d", len(paged), len(all))
	}
}
