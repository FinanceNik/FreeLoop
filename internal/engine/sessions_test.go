package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

func TestTitleFrom(t *testing.T) {
	for in, want := range map[string]string{
		"\n  fix the   login bug\nmore detail": "fix the login bug",
		"":                                     "",
		strings.Repeat("word ", 30):            "word word word word word word word word word word word word…",
	} {
		if got := titleFrom(in); got != want {
			t.Errorf("titleFrom(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAutoTitleRenameArchiveDelete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	ch, unsub := e.Subscribe()
	defer unsub()
	ses, _ := demoSession(t, e, "", "ask")

	e.Prompt(ctx, ses.ID, "Add a failing test for the parser\nthen fix it")
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	got, _ := e.store.GetSession(ctx, ses.ID)
	if got.Title != "Add a failing test for the parser" {
		t.Errorf("auto title = %q", got.Title)
	}
	e.Prompt(ctx, ses.ID, "second message")
	if got, _ := e.store.GetSession(ctx, ses.ID); got.Title != "Add a failing test for the parser" {
		t.Errorf("title changed by a later prompt: %q", got.Title)
	}

	// Archiving stops the waiting turn.
	if err := e.ArchiveSession(ctx, ses.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.store.GetSession(ctx, ses.ID); got.Status != session.StatusArchived {
		t.Errorf("status = %s", got.Status)
	}
	if len(e.PendingPermissions()) != 0 {
		t.Error("archived session still waits for approval")
	}
	e.ArchiveSession(ctx, ses.ID, false)
	if got, _ := e.store.GetSession(ctx, ses.ID); got.Status != session.StatusIdle {
		t.Errorf("status after unarchive = %s", got.Status)
	}
	if err := e.RenameSession(ctx, ses.ID, "  Parser fix  "); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.store.GetSession(ctx, ses.ID); got.Title != "Parser fix" {
		t.Errorf("renamed title = %q", got.Title)
	}

	if err := e.DeleteSession(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sessions", ses.ID)); !os.IsNotExist(err) {
		t.Errorf("session files still on disk: %v", err)
	}
	if list, _ := e.ListSessions(ctx); len(list) != 0 {
		t.Errorf("sessions after delete = %+v", list)
	}
}
