package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// ProjectInfo is a project as the UI sees it.
type ProjectInfo struct {
	session.Project
	// Missing is set when the folder no longer exists; offer to relink.
	Missing bool `json:"missing"`
}

// ListProjects returns every project by name.
func (e *Engine) ListProjects(ctx context.Context) ([]ProjectInfo, error) {
	list, err := e.store.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectInfo, 0, len(list))
	for _, p := range list {
		out = append(out, ProjectInfo{Project: p, Missing: !environment.IsDir(p.Root)})
	}
	return out, nil
}

// localEnv opens a local environment at root for the app's own reads.
func (e *Engine) localEnv(root string, vars map[string]string) (*environment.Local, error) {
	return environment.NewLocal(environment.LocalOptions{
		ID: "local", Root: root, Env: vars, Guard: e.broker.Policy().Guard(root),
	})
}

// AddProject returns the project at root, creating it on first use with
// its git fingerprint (OQ-020).
func (e *Engine) AddProject(ctx context.Context, root string) (session.Project, error) {
	if root == "" {
		return session.Project{}, errors.New("choose a project folder")
	}
	if !environment.IsDir(root) {
		return session.Project{}, fmt.Errorf("%s is not a folder", root)
	}
	root = environment.Canonical(root)
	if p, err := e.store.ProjectByRoot(ctx, root); err == nil {
		return p, nil
	} else if !errors.Is(err, store.ErrProjectNotFound) {
		return session.Project{}, err
	}
	env, err := e.localEnv(root, nil)
	if err != nil {
		return session.Project{}, err
	}
	first, remote := fingerprint(ctx, env)
	return e.store.CreateProject(ctx, session.Project{
		Name: filepath.Base(root), Root: root, FirstCommit: first, Remote: remote, DefaultEnvironmentID: DefaultEnvironmentID,
	})
}

// RelinkProject points a project at its new folder after a move. When the
// project has a fingerprint, the new folder must have the same first
// commit.
func (e *Engine) RelinkProject(ctx context.Context, id, root string) (session.Project, error) {
	p, err := e.store.GetProject(ctx, id)
	if err != nil {
		return session.Project{}, err
	}
	if !environment.IsDir(root) {
		return session.Project{}, fmt.Errorf("%s is not a folder", root)
	}
	root = environment.Canonical(root)
	if other, err := e.store.ProjectByRoot(ctx, root); err == nil && other.ID != id {
		return session.Project{}, fmt.Errorf("%s already belongs to project %s", root, other.Name)
	}
	env, err := e.localEnv(root, nil)
	if err != nil {
		return session.Project{}, err
	}
	first, remote := fingerprint(ctx, env)
	if p.FirstCommit != "" && first != p.FirstCommit {
		return session.Project{}, fmt.Errorf("%s is a different repository than %s (its first commit differs)", root, p.Name)
	}
	p.Root, p.FirstCommit = root, first
	if remote != "" {
		p.Remote = remote
	}
	if err := e.store.UpdateProject(ctx, p); err != nil {
		return session.Project{}, err
	}
	return p, nil
}
