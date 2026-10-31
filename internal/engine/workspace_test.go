package engine

import (
	"slices"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// GUI-017: the workspace survives a restart; a malformed one is refused.
func TestWorkspaceIsKept(t *testing.T) {
	dir := t.TempDir()
	e := openTest(t, dir)
	if w := e.Workspace(); w.Layout != 1 || len(w.Panes) != 1 {
		t.Errorf("fresh = %+v", w)
	}
	if err := e.SaveWorkspace(config.Workspace{Layout: 3, Panes: []string{"a", "", "b"}, Active: 2}); err != nil {
		t.Fatal(err)
	}
	if err := e.SaveWorkspace(config.Workspace{Layout: 5, Panes: make([]string, 5)}); err == nil {
		t.Error("five panes were accepted")
	}
	e.Close()
	e = openTest(t, dir)
	defer e.Close()
	if w := e.Workspace(); w.Layout != 3 || !slices.Equal(w.Panes, []string{"a", "", "b"}) || w.Active != 2 {
		t.Errorf("after a restart = %+v", w)
	}
}
