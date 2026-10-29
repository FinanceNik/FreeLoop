// Package store persists sessions: SQLite for the index and metadata, one
// append-only JSONL file per session for the transcript (SES-002).
//
// An event is written to its transcript with a single write call before
// OnAppend sees it, so an app crash loses at most the event being written.
// Files are fsynced on a short interval and on Close, which bounds what an OS
// crash or power loss can take.
package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// ErrNotFound is returned for an unknown session ID.
var ErrNotFound = errors.New("store: session not found")

// Options configures Open.
type Options struct {
	// Dir is the app data directory; the store owns freeloop.db and sessions/ in it.
	Dir string
	// OnAppend, if set, is called with every appended event after it is written
	// and while the session's lock is held, so calls for one session arrive in
	// seq order. It must not block.
	OnAppend func(event.Event)
	// SyncInterval is how often dirty transcripts are fsynced. Default 200 ms.
	SyncInterval time.Duration
	// Now is the clock; tests replace it. Default time.Now.
	Now func() time.Time
	// Redact, if set, rewrites event data before it is written, to remove
	// known secret values (SEC-004).
	Redact func(json.RawMessage) json.RawMessage
}

// Store is safe for concurrent use.
type Store struct {
	db       *sql.DB
	dir      string
	onAppend func(event.Event)
	now      func() time.Time
	redact   func(json.RawMessage) json.RawMessage

	mu    sync.Mutex // guards files
	files map[string]*transcript

	stop chan struct{}
	done chan struct{}
}

// Open opens or creates the store in opts.Dir.
func Open(opts Options) (*Store, error) {
	if opts.Dir == "" {
		return nil, errors.New("store: Dir is required")
	}
	if err := os.MkdirAll(filepath.Join(opts.Dir, "sessions"), 0o700); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	dsn := "file:" + filepath.Join(opts.Dir, "freeloop.db") +
		"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{
		db:       db,
		dir:      opts.Dir,
		onAppend: opts.OnAppend,
		now:      opts.Now,
		redact:   opts.Redact,
		files:    map[string]*transcript{},
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	if s.now == nil {
		s.now = time.Now
	}
	interval := opts.SyncInterval
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	go s.syncLoop(interval)
	return s, nil
}

// Close fsyncs and closes every open transcript and the database.
func (s *Store) Close() error {
	close(s.stop)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for id, t := range s.files {
		errs = append(errs, t.close())
		delete(s.files, id)
	}
	errs = append(errs, s.db.Close())
	return errors.Join(errs...)
}

// CreateSession inserts a session record built from tmpl, which supplies
// the title, harness, project root, provider, model and mode.
func (s *Store) CreateSession(ctx context.Context, tmpl session.Session) (session.Session, error) {
	now := s.now().UTC()
	ses := tmpl
	ses.ID = session.NewID(now)
	ses.Status = session.StatusIdle
	ses.LastSeq = 0
	ses.CreatedAt, ses.UpdatedAt = now, now
	if err := os.MkdirAll(s.sessionDir(ses.ID), 0o700); err != nil {
		return session.Session{}, fmt.Errorf("store: %w", err)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, title, status, harness_id, project_id, project_root, environment_id, base_commit, branch, provider_id, model, mode, last_seq, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		ses.ID, ses.Title, ses.Status, ses.HarnessID, ses.ProjectID, ses.ProjectRoot, ses.EnvironmentID, ses.BaseCommit, ses.Branch,
		ses.ProviderID, ses.Model, ses.Mode, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return session.Session{}, fmt.Errorf("store: insert session: %w", err)
	}
	return ses, nil
}

// ListSessions returns all sessions, most recently updated first.
func (s *Store) ListSessions(ctx context.Context) ([]session.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	defer rows.Close()
	out := []session.Session{}
	for rows.Next() {
		ses, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ses)
	}
	return out, rows.Err()
}

// GetSession returns one session.
func (s *Store) GetSession(ctx context.Context, id string) (session.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
	ses, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return session.Session{}, ErrNotFound
	}
	return ses, err
}

// Append assigns the next seq to an event, writes it to the session's
// transcript, updates the session record and then calls OnAppend.
func (s *Store) Append(ctx context.Context, sid string, typ event.Type, data json.RawMessage) (event.Event, error) {
	if !typ.Valid() {
		return event.Event{}, fmt.Errorf("store: unknown event type %q", typ)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		data = json.RawMessage(`{}`)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil || compact.Bytes()[0] != '{' {
		return event.Event{}, errors.New("store: event data must be a JSON object")
	}
	data = compact.Bytes()
	if s.redact != nil {
		data = s.redact(data)
	}
	t, err := s.transcript(ctx, sid)
	if err != nil {
		return event.Event{}, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	ev := event.Event{V: event.Version, Seq: t.lastSeq + 1, TS: s.now().UTC(), SID: sid, Type: typ, Data: data}
	if err := t.write(ev); err != nil {
		return event.Event{}, err
	}
	// Streaming chunks skip the index: last_seq is repaired from the
	// transcript on open, and the UI follows chunks live.
	if !chunk(typ) {
		sets, args := `last_seq = ?, updated_at = ?`, []any{ev.Seq, ev.TS.UnixMilli()}
		var d struct {
			Status        string `json:"status"`
			CurrentModeID string `json:"currentModeId"`
			Model         string `json:"model"`
			Branch        string `json:"branch"`
		}
		json.Unmarshal(data, &d)
		switch {
		case typ == event.SessionStatus && d.Status != "":
			sets += `, status = ?`
			args = append(args, d.Status)
		case typ == event.CurrentModeUpdate && d.CurrentModeID != "":
			sets += `, mode = ?`
			args = append(args, d.CurrentModeID)
		case typ == event.ModelChanged && d.Model != "":
			sets += `, model = ?`
			args = append(args, d.Model)
		case typ == event.GitState:
			sets += `, branch = ?`
			args = append(args, d.Branch)
		}
		// The transcript is the source of truth; a failed index update is
		// repaired from it on the next open, so it does not fail the append.
		s.db.ExecContext(context.WithoutCancel(ctx), `UPDATE sessions SET `+sets+` WHERE id = ?`, append(args, sid)...)
	}
	if s.onAppend != nil {
		s.onAppend(ev)
	}
	return ev, nil
}

func chunk(t event.Type) bool {
	return t == event.AgentMessageChunk || t == event.AgentThoughtChunk || t == event.UserMessageChunk
}

// Events returns a session's events with seq greater than afterSeq, in order.
func (s *Store) Events(ctx context.Context, sid string, afterSeq int64) ([]event.Event, error) {
	if _, err := s.GetSession(ctx, sid); err != nil {
		return nil, err
	}
	t, err := s.transcript(ctx, sid)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return readEvents(t.path, afterSeq)
}

// transcript returns the open transcript for sid, opening and recovering it
// on first use.
func (s *Store) transcript(ctx context.Context, sid string) (*transcript, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.files[sid]; ok {
		return t, nil
	}
	ses, err := s.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	t, err := openTranscript(filepath.Join(s.sessionDir(sid), "events.jsonl"))
	if err != nil {
		return nil, err
	}
	if t.lastSeq != ses.LastSeq {
		s.db.ExecContext(ctx, `UPDATE sessions SET last_seq = ? WHERE id = ?`, t.lastSeq, sid)
	}
	s.files[sid] = t
	return t, nil
}

func (s *Store) sessionDir(sid string) string {
	return filepath.Join(s.dir, "sessions", sid)
}

func (s *Store) syncLoop(interval time.Duration) {
	defer close(s.done)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
			s.mu.Lock()
			open := make([]*transcript, 0, len(s.files))
			for _, t := range s.files {
				open = append(open, t)
			}
			s.mu.Unlock()
			for _, t := range open {
				t.syncIfDirty()
			}
		}
	}
}

const sessionColumns = `id, title, status, harness_id, project_id, project_root, environment_id, base_commit, branch, provider_id, model, mode, last_seq, created_at, updated_at`

type scanner interface{ Scan(...any) error }

func scanSession(r scanner) (session.Session, error) {
	var ses session.Session
	var created, updated int64
	if err := r.Scan(&ses.ID, &ses.Title, &ses.Status, &ses.HarnessID, &ses.ProjectID, &ses.ProjectRoot, &ses.EnvironmentID,
		&ses.BaseCommit, &ses.Branch, &ses.ProviderID, &ses.Model, &ses.Mode, &ses.LastSeq, &created, &updated); err != nil {
		return session.Session{}, err
	}
	ses.CreatedAt = time.UnixMilli(created).UTC()
	ses.UpdatedAt = time.UnixMilli(updated).UTC()
	return ses, nil
}

var migrations = []string{
	`CREATE TABLE sessions (
		id         TEXT PRIMARY KEY,
		title      TEXT NOT NULL,
		status     TEXT NOT NULL,
		harness_id TEXT NOT NULL,
		last_seq   INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE INDEX sessions_updated_at ON sessions (updated_at DESC);`,
	`ALTER TABLE sessions ADD COLUMN project_root TEXT NOT NULL DEFAULT '';
	ALTER TABLE sessions ADD COLUMN provider_id TEXT NOT NULL DEFAULT '';
	ALTER TABLE sessions ADD COLUMN model TEXT NOT NULL DEFAULT '';
	ALTER TABLE sessions ADD COLUMN mode TEXT NOT NULL DEFAULT 'ask';`,
	`CREATE TABLE projects (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL,
		root         TEXT NOT NULL UNIQUE,
		first_commit TEXT NOT NULL DEFAULT '',
		remote       TEXT NOT NULL DEFAULT '',
		default_env  TEXT NOT NULL DEFAULT '',
		created_at   INTEGER NOT NULL
	);
	ALTER TABLE sessions ADD COLUMN project_id TEXT NOT NULL DEFAULT '';
	ALTER TABLE sessions ADD COLUMN environment_id TEXT NOT NULL DEFAULT '';
	ALTER TABLE sessions ADD COLUMN base_commit TEXT NOT NULL DEFAULT '';
	ALTER TABLE sessions ADD COLUMN branch TEXT NOT NULL DEFAULT '';`,
}

// migrate applies pending migrations, tracked in PRAGMA user_version.
func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("store: database schema version %d is newer than this build (%d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
