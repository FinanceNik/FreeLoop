package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"go.yaml.in/yaml/v3"
)

// ProvidersFileName holds provider configurations; keys are Keychain
// references only (SEC-004).
const ProvidersFileName = "providers.yaml"

// Provider is one configured model endpoint.
type Provider struct {
	ID           string            `yaml:"id" json:"id"`
	Name         string            `yaml:"name" json:"name"`
	Type         string            `yaml:"type" json:"type"`
	BaseURL      string            `yaml:"baseUrl,omitempty" json:"baseUrl"`
	APIKey       string            `yaml:"apiKey,omitempty" json:"-"` // keychain:// reference
	DefaultModel string            `yaml:"defaultModel,omitempty" json:"defaultModel"`
	Headers      map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	// Preset names the OpenAI-compatible endpoint preset.
	Preset string `yaml:"preset,omitempty" json:"preset,omitempty"`
	// Models holds per-model settings the user chose.
	Models map[string]ModelSettings `yaml:"models,omitempty" json:"models,omitempty"`
}

// ModelSettings are a user's choices for one model (PRV-003, PRV-005).
type ModelSettings struct {
	// Tools overrides detected tool-calling support.
	Tools *bool `yaml:"tools,omitempty" json:"tools,omitempty"`
	// Favorite models are listed first; hidden ones only on request.
	Favorite bool `yaml:"favorite,omitempty" json:"favorite,omitempty"`
	Hidden   bool `yaml:"hidden,omitempty" json:"hidden,omitempty"`
}

// IsZero reports whether s holds no choice, so it need not be stored.
func (s ModelSettings) IsZero() bool { return s.Tools == nil && !s.Favorite && !s.Hidden }

type providersFile struct {
	Version   int        `yaml:"version"`
	Providers []Provider `yaml:"providers"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// LoadProviders reads providers.yaml from dir. A missing file is an empty
// list; entries with problems are skipped and reported.
func LoadProviders(dir string) ([]Provider, []Issue, error) {
	path := filepath.Join(dir, ProvidersFileName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	var f providersFile
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, []Issue{{File: path, Message: err.Error()}}, nil
	}
	var out []Provider
	var issues []Issue
	seen := map[string]bool{}
	for i, p := range f.Providers {
		field := fmt.Sprintf("providers[%d]", i)
		switch {
		case !idPattern.MatchString(p.ID):
			issues = append(issues, Issue{File: path, Field: field + ".id", Message: fmt.Sprintf("%q must be lowercase letters, digits and dashes", p.ID)})
		case seen[p.ID]:
			issues = append(issues, Issue{File: path, Field: field + ".id", Message: fmt.Sprintf("duplicate id %q", p.ID)})
		case p.Type == "":
			issues = append(issues, Issue{File: path, Field: field + ".type", Message: "type is required"})
		default:
			seen[p.ID] = true
			out = append(out, p)
		}
	}
	return out, issues, nil
}

// SaveProviders replaces providers.yaml atomically.
func SaveProviders(dir string, providers []Provider) error {
	b, err := yaml.Marshal(providersFile{Version: 1, Providers: providers})
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, ProvidersFileName), b)
}

func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
