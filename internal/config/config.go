// Package config reads and writes the app's files in its data folder
// (SET-001): config.yaml, providers, profiles, environments, rules and the
// rest, with strict decoding and validation that reports issues instead of
// failing.
package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// HomeEnv names the environment variable that overrides the data directory, for
// development and tests. The app reads it, unless it is a shipped build; this
// package does not.
const HomeEnv = "FREELOOP_HOME"

// FileName is the general settings file inside the data directory.
const FileName = "config.yaml"

// DataDir returns the app data directory: home if not empty, otherwise the OS
// user config directory plus "freeloop" (on macOS,
// ~/Library/Application Support/freeloop). It creates the directory.
func DataDir(home string) (string, error) {
	dir := home
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("config: %w", err)
		}
		dir = filepath.Join(base, "freeloop")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("config: %w", err)
	}
	// MkdirAll leaves an existing folder as it is, and this one is what keeps
	// other users out of the socket.
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("config: %w", err)
	}
	return dir, nil
}

// Config is the content of config.yaml.
type Config struct {
	Version     int         `yaml:"version" json:"version"`
	UI          UI          `yaml:"ui" json:"ui"`
	Permissions Permissions `yaml:"permissions" json:"permissions"`
	Defaults    Defaults    `yaml:"defaults,omitempty" json:"defaults"`
	// Notifications are macOS notifications for sessions that need the
	// user while the app is in the background (GUI-008).
	Notifications Notifications `yaml:"notifications" json:"notifications"`
	// Skills holds the global skill switches (SKL-002).
	Skills Skills `yaml:"skills,omitempty" json:"skills"`
	// Prices estimate what sessions cost (PRV-008).
	Prices []Price `yaml:"prices,omitempty" json:"prices"`
	// Budgets warn when estimated spending reaches a share of an amount (PRV-017).
	Budgets []Budget `yaml:"budgets,omitempty" json:"budgets"`
	// Fallbacks name, per agent (a harness ID, "builtin" for the built-in one), the
	// profile a stuck session is offered to continue on (HRN-020).
	Fallbacks map[string]string `yaml:"fallbacks,omitempty" json:"fallbacks"`
	// Hooks run for every session (EXT-005).
	Hooks []Hook `yaml:"hooks,omitempty" json:"hooks"`
	// OptimizerApprovals record the token optimizer files the user approved, by
	// ID, with a mark only the app can compute (HRN-019).
	OptimizerApprovals map[string]string `yaml:"optimizerApprovals,omitempty" json:"optimizerApprovals"`
	// WorkflowApprovals record the workflow files the user approved, by ID, with a
	// mark only the app can compute (WFL-001).
	WorkflowApprovals map[string]string `yaml:"workflowApprovals,omitempty" json:"workflowApprovals"`
	// CommandApprovals record what the user approved in the app's files, with
	// marks only the app can compute (SET-005): hooks, agent presets, allow rules,
	// environments, permissions.allowBypass and a profile's mode.
	CommandApprovals []string `yaml:"commandApprovals,omitempty" json:"commandApprovals"`
	// Network holds the local-only switch (NFR-OFF-003).
	Network Network `yaml:"network,omitempty" json:"network"`
	// Updates holds the opt-in update check (SEC-009, OQ-015).
	Updates Updates `yaml:"updates,omitempty" json:"updates"`
	// Filters are the sidebar's saved filters (SES-013).
	Filters []Filter `yaml:"filters,omitempty" json:"filters"`
	// Web holds the web_search endpoint (HRN-017, OQ-025).
	Web Web `yaml:"web,omitempty" json:"web"`
	// MCP holds the server registry to browse (MCP-009).
	MCP MCPSettings `yaml:"mcp,omitempty" json:"mcp"`
	// ModelSets are the user's model sets, and WorkflowModelSets the one each
	// workflow uses, by their IDs (WFL-013).
	ModelSets         []ModelSet        `yaml:"modelSets,omitempty" json:"modelSets"`
	WorkflowModelSets map[string]string `yaml:"workflowModelSets,omitempty" json:"workflowModelSets"`
}

// MCPSettings are settings for MCP as a whole; the servers are in mcp.yaml.
type MCPSettings struct {
	// RegistryURL is a server registry to browse; empty, nothing connects.
	RegistryURL string `yaml:"registryURL,omitempty" json:"registryURL"`
}

// Web settings for the built-in harness's web_search tool.
type Web struct {
	// SearchURL is the search endpoint, with {query} where the query goes;
	// web_search is offered only when it is set.
	SearchURL string `yaml:"searchURL,omitempty" json:"searchURL"`
	// SearchHeader names the header that carries the key the Keychain
	// holds for it, such as X-Subscription-Token; empty for none.
	SearchHeader string `yaml:"searchHeader,omitempty" json:"searchHeader"`
}

// Filter is a named set of sidebar filters (SES-013); a session shows when
// it matches every one that is set.
type Filter struct {
	Name   string   `yaml:"name" json:"name"`
	Query  string   `yaml:"query,omitempty" json:"query"`
	Status string   `yaml:"status,omitempty" json:"status"`
	Tags   []string `yaml:"tags,omitempty" json:"tags"`
}

// Updates settings.
type Updates struct {
	// Check looks for a newer release on GitHub once a day; off unless the
	// user turns it on.
	Check bool `yaml:"check,omitempty" json:"check"`
}

// Network settings.
type Network struct {
	// LocalOnly blocks every connection the app makes to anything but
	// this computer.
	LocalOnly bool `yaml:"localOnly,omitempty" json:"localOnly"`
}

// Skills lists skills turned off everywhere; every other discovered skill
// is on unless a project or profile says otherwise.
type Skills struct {
	Disabled []string `yaml:"disabled,omitempty" json:"disabled"`
}

// Notifications holds notification settings.
type Notifications struct {
	// Enabled sends notifications at all; projects can be muted one by one.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Finished also notifies when a turn ends without needing the user.
	Finished bool `yaml:"finished" json:"finished"`
}

// Defaults preselect the new-session form (SET-004).
type Defaults struct {
	ProviderID    string `yaml:"providerId,omitempty" json:"providerId"`
	Model         string `yaml:"model,omitempty" json:"model"`
	EnvironmentID string `yaml:"environmentId,omitempty" json:"environmentId"`
	// ProfileID is used when the project has no default profile.
	ProfileID string `yaml:"profileId,omitempty" json:"profileId"`
	// Optimizer is the token optimizer of new built-in sessions whose profile
	// does not name one (HRN-019); none when empty.
	Optimizer string `yaml:"optimizer,omitempty" json:"optimizer"`
	// Workflow is the workflow of new sessions whose profile does not name one
	// (WFL-002); none when empty.
	Workflow string `yaml:"workflow,omitempty" json:"workflow"`
}

// Permissions holds global permission settings.
type Permissions struct {
	// AllowBypass makes bypass mode selectable (SEC-001). Off by default.
	AllowBypass bool `yaml:"allowBypass" json:"allowBypass"`
}

// BypassFingerprint identifies permissions.allowBypass being on.
func BypassFingerprint() string {
	return commandFingerprint("setting", "permissions.allowBypass")
}

// AddressFingerprint identifies an address a setting sends things to. A file's
// address gets nothing until the user approved exactly this.
func AddressFingerprint(setting, address string) string {
	return commandFingerprint("address", []string{setting, address})
}

// UpdatesFingerprint identifies updates.check being on.
func UpdatesFingerprint() string {
	return commandFingerprint("setting", "updates.check")
}

// DefaultOptimizerFingerprint identifies defaults.optimizer naming id.
func DefaultOptimizerFingerprint(id string) string {
	return commandFingerprint("default optimizer", id)
}

// UI holds interface settings.
type UI struct {
	// Theme is "system", "light" or "dark" (GUI-011).
	Theme string `yaml:"theme" json:"theme"`
	// Language is the interface's: "system", "en", "de" or "nl" (GUI-048). System
	// follows the languages the computer prefers, falling back to English.
	Language string `yaml:"language,omitempty" json:"language"`
	// FontSize is the base font size in points, 11 to 20.
	FontSize int `yaml:"fontSize,omitempty" json:"fontSize"`
	// TextFont is the family name for the interface and messages, CodeFont
	// the one for code, tool output, diffs and the terminal (GUI-025). Empty
	// means the app's own.
	TextFont string `yaml:"textFont,omitempty" json:"textFont"`
	CodeFont string `yaml:"codeFont,omitempty" json:"codeFont"`
	// Shortcuts are the user's changes to the app's keyboard shortcuts, by name
	// (SET-006). A name left out keeps its default.
	Shortcuts map[string]string `yaml:"shortcuts,omitempty" json:"shortcuts"`
	// SourceMarks are the user's choices for the mark that shows what a session runs on,
	// by provider or agent ID (GUI-034). One left out keeps its own.
	SourceMarks map[string]SourceMark `yaml:"sourceMarks,omitempty" json:"sourceMarks"`
	// Onboarded is set once the first-run guide was finished or skipped
	// (GUI-010).
	Onboarded bool `yaml:"onboarded,omitempty" json:"onboarded"`
}

// Languages are the values ui.language takes (GUI-048).
var Languages = []string{"system", "en", "de", "nl"}

// SourceMark is a provider's or agent's mark: a colour slot from 1 to SourceSlots, and
// one or two letters. Zero and empty mean its own.
type SourceMark struct {
	Slot    int    `yaml:"slot,omitempty" json:"slot"`
	Letters string `yaml:"letters,omitempty" json:"letters"`
}

// SourceSlots is how many colours marks have (--source-1 and so on in app.css).
const SourceSlots = 8

var sourceLetters = regexp.MustCompile(`^[\p{L}\p{N}]{1,2}$`)

// CleanSourceMarks keeps what a mark can be; a file can say anything, and these reach the
// window. A bad part is dropped and reported, and the good part kept.
func CleanSourceMarks(in map[string]SourceMark) (map[string]SourceMark, []Issue) {
	if len(in) == 0 {
		return in, nil
	}
	out := map[string]SourceMark{}
	var issues []Issue
	for id, m := range in {
		if m.Slot < 0 || m.Slot > SourceSlots {
			issues = append(issues, Issue{Field: "ui.sourceMarks." + id + ".slot", Message: fmt.Sprintf("%d is not a colour from 1 to %d", m.Slot, SourceSlots)})
			m.Slot = 0
		}
		if m.Letters != "" && !sourceLetters.MatchString(m.Letters) {
			issues = append(issues, Issue{Field: "ui.sourceMarks." + id + ".letters", Message: fmt.Sprintf("%q is not one or two letters or digits", m.Letters)})
			m.Letters = ""
		}
		if m != (SourceMark{}) {
			out[id] = m
		}
	}
	return out, issues
}

// Default returns the configuration used when config.yaml is missing.
func Default() Config {
	return Config{Version: 1, UI: UI{Theme: "system", Language: "system", FontSize: 14}, Notifications: Notifications{Enabled: true, Finished: true}}
}

// Validate fixes invalid fields to their defaults and reports each.
func (c *Config) Validate() []Issue {
	var issues []Issue
	def := Default()
	if c.Version != 1 {
		issues = append(issues, Issue{Field: "version", Message: fmt.Sprintf("unsupported version %d; this build reads version 1", c.Version)})
		c.Version = def.Version
	}
	if !slices.Contains([]string{"system", "light", "dark"}, c.UI.Theme) {
		issues = append(issues, Issue{Field: "ui.theme", Message: fmt.Sprintf("%q is not one of system, light, dark", c.UI.Theme)})
		c.UI.Theme = def.UI.Theme
	}
	if c.UI.Language == "" {
		c.UI.Language = def.UI.Language
	} else if !slices.Contains(Languages, c.UI.Language) {
		issues = append(issues, Issue{Field: "ui.language", Message: fmt.Sprintf("%q is not one of %s", c.UI.Language, strings.Join(Languages, ", "))})
		c.UI.Language = def.UI.Language
	}
	if c.UI.FontSize == 0 {
		c.UI.FontSize = def.UI.FontSize
	} else if c.UI.FontSize < 11 || c.UI.FontSize > 20 {
		issues = append(issues, Issue{Field: "ui.fontSize", Message: fmt.Sprintf("%d is outside 11 to 20", c.UI.FontSize)})
		c.UI.FontSize = def.UI.FontSize
	}
	issues = append(issues, validateFont("ui.textFont", &c.UI.TextFont)...)
	issues = append(issues, validateFont("ui.codeFont", &c.UI.CodeFont)...)
	var priceIssues, hookIssues []Issue
	c.Prices, priceIssues = validatePrices(c.Prices)
	var budgetIssues []Issue
	c.Budgets, budgetIssues = validateBudgets(c.Budgets)
	priceIssues = append(priceIssues, budgetIssues...)
	c.Hooks, hookIssues = validateHooks(c.Hooks, "hooks")
	var setIssues []Issue
	c.ModelSets, setIssues = validateModelSets(c.ModelSets)
	setIssues = append(setIssues, validateWorkflowModelSets(c.WorkflowModelSets, c.ModelSets)...)
	return append(append(append(issues, priceIssues...), hookIssues...), setIssues...)
}

// Save writes config.yaml.
func Save(dir string, c Config) error {
	if issues := c.Validate(); len(issues) > 0 {
		return fmt.Errorf("config: %s: %s", issues[0].Field, issues[0].Message)
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, FileName), b); err != nil {
		return err
	}
	keepLastGood(dir, b)
	return nil
}

// Issue is a problem found in a config file. Issues are shown in the UI; the
// app keeps running on defaults.
type Issue struct {
	File  string `json:"file"`
	Field string `json:"field,omitempty"`
	// Line is the line in File the problem is on, when known.
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
	// Unreadable marks a file that could not be parsed at all, as opposed
	// to one with a setting ignored.
	Unreadable bool `json:"unreadable,omitempty"`
	// Approval marks something waiting for the user's approval in Settings.
	Approval bool `json:"approval,omitempty"`
}

// Unreadable reports whether any issue says its file could not be parsed.
func Unreadable(issues []Issue) bool {
	for _, is := range issues {
		if is.Unreadable {
			return true
		}
	}
	return false
}

// Load reads config.yaml from dir. A missing file yields Default(). A file
// that cannot be parsed yields Default() plus an unreadable issue; an
// unknown key is reported and the rest of the file kept, so a typo does not
// turn off local-only mode or hooks; invalid fields fall back to their
// defaults with an issue each. The error is reserved for I/O failures
// other than a missing file.
func Load(dir string) (Config, []Issue, error) {
	path := filepath.Join(dir, FileName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil, nil
	}
	if err != nil {
		return Default(), nil, fmt.Errorf("config: %w", err)
	}
	cfg, issues := parse(b)
	if !Unreadable(issues) {
		keepLastGood(dir, b)
	} else if good, err := os.ReadFile(filepath.Join(dir, LastGoodFileName)); err == nil {
		// Start with the settings as they last were, local-only mode and
		// hooks included, rather than with defaults that turn them off.
		if prev, prevIssues := parse(good); !Unreadable(prevIssues) {
			cfg = prev
			issues[0].Message += "; using the settings from the last time it could be read until it is fixed"
		}
	}
	for i := range issues {
		issues[i].File = path
	}
	return cfg, issues, nil
}

// LastGoodFileName is a copy of the last config.yaml that could be read,
// which the app starts with when config.yaml cannot be. The watcher does
// not look at it.
const LastGoodFileName = ".config-last-good.yaml"

// keepLastGood records b as the last readable config.yaml, when it changed.
func keepLastGood(dir string, b []byte) {
	path := filepath.Join(dir, LastGoodFileName)
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return
	}
	if err := writeAtomic(path, b); err != nil {
		log.Printf("config: keep a copy of %s: %v", FileName, err)
	}
}

func parse(b []byte) (Config, []Issue) {
	cfg := Default()
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var issues []Issue
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		// Read again without the strict check: an unknown key is reported
		// and ignored; only a file that still cannot be read is dropped.
		cfg = Default()
		if err2 := yaml.Unmarshal(b, &cfg); err2 != nil {
			return Default(), []Issue{{Message: err2.Error(), Unreadable: true}}
		}
		issues = append(issues, Issue{Message: err.Error() + " (ignored; the rest of the file applies)"})
	}
	// Saving would drop what this version does not know.
	if cfg.Version > 1 {
		return cfg, append(issues, Issue{Message: fmt.Sprintf("version %d was written by a newer version of Freeloop, so this one does not overwrite it", cfg.Version), Unreadable: true})
	}
	marks, markIssues := CleanSourceMarks(cfg.UI.SourceMarks)
	cfg.UI.SourceMarks = marks
	issues = append(issues, markIssues...)
	return cfg, append(issues, cfg.Validate()...)
}

// SocketFileName is the headless API's Unix socket in the data folder
// (EXT-004); the folder is the user's alone (0700), so only they reach it.
const SocketFileName = "freeloop.sock"

// SocketPath is the headless API's socket. A Unix socket's path must be
// short (104 bytes on macOS); for a data folder too deep for that, the
// socket goes in the user's own temporary folder ($TMPDIR, which only they
// can enter), named after the data folder.
func SocketPath(dir string) string {
	p := filepath.Join(dir, SocketFileName)
	if len(p) < 100 {
		return p
	}
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(os.TempDir(), "freeloop-"+hex.EncodeToString(sum[:6])+".sock")
}

// ClearSocket removes a socket file a previous run left behind; anything
// that is not a socket is left alone.
func ClearSocket(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s is not a socket", path)
	}
	return os.Remove(path)
}
