// Package config locates the app data directory and loads config.yaml
// (SET-001). This is the M0 skeleton: one file, strict decoding, validation
// that reports issues instead of failing. Further files, JSON Schema
// validation and hot reload arrive in M1.
package config

import (
	"bytes"
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
}

// Defaults preselect the new-session form (SET-004).
type Defaults struct {
	ProviderID    string `yaml:"providerId,omitempty" json:"providerId"`
	Model         string `yaml:"model,omitempty" json:"model"`
	EnvironmentID string `yaml:"environmentId,omitempty" json:"environmentId"`
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
}

// Default returns the configuration used when config.yaml is missing.
func Default() Config {
	return Config{Version: 1, UI: UI{Theme: "system", FontSize: 14}}
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
	return issues
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
	File    string `json:"file"`
	Field   string `json:"field,omitempty"`
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
