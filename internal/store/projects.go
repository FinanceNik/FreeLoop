package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/session"
)

// ErrProjectNotFound is returned for an unknown project.
var ErrProjectNotFound = errors.New("store: project not found")

// ErrProjectInUse is returned for deleting a project that sessions still belong to.
var ErrProjectInUse = errors.New("store: project still has sessions")

const projectColumns = `id, name, root, first_commit, remote, default_env, default_profile, trusted, muted, disabled_skills, worktree_default, created_at`

func scanProject(r scanner) (session.Project, error) {
	var p session.Project
	var created int64
	var skills string
	if err := r.Scan(&p.ID, &p.Name, &p.Root, &p.FirstCommit, &p.Remote, &p.DefaultEnvironmentID, &p.DefaultProfileID, &p.Trusted, &p.Muted, &skills, &p.WorktreeDefault, &created); err != nil {
		return session.Project{}, err
	}
	p.DisabledSkills = []string{}
	if skills != "" {
		p.DisabledSkills = strings.Split(skills, ",")
	}
	p.CreatedAt = time.UnixMilli(created).UTC()
	return p, nil
}

// CreateProject inserts a project built from tmpl.
func (s *Store) CreateProject(ctx context.Context, tmpl session.Project) (session.Project, error) {
	p := tmpl
	now := s.now().UTC()
	p.ID, p.CreatedAt = session.NewProjectID(now), now
	_, err := s.db.ExecContext(ctx, `INSERT INTO projects (`+projectColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Root, p.FirstCommit, p.Remote, p.DefaultEnvironmentID, p.DefaultProfileID, p.Trusted, p.Muted, strings.Join(p.DisabledSkills, ","), p.WorktreeDefault, now.UnixMilli())
	if err != nil {
		return session.Project{}, fmt.Errorf("store: insert project: %w", err)
	}
	return p, nil
}

// ListProjects returns all projects by name.
func (s *Store) ListProjects(ctx context.Context) ([]session.Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list projects: %w", err)
	}
	defer rows.Close()
	out := []session.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) projectWhere(ctx context.Context, where string, arg any) (session.Project, error) {
	p, err := scanProject(s.db.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return session.Project{}, ErrProjectNotFound
	}
	return p, err
}

// GetProject returns one project.
func (s *Store) GetProject(ctx context.Context, id string) (session.Project, error) {
	return s.projectWhere(ctx, `id = ?`, id)
}

// ProjectByRoot returns the project at root.
func (s *Store) ProjectByRoot(ctx context.Context, root string) (session.Project, error) {
	return s.projectWhere(ctx, `root = ?`, root)
}

// UpdateProject saves a project's name, root, fingerprint and default
// environment. Sessions follow a new root.
func (s *Store) UpdateProject(ctx context.Context, p session.Project) error {
	old, err := s.GetProject(ctx, p.ID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET name = ?, root = ?, first_commit = ?, remote = ?, default_env = ?, default_profile = ?, trusted = ?, muted = ?, disabled_skills = ?, worktree_default = ? WHERE id = ?`,
		p.Name, p.Root, p.FirstCommit, p.Remote, p.DefaultEnvironmentID, p.DefaultProfileID, p.Trusted, p.Muted, strings.Join(p.DisabledSkills, ","), p.WorktreeDefault, p.ID); err != nil {
		return fmt.Errorf("store: update project: %w", err)
	}
	if old.Root != p.Root {
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET project_root = ? WHERE project_id = ?`, p.Root, p.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteProject forgets a project that no session belongs to any more.
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE project_id = ?`, id).Scan(&n); err != nil {
		return fmt.Errorf("store: delete project: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w (%s, %d)", ErrProjectInUse, id, n)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete project: %w", err)
	}
	return nil
}

// ProjectScripts returns the worktree scripts set in the app for a
// project, as JSON, "" when none (SES-016).
func (s *Store) ProjectScripts(ctx context.Context, id string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT worktree_scripts FROM projects WHERE id = ?`, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrProjectNotFound
	}
	return v, err
}

// SetProjectScripts records a project's worktree scripts, as JSON.
func (s *Store) SetProjectScripts(ctx context.Context, id, scripts string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET worktree_scripts = ? WHERE id = ?`, scripts, id)
	if err != nil {
		return fmt.Errorf("store: project scripts: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// PortBases returns the port blocks of the sessions that are not archived.
func (s *Store) PortBases(ctx context.Context) ([]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT port_base FROM sessions WHERE port_base > 0 AND status != 'archived'`)
	if err != nil {
		return nil, fmt.Errorf("store: port bases: %w", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
