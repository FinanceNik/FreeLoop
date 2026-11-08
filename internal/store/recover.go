package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// The database is an index: the transcripts hold what happened. So losing it
// must not lose sessions. A copy is kept once a day, each session records what its
// transcript cannot say in meta.json, and a database that is gone or damaged is
// set aside, restored from the copy and brought up to date from the transcripts.

const (
	dbBackupName = "freeloop.db.bak"
	metaName     = "meta.json"
	// dbBackupEvery is how old the copy of the database may get.
	dbBackupEvery = 24 * time.Hour
)

// damagedDB reports an error that means the file is not a usable database, not
// that it could not be reached: a full disk or a read-only folder must not cost
// the user the file, and neither must a newer version's database.
func damagedDB(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"malformed", "not a database", "sqlite_corrupt", "sqlite_notadb", "integrity check failed"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// openDB opens the database, brings it up to date and checks it.
func openDB(dbPath string) (*sql.DB, error) {
	if f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0o600); err == nil {
		f.Close()
	}
	// SQLite gives the WAL and shared-memory files the database's mode, and the
	// database holds session titles, commands and search text.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Chmod(dbPath+suffix, 0o600)
	}
	dsn := "file:" + uriPath(dbPath) +
		"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	var check string
	if err := db.QueryRow(`PRAGMA quick_check(1)`).Scan(&check); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: check db: %w", err)
	}
	if check != "ok" {
		db.Close()
		return nil, fmt.Errorf("store: integrity check failed: %s", check)
	}
	return db, nil
}

// uriPath writes a file's path for an SQLite URI, in which % starts an escape, ?
// the parameters and # a fragment. A Windows user's folder can have # in its name
// (NFR-XPLAT-003); backslashes and drive letters SQLite takes as they are.
func uriPath(p string) string {
	return strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(p)
}

// openOrRestore opens the database. It reports why it had to rebuild, if it did.
func openOrRestore(dir, dbPath string) (*sql.DB, string, error) {
	missing := false
	if fi, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) || (err == nil && fi.Size() == 0) {
		missing = transcriptsExist(dir)
		if missing {
			restoreCopy(dir, dbPath)
		}
	}
	db, err := openDB(dbPath)
	if damagedDB(err) {
		kept := fmt.Sprintf("%s.damaged-%d", dbPath, time.Now().Unix())
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Rename(dbPath+suffix, kept+suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, "", fmt.Errorf("store: set aside the damaged database: %w", err)
			}
		}
		log.Printf("store: the database was damaged (%v); kept as %s, rebuilding it", err, filepath.Base(kept))
		restoreCopy(dir, dbPath)
		if db, err = openDB(dbPath); damagedDB(err) {
			// The copy is damaged too. The transcripts are still there.
			os.Rename(dbPath, kept+".copy")
			db, err = openDB(dbPath)
		}
		if err != nil {
			return nil, "", err
		}
		return db, "The session list could not be read, so it was rebuilt from the transcripts. The damaged file is kept as " + filepath.Base(kept) + ".", nil
	}
	if err != nil {
		return nil, "", err
	}
	if missing {
		return db, "The session list was missing, so it was rebuilt from the transcripts.", nil
	}
	return db, "", nil
}

func transcriptsExist(dir string) bool {
	entries, err := os.ReadDir(filepath.Join(dir, "sessions"))
	return err == nil && len(entries) > 0
}

// restoreCopy puts the daily copy of the database in place, if there is one.
func restoreCopy(dir, dbPath string) {
	src, err := os.Open(filepath.Join(dir, dbBackupName))
	if err != nil {
		return
	}
	defer src.Close()
	dst, err := os.OpenFile(dbPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		log.Printf("store: restore the copy of the database: %v", err)
		dst.Truncate(0)
	}
}

// backupDB refreshes the daily copy. VACUUM INTO writes a consistent copy while
// sessions keep writing.
func (s *Store) backupDB(ctx context.Context) {
	path := filepath.Join(s.dir, dbBackupName)
	if fi, err := os.Stat(path); err == nil && s.now().Sub(fi.ModTime()) < dbBackupEvery {
		return
	}
	tmp := path + ".tmp"
	os.Remove(tmp)
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		os.Remove(tmp)
		if ctx.Err() == nil {
			log.Printf("store: copy the database: %v", err)
		}
		return
	}
	os.Chmod(tmp, 0o600)
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("store: copy the database: %v", err)
		os.Remove(tmp)
	}
}

// writeMeta records a session's settings beside its transcript, for a rebuild.
func (s *Store) writeMeta(ses session.Session) {
	b, err := json.Marshal(ses)
	if err != nil {
		return
	}
	path := filepath.Join(s.sessionDir(ses.ID), metaName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		log.Printf("store: record session %s: %v", ses.ID, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("store: record session %s: %v", ses.ID, err)
		os.Remove(tmp)
	}
}

// backfillMeta records the settings of sessions from before meta.json.
func (s *Store) backfillMeta(ctx context.Context) {
	list, err := s.ListSessions(ctx)
	if err != nil {
		return
	}
	for _, ses := range list {
		if _, err := os.Stat(filepath.Join(s.sessionDir(ses.ID), metaName)); errors.Is(err, os.ErrNotExist) {
			s.writeMeta(ses)
		}
	}
}

// rebuildSessions brings the database up to date from the session folders.
func (s *Store) rebuildSessions(ctx context.Context) error {
	entries, err := os.ReadDir(filepath.Join(s.dir, "sessions"))
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		if err := s.rebuildSession(ctx, de.Name()); err != nil {
			log.Printf("store: rebuild session %s: %v", de.Name(), err)
		}
	}
	return nil
}

func (s *Store) rebuildSession(ctx context.Context, sid string) error {
	events, err := readEvents(filepath.Join(s.sessionDir(sid), "events.jsonl"), 0)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ses := session.Session{ID: sid, HarnessID: "builtin", Mode: "ask", Status: session.StatusIdle}
	if b, err := os.ReadFile(filepath.Join(s.sessionDir(sid), metaName)); err == nil {
		if err := json.Unmarshal(b, &ses); err != nil {
			log.Printf("store: read %s of session %s: %v", metaName, sid, err)
		}
		ses.ID = sid
	}
	foldEvents(&ses, events)
	if ses.ProjectID != "" {
		name := cmp.Or(filepath.Base(ses.ProjectRoot), ses.ProjectID)
		// A rebuilt project is not trusted: trust is the user's to give again.
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO projects (id, name, root, created_at) VALUES (?, ?, ?, ?)`,
			ses.ProjectID, name, ses.ProjectRoot, ses.CreatedAt.UnixMilli()); err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, title, status, harness_id, project_id, project_root, environment_id, base_commit, branch, provider_id, model, mode, profile_id, external_id, worktree, worktree_branch, parent_id, tags, optimizer, effort, port_base, last_seq, created_at, updated_at, imported_from, workflow)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET title = excluded.title, status = excluded.status, branch = excluded.branch, provider_id = excluded.provider_id,
		   model = excluded.model, mode = excluded.mode, tags = excluded.tags, optimizer = excluded.optimizer, workflow = excluded.workflow, effort = excluded.effort, last_seq = excluded.last_seq, updated_at = excluded.updated_at, imported_from = excluded.imported_from`,
		ses.ID, ses.Title, ses.Status, ses.HarnessID, ses.ProjectID, ses.ProjectRoot, ses.EnvironmentID, ses.BaseCommit, ses.Branch,
		ses.ProviderID, ses.Model, ses.Mode, ses.ProfileID, ses.ExternalID, ses.Worktree, ses.WorktreeBranch, ses.ParentID, strings.Join(ses.Tags, " "), ses.Optimizer, ses.Effort, ses.PortBase,
		ses.LastSeq, ses.CreatedAt.UnixMilli(), ses.UpdatedAt.UnixMilli(), ses.ImportedFrom, ses.Workflow)
	return err
}

// foldEvents applies what a transcript says about its session, as appending each
// of them did.
func foldEvents(ses *session.Session, events []event.Event) {
	firstUser := ""
	for _, ev := range events {
		var d struct {
			Status        string   `json:"status"`
			CurrentModeID string   `json:"currentModeId"`
			Model         string   `json:"model"`
			ProviderID    string   `json:"providerId"`
			Branch        string   `json:"branch"`
			Title         string   `json:"title"`
			Tags          []string `json:"tags"`
			Optimizer     string   `json:"optimizer"`
			Workflow      string   `json:"workflow"`
			Effort        string   `json:"effort"`
			Harness       string   `json:"harness"`
			Source        string   `json:"source"`
			Content       struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		json.Unmarshal(ev.Data, &d)
		switch ev.Type {
		case event.SessionStatus:
			if d.Status != "" {
				ses.Status = session.Status(d.Status)
			}
		case event.CurrentModeUpdate:
			if d.CurrentModeID != "" {
				ses.Mode = d.CurrentModeID
			}
		case event.ModelChanged:
			if d.Model != "" {
				ses.Model, ses.ProviderID = d.Model, cmp.Or(d.ProviderID, ses.ProviderID)
			}
		case event.UsageUpdate:
			// Without meta.json, this is what names the model.
			if ses.Model == "" {
				ses.Model, ses.ProviderID = d.Model, cmp.Or(d.ProviderID, ses.ProviderID)
			}
		case event.GitState:
			ses.Branch = d.Branch
		case event.SessionInfoUpdate:
			if d.Title != "" {
				ses.Title = d.Title
			}
		case event.TagsChanged:
			ses.Tags = d.Tags
		case event.OptimizerChanged:
			ses.Optimizer = d.Optimizer
		case event.WorkflowChanged:
			ses.Workflow = d.Workflow
		case event.EffortChanged:
			ses.Effort = d.Effort
		case event.SessionImported:
			ses.ImportedFrom = d.Harness + ":" + d.Source
		case event.UserMessageChunk:
			firstUser = cmp.Or(firstUser, d.Content.Text)
		}
		ses.LastSeq = ev.Seq
		if ses.CreatedAt.IsZero() {
			ses.CreatedAt = ev.TS
		}
		ses.UpdatedAt = ev.TS
	}
	if ses.Title == "" {
		ses.Title = cmp.Or(firstLine(firstUser), "Recovered session")
	}
	now := time.Now().UTC()
	if ses.CreatedAt.IsZero() {
		ses.CreatedAt = now
	}
	if ses.UpdatedAt.IsZero() {
		ses.UpdatedAt = ses.CreatedAt
	}
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(s); len(r) > 80 {
		return string(r[:80])
	}
	return s
}
