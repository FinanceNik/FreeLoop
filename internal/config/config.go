// Package config locates the app data directory and loads config.yaml
// (SET-001). This is the M0 skeleton: one file, strict decoding, validation
// that reports issues instead of failing. Further files, JSON Schema
// validation and hot reload arrive in M1.
package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"
)

// HomeEnv overrides the data directory, for development and tests.
const HomeEnv = "FREELOOP_HOME"

// FileName is the general settings file inside the data directory.
const FileName = "config.yaml"

// DataDir returns the app data directory: $FREELOOP_HOME if set, otherwise
// the OS user config directory plus "freeloop" (on macOS,
// ~/Library/Application Support/freeloop). It creates the directory.
func DataDir() (string, error) {
	dir := os.Getenv(HomeEnv)
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
	// Hooks run for every session (EXT-005).
	Hooks []Hook `yaml:"hooks,omitempty" json:"hooks"`
	// Network holds the local-only switch (NFR-OFF-003).
	Network Network `yaml:"network,omitempty" json:"network"`
	// Updates holds the opt-in update check (SEC-009, OQ-015).
	Updates Updates `yaml:"updates,omitempty" json:"updates"`
	// Filters are the sidebar's saved filters (SES-013).
	Filters []Filter `yaml:"filters,omitempty" json:"filters"`
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
	// this Mac.
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
}

// Permissions holds global permission settings.
type Permissions struct {
	// AllowBypass makes bypass mode selectable (SEC-001). Off by default.
	AllowBypass bool `yaml:"allowBypass" json:"allowBypass"`
}

// UI holds interface settings.
type UI struct {
	// Theme is "system", "light" or "dark" (GUI-011).
	Theme string `yaml:"theme" json:"theme"`
	// FontSize is the base font size in points, 11 to 20.
	FontSize int `yaml:"fontSize,omitempty" json:"fontSize"`
	// Onboarded is set once the first-run guide was finished or skipped
	// (GUI-010).
	Onboarded bool `yaml:"onboarded,omitempty" json:"onboarded"`
}

// Default returns the configuration used when config.yaml is missing.
func Default() Config {
	return Config{Version: 1, UI: UI{Theme: "system", FontSize: 14}, Notifications: Notifications{Enabled: true, Finished: true}}
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
	if c.UI.FontSize == 0 {
		c.UI.FontSize = def.UI.FontSize
	} else if c.UI.FontSize < 11 || c.UI.FontSize > 20 {
		issues = append(issues, Issue{Field: "ui.fontSize", Message: fmt.Sprintf("%d is outside 11 to 20", c.UI.FontSize)})
		c.UI.FontSize = def.UI.FontSize
	}
	var priceIssues, hookIssues []Issue
	c.Prices, priceIssues = validatePrices(c.Prices)
	c.Hooks, hookIssues = validateHooks(c.Hooks, "hooks")
	return append(append(issues, priceIssues...), hookIssues...)
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
	return writeAtomic(filepath.Join(dir, FileName), b)
}

// Issue is a problem found in a config file. Issues are shown in the UI; the
// app keeps running on defaults.
type Issue struct {
	File  string `json:"file"`
	Field string `json:"field,omitempty"`
	// Line is the line in File the problem is on, when known.
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// Load reads config.yaml from dir. A missing file yields Default(). A file
// that cannot be parsed yields Default() plus an issue; invalid fields fall
// back to their defaults with an issue each. The error is reserved for I/O
// failures other than a missing file.
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
	for i := range issues {
		issues[i].File = path
	}
	return cfg, issues, nil
}

func parse(b []byte) (Config, []Issue) {
	cfg := Default()
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Default(), []Issue{{Message: err.Error()}}
	}
	issues := cfg.Validate()
	return cfg, issues
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
