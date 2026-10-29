package config

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// PresetsDir holds ACP agent presets (HRN-005, HRN-006): one YAML file per
// external harness. Adding an agent that speaks ACP needs no Go.
const PresetsDir = "presets/harnesses"

// HarnessPreset describes an external agent the generic ACP client runs.
type HarnessPreset struct {
	ID          string `yaml:"-" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description"`
	// Command and Args start the agent; it is run through the login shell
	// so npx and similar resolve.
	Command string   `yaml:"command" json:"command"`
	Args    []string `yaml:"args,omitempty" json:"args"`
	// Env is added to the agent's environment. Credentials never go here
	// for a subscription agent (HRN-008): it signs in on its own.
	Env map[string]string `yaml:"env,omitempty" json:"env"`
	// Modes maps the app's permission modes to the agent's session modes,
	// so changing the mode in the app also changes it in the agent.
	Modes map[string]string `yaml:"modes,omitempty" json:"modes"`
	// Notes are shown where the preset is chosen, such as how to sign in.
	Notes string `yaml:"notes,omitempty" json:"notes"`
	// Builtin is set for presets that ship with the app.
	Builtin bool `yaml:"-" json:"builtin"`
}

// ClaudeCodePresetID is the preset for Claude Code (HRN-006).
const ClaudeCodePresetID = "claude-code"

// BuiltinPresets ship with the app; a file with the same ID replaces one.
// The Claude Code preset runs the ACP adapter for Claude Code (first
// published by Zed as @zed-industries/claude-code-acp, now
// @agentclientprotocol/claude-agent-acp), pinned to a version, which drives
// the unmodified Claude Code SDK; Claude Code signs in by itself and the app
// passes it no credentials (OQ-005, HRN-008).
func BuiltinPresets() []HarnessPreset {
	return []HarnessPreset{{
		ID:          ClaudeCodePresetID,
		Name:        "Claude Code",
		Description: "Claude Code through its ACP adapter, with your Claude subscription or Anthropic account.",
		Command:     "npx",
		Args:        []string{"-y", "@agentclientprotocol/claude-agent-acp@0.99.1"},
		Modes:       map[string]string{"ask": "default", "accept_edits": "acceptEdits", "plan": "plan", "bypass": "bypassPermissions"},
		Notes:       "Needs Node.js. Sign in once with the claude command in a terminal; Freeloop never sees or stores the login.",
		Builtin:     true,
	}}
}

// Validate reports a preset that cannot run.
func (p HarnessPreset) Validate() error {
	switch {
	case !idPattern.MatchString(p.ID) || p.ID == "builtin":
		return fieldErr("id", "%q must be lowercase letters, digits and dashes, and not builtin", p.ID)
	case strings.TrimSpace(p.Name) == "":
		return fieldErr("name", "a name is required")
	case strings.TrimSpace(p.Command) == "":
		return fieldErr("command", "a command is required")
	}
	for k := range p.Env {
		if !varName.MatchString(k) {
			return fieldErr("env", "%q is not a valid variable name", k)
		}
	}
	return nil
}

// LoadPresets returns the built-in presets with those in presets/harnesses/
// on top, sorted by ID.
func LoadPresets(dir string) ([]HarnessPreset, []Issue) {
	out := BuiltinPresets()
	var issues []Issue
	pdir := filepath.Join(dir, PresetsDir)
	entries, err := os.ReadDir(pdir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		issues = append(issues, Issue{File: pdir, Message: err.Error()})
	}
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name(), ".yaml")
		if !ok || de.IsDir() {
			continue
		}
		path := filepath.Join(pdir, de.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		var p HarnessPreset
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if err := dec.Decode(&p); err != nil && !errors.Is(err, io.EOF) {
			issues = append(issues, Issue{File: path, Line: yamlErrorLine(err), Message: err.Error()})
			continue
		}
		p.ID = id
		if err := p.Validate(); err != nil {
			is := Issue{File: path, Message: err.Error()}
			var fe fieldError
			if errors.As(err, &fe) {
				is.Field, is.Message, is.Line = fe.field, fe.msg, fieldLine(b, fe.field)
			}
			issues = append(issues, is)
			continue
		}
		out = slices.DeleteFunc(out, func(x HarnessPreset) bool { return x.ID == id })
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b HarnessPreset) int { return strings.Compare(a.ID, b.ID) })
	return out, issues
}

// PresetProfile is the profile each preset brings, so the agent can be
// chosen in the new-session form without writing a profile first.
func PresetProfile(p HarnessPreset) Profile {
	return Profile{ID: p.ID, Name: p.Name, Description: p.Description, Harness: p.ID}
}
