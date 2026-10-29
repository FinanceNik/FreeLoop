package engine

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// GUI-002: five sessions prompting at once are all listed, and resolved from
// the list alone; a session whose turn failed is listed with its error until
// it is dismissed.
func TestAttentionListsWaitingAndFailedSessions(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()

	var sids []string
	for range 5 {
		ses, _ := demoSession(t, e, "demo", "ask")
		sids = append(sids, ses.ID)
		if _, err := e.Prompt(ctx, ses.ID, "go"); err != nil {
			t.Fatal(err)
		}
	}
	for len(e.PendingPermissions()) < 5 {
		waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	}

	broken, root := demoSession(t, e, "broken", "ask")
	os.RemoveAll(root)
	if _, err := e.Prompt(ctx, broken.ID, "go"); err == nil {
		t.Fatal("prompt in a missing folder succeeded")
	}

	items, err := e.Attention(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 {
		t.Fatalf("attention has %d items, want 6", len(items))
	}
	for _, it := range items[:5] {
		if len(it.Requests) != 1 || it.Requests[0].SessionID != it.Session.ID || it.ProjectName == "" {
			t.Errorf("waiting item = %+v", it)
		}
	}
	last := items[5]
	if last.Session.ID != broken.ID || last.Session.Status != session.StatusError || !strings.Contains(last.Error, "missing") {
		t.Errorf("failed item = %+v", last)
	}

	// Resolve every waiting session from the list alone; the demo then
	// asks once more (the shell call), which the list shows next.
	ended := map[string]bool{}
	for len(ended) < len(sids) {
		items, _ := e.Attention(ctx)
		for _, it := range items {
			for _, r := range it.Requests {
				e.Decide(DecideRequest{RequestID: r.ID, OptionID: "reject_once"})
			}
		}
		ev := waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.TurnEnded || ev.Type == event.PermissionRequested })
		if ev.Type == event.TurnEnded {
			ended[ev.SID] = true
		}
	}
	if err := e.DismissError(ctx, broken.ID); err != nil {
		t.Fatal(err)
	}
	if items, _ = e.Attention(ctx); len(items) != 0 {
		t.Errorf("attention after resolving = %+v", items)
	}
	if err := e.DismissError(ctx, broken.ID); err == nil {
		t.Error("dismissing a session not in error succeeded")
	}
}
