package engine

import (
	"errors"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// Workspace returns the main window's saved panes (GUI-017).
func (e *Engine) Workspace() config.Workspace {
	return config.LoadWorkspace(e.dataDir)
}

// SaveWorkspace records the main window's panes: one to four, each a session
// ID or "".
func (e *Engine) SaveWorkspace(w config.Workspace) error {
	if w.Layout < 1 || w.Layout > 4 || len(w.Panes) != w.Layout {
		return errors.New("a workspace has one to four panes")
	}
	if w.Active < 0 || w.Active >= w.Layout {
		w.Active = 0
	}
	return config.SaveWorkspace(e.dataDir, w)
}
