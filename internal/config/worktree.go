package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"go.yaml.in/yaml/v3"
)

// WorktreeFileName is a project's worktree scripts in .freeloop/ (SES-016).
const WorktreeFileName = "worktree.yaml"

// WorktreeScripts prepare and run a session's own worktree (SES-016): files
// copied from the main checkout (an .env the repository ignores), a command
// run once when the worktree is made, one that runs the app (a dev
// server), and one run before the worktree is removed. A trusted project's
// .freeloop/worktree.yaml sets them, and the app's own settings for the
// project override them field by field.
type WorktreeScripts struct {
	Copy    []string `yaml:"copy,omitempty" json:"copy"`
	Setup   string   `yaml:"setup,omitempty" json:"setup"`
	Run     string   `yaml:"run,omitempty" json:"run"`
	Archive string   `yaml:"archive,omitempty" json:"archive"`
}

// Empty reports whether the scripts do nothing.
func (w WorktreeScripts) Empty() bool {
	return len(w.Copy) == 0 && strings.TrimSpace(w.Setup) == "" && strings.TrimSpace(w.Run) == "" && strings.TrimSpace(w.Archive) == ""
}

// Over returns w with every field over sets replacing w's.
func (w WorktreeScripts) Over(over WorktreeScripts) WorktreeScripts {
	if len(over.Copy) > 0 {
		w.Copy = over.Copy
	}
	for _, f := range []struct{ dst, src *string }{{&w.Setup, &over.Setup}, {&w.Run, &over.Run}, {&w.Archive, &over.Archive}} {
		if strings.TrimSpace(*f.src) != "" {
			*f.dst = *f.src
		}
	}
	return w
}

// Validate reports the first entry that cannot work: a copy path must be
// relative and stay inside the checkout.
func (w WorktreeScripts) Validate() error {
	for _, c := range w.Copy {
		clean := path.Clean(strings.TrimSpace(c))
		if c == "" || path.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("copy: %q must be a path inside the project", c)
		}
	}
	return nil
}

// ParseWorktreeScripts reads a .freeloop/worktree.yaml.
func ParseWorktreeScripts(b []byte) (WorktreeScripts, error) {
	var w WorktreeScripts
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&w); err != nil && !errors.Is(err, io.EOF) {
		return WorktreeScripts{}, err
	}
	return w, w.Validate()
}
