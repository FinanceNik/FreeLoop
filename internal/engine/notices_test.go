package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// GUI-008: approvals, failures and finished turns become notices naming
// the session; muted projects and the settings in config.yaml silence them.
func TestNotices(t *testing.T) {
	ctx := context.Background()
	notices := make(chan Notice, 16)
	e, err := Open(Options{DataDir: t.TempDir(), Secrets: secrets.NewMemory(), Home: t.TempDir(), Dev: true,
		OnNotice: func(n Notice) { notices <- n }})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	next := func() Notice {
		t.Helper()
		select {
		case n := <-notices:
			return n
		case <-time.After(5 * time.Second):
			t.Fatal("no notice")
			return Notice{}
		}
	}
	none := func() {
		t.Helper()
		select {
		case n := <-notices:
			t.Fatalf("unexpected notice %+v", n)
		case <-time.After(200 * time.Millisecond):
		}
	}

	ses, _ := demoSession(t, e, "Fix the parser", "ask")
	e.Prompt(ctx, ses.ID, "go")
	n := next()
	if n.Kind != "approval" || n.SessionID != ses.ID || n.Title != "Approval needed: Fix the parser" || !strings.Contains(n.Body, "freeloop-demo.txt") {
		t.Errorf("approval notice = %+v", n)
	}
	for _, r := range e.PendingPermissions() {
		e.Decide(DecideRequest{RequestID: r.ID, OptionID: "reject_once"})
	}
	if n = next(); n.Kind != "approval" { // the demo's shell call
		t.Fatalf("second notice = %+v", n)
	}
	for _, r := range e.PendingPermissions() {
		e.Decide(DecideRequest{RequestID: r.ID, OptionID: "reject_once"})
	}
	if n = next(); n.Kind != "finished" || n.Body != "Finished" {
		t.Errorf("finished notice = %+v", n)
	}

	// Muted project: nothing.
	if err := e.SetProjectMuted(ctx, ses.ProjectID, true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, ch, func(ev event.Event) bool {
		return ev.Type == event.SessionStatus && strings.Contains(string(ev.Data), "idle")
	})
	e.Prompt(ctx, ses.ID, "again")
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	none()
	e.Cancel(ses.ID)
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.TurnEnded })

	// Finished notices off in config.yaml; approvals still come.
	cfg := e.Config()
	cfg.Notifications = config.Notifications{Enabled: true, Finished: false}
	e.SaveSettings(cfg)
	other, _ := demoSession(t, e, "Other", "accept_edits")
	e.Prompt(ctx, other.ID, "go")
	if n = next(); n.Kind != "approval" || n.SessionID != other.ID {
		t.Fatalf("notice = %+v", n)
	}
	for _, r := range e.PendingPermissions() {
		e.Decide(DecideRequest{RequestID: r.ID, OptionID: "allow_once"})
	}
	waitIdle(t, ch, other.ID)
	none()
	if e.AttentionCount(ctx) != 0 {
		t.Errorf("attention count = %d", e.AttentionCount(ctx))
	}
}
