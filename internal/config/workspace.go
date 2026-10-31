package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WorkspaceFile keeps the main window's panes between launches (GUI-017).
// It changes whenever a pane does, so it is apart from config.yaml, which
// the app watches and reloads.
const WorkspaceFile = "workspace.json"

// Workspace is the main window's layout: how many panes and which session
// each shows ("" for an empty pane), and the active one.
type Workspace struct {
	Layout int      `json:"layout"`
	Panes  []string `json:"panes"`
	Active int      `json:"active"`
}

// LoadWorkspace returns the saved workspace, or one empty pane.
func LoadWorkspace(dir string) Workspace {
	w := Workspace{Layout: 1, Panes: []string{""}}
	b, err := os.ReadFile(filepath.Join(dir, WorkspaceFile))
	if err != nil {
		return w
	}
	var saved Workspace
	if json.Unmarshal(b, &saved) != nil || saved.Layout < 1 || saved.Layout > 4 {
		return w
	}
	return saved
}

// SaveWorkspace records the workspace.
func SaveWorkspace(dir string, w Workspace) error {
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, WorkspaceFile), b, 0o600)
}
