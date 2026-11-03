package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	// Watch is a folder whose changes restart the agent for idle sessions
	// on their next message: the source of a custom harness (HRN-011).
	Watch string `yaml:"watch,omitempty" json:"watch"`
	// Skills says how the agent is given the skills enabled in the app
	// (SKL-008): SkillsClaudePlugin passes them as a local plugin folder in
	// session/new's _meta.claudeCode.options.plugins; empty, not at all.
	Skills string `yaml:"skills,omitempty" json:"skills"`
	// HideText lists starts of messages the agent sends for machines, not
	// people, which the conversation leaves out.
	HideText []string `yaml:"hideText,omitempty" json:"hideText"`
	// MissingFileIsEmpty answers an agent's read of a file that is not there
	// with no content instead of the protocol's "not found", for an agent that
	// cannot recognize that error when it reads a file before creating it.
	MissingFileIsEmpty bool `yaml:"missingFileIsEmpty,omitempty" json:"missingFileIsEmpty"`
	// Builtin is set for presets that ship with the app.
	Builtin bool `yaml:"-" json:"builtin"`
}

// ClaudeCodePresetID is the preset for Claude Code (HRN-006).
const ClaudeCodePresetID = "claude-code"

// TakesCommandHook reports an agent that runs a plugin's hook before each shell
// command, which is how a token optimizer reaches an agent that runs its commands
// itself (HRN-019).
func (p HarnessPreset) TakesCommandHook() bool { return p.Skills == SkillsClaudePlugin }

// SkillsClaudePlugin is the HarnessPreset.Skills way of agents built on the
// Claude Agent SDK, whose ACP adapter passes _meta.claudeCode.options on to
// the SDK.
const SkillsClaudePlugin = "claude-plugin"

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
		Description: "Use your Claude subscription or Anthropic account. Freeloop starts Claude Code, which signs in by itself.",
		Command:     "npx",
		Args:        []string{"-y", "@agentclientprotocol/claude-agent-acp@0.99.1"},
		Modes:       map[string]string{"ask": "default", "accept_edits": "acceptEdits", "plan": "plan", "bypass": "bypassPermissions"},
		Notes:       "Needs Node.js. Sign in once by running claude in a terminal; Freeloop never sees or stores the login.",
		Skills:      SkillsClaudePlugin,
		Builtin:     true,
	}, {
		ID:          "gemini-cli",
		Name:        "Gemini CLI",
		Description: "Google's agent, with a Gemini API key.",
		Command:     "npx",
		Args:        []string{"-y", "@google/gemini-cli@0.66.0", "--acp"},
		Modes:       askingModes("default", "plan"),
		Notes:       "Needs Node.js and a Gemini API key: add GEMINI_API_KEY as a secret of the session's environment. Sign-in with a personal Google account no longer works for this agent.",
		HideText:    []string{"[MODE_UPDATE]"},
		// 0.66.0 gets the error as a plain object, so its check for one that
		// means "not found" fails, and it refuses to create the file.
		MissingFileIsEmpty: true,
		Builtin:            true,
	}, {
		ID:          "codex",
		Name:        "Codex CLI",
		Description: "OpenAI's agent, with an API key or your ChatGPT plan.",
		Command:     "npx",
		Args:        []string{"-y", "@agentclientprotocol/codex-acp@2.5.0"},
		// Its read-only mode is the one that asks before an edit, and the app
		// decides what to allow; its default asks only about what it finds
		// unsafe.
		Modes:   askingModes("read-only", "read-only"),
		Env:     map[string]string{"NO_BROWSER": "1", "DEFAULT_AUTH_REQUEST": `{"methodId":"api-key"}`},
		Notes:   "Needs Node.js. Add CODEX_API_KEY or OPENAI_API_KEY as a secret of the session's environment, or sign in once by running codex login in a terminal; Freeloop never sees or stores the login.",
		Builtin: true,
	}, {
		ID:          "goose",
		Name:        "goose",
		Description: "Block's open-source agent, on the provider you set up for it.",
		Command:     "goose",
		Args:        []string{"acp"},
		// Its default approves every tool call. It has no read-only mode that
		// keeps reading: plan is chat, with no tools at all.
		Modes:   askingModes("approve", "chat"),
		Notes:   "Install it (brew install block-goose-cli; tested with 1.57.0) and choose its provider and key once by running goose configure in a terminal.",
		Builtin: true,
	}, {
		ID:          "opencode",
		Name:        "OpenCode",
		Description: "The open-source agent, on the providers you set up for it.",
		Command:     "npx",
		Args:        []string{"-y", "opencode-ai@1.19.5", "acp"},
		Modes:       askingModes("build", "plan"),
		// Its default allows every tool without asking.
		Env:     map[string]string{"OPENCODE_PERMISSION": `{"*":"ask"}`},
		Notes:   "Needs Node.js. Add a provider once by running opencode auth login in a terminal, or add its API key as a secret of the session's environment. Without one it uses OpenCode's own hosted free models, so your prompts and code go there.",
		Builtin: true,
	}}
}

// askingModes maps the app's modes to an agent's: plan to its read-only mode,
// and all the others to the mode in which it asks before acting, so every
// action reaches the permission broker, which applies the session's real mode
// (SEC-003). An agent left to approve its own edits would act out of the
// app's sight, whichever folder they land in.
func askingModes(asks, readOnly string) map[string]string {
	return map[string]string{"ask": asks, "accept_edits": asks, "plan": readOnly, "bypass": asks}
}

// Fingerprint identifies what a preset runs. One from a file runs only after the
// user approved exactly this.
func (p HarnessPreset) Fingerprint() string {
	return commandFingerprint("preset", p)
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
	case p.Skills != "" && p.Skills != SkillsClaudePlugin:
		return fieldErr("skills", "%q is not a way to give the agent skills; use %s or leave it out", p.Skills, SkillsClaudePlugin)
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

// SavePreset writes presets/harnesses/<id>.yaml.
func SavePreset(dir string, p HarnessPreset) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, PresetsDir), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return writeAtomic(filepath.Join(dir, PresetsDir, p.ID+".yaml"), b)
}

// DirFingerprint describes the files below dir by path, size and
// modification time, skipping dot folders, so a poll can tell a change;
// "" when dir cannot be read.
func DirFingerprint(dir string) string {
	var sb strings.Builder
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && p != dir && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if fi, err := d.Info(); err == nil && fi.Mode().IsRegular() {
			fmt.Fprintf(&sb, "%s/%d/%d\n", p, fi.Size(), fi.ModTime().UnixNano())
		}
		return nil
	})
	return sb.String()
}
