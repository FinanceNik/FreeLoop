package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// A store with one session that has a title, a model change and a reply.
func seeded(t *testing.T, dir string) string {
	t.Helper()
	ctx := context.Background()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ses, err := s.CreateSession(ctx, session.Session{Title: "First", HarnessID: "builtin", ProjectRoot: "/tmp/p", ProviderID: "a", Model: "small", Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	for typ, data := range map[event.Type]any{
		event.SessionInfoUpdate: map[string]string{"title": "Named"},
		event.ModelChanged:      map[string]string{"providerId": "b", "model": "large"},
	} {
		b, _ := json.Marshal(data)
		if _, err := s.Append(ctx, ses.ID, typ, b); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return ses.ID
}

func checkRebuilt(t *testing.T, dir, sid string) {
	t.Helper()
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Rebuilt() == "" {
		t.Error("nothing says it was rebuilt")
	}
	got, err := s.GetSession(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Named" || got.Model != "large" || got.ProviderID != "b" || got.ProjectRoot != "/tmp/p" || got.LastSeq != 2 {
		t.Errorf("rebuilt session: %+v", got)
	}
	evs, err := s.Events(context.Background(), sid, 0)
	if err != nil || len(evs) != 2 {
		t.Errorf("events: %d, %v", len(evs), err)
	}
}

func TestRebuildsAMissingDatabase(t *testing.T) {
	dir := t.TempDir()
	sid := seeded(t, dir)
	for _, name := range []string{"freeloop.db", "freeloop.db-wal", "freeloop.db-shm", dbBackupName} {
		os.Remove(filepath.Join(dir, name))
	}
	checkRebuilt(t, dir, sid)
}

func TestRebuildsADamagedDatabase(t *testing.T) {
	dir := t.TempDir()
	sid := seeded(t, dir)
	db := filepath.Join(dir, "freeloop.db")
	os.Remove(db + "-wal")
	os.Remove(db + "-shm")
	os.Remove(filepath.Join(dir, dbBackupName))
	if err := os.WriteFile(db, []byte(strings.Repeat("not a database at all ", 400)), 0o600); err != nil {
		t.Fatal(err)
	}
	checkRebuilt(t, dir, sid)
	kept, _ := filepath.Glob(db + ".damaged-*")
	if len(kept) == 0 {
		t.Error("the damaged file was not kept")
	}
}

// Without meta.json a session can still be read.
func TestRebuildsASessionWithoutItsRecord(t *testing.T) {
	dir := t.TempDir()
	sid := seeded(t, dir)
	os.Remove(filepath.Join(dir, "sessions", sid, metaName))
	for _, name := range []string{"freeloop.db", "freeloop.db-wal", "freeloop.db-shm", dbBackupName} {
		os.Remove(filepath.Join(dir, name))
	}
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetSession(context.Background(), sid)
	if err != nil || got.Title != "Named" || got.Model != "large" {
		t.Errorf("session: %+v, %v", got, err)
	}
}

// A database from a newer version is not damage: it stays as it is.
func TestANewerDatabaseIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	seeded(t, dir)
	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 9999`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := Open(Options{Dir: dir}); err == nil {
		t.Error("opened a database from a newer version")
	}
	kept, _ := filepath.Glob(filepath.Join(dir, "freeloop.db.damaged-*"))
	if len(kept) != 0 {
		t.Errorf("set aside: %v", kept)
	}
}

// An event that cannot be written is reported, and leaves nothing for the next one
// to merge into.
func TestAFailedWriteLeavesWholeLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	tr, err := openTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	ev := event.Event{Seq: 1, Type: event.Notice, Data: json.RawMessage(`{"message":"one"}`)}
	if err := tr.write(ev); err != nil {
		t.Fatal(err)
	}
	good := tr.f
	ro, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	tr.f = ro
	ev.Seq = 2
	if err := tr.write(ev); err == nil || !isWriteErr(err) {
		t.Errorf("a failed write gave %v", err)
	}
	ro.Close()
	tr.f = good
	if err := tr.write(ev); err != nil {
		t.Fatal(err)
	}
	tr.close()
	evs, err := readEvents(path, 0)
	if err != nil || len(evs) != 2 {
		t.Errorf("events: %d, %v", len(evs), err)
	}
}

func isWriteErr(err error) bool { return errors.Is(err, ErrWrite) }
