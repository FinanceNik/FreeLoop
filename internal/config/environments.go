package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/FinanceNik/Freeloop/internal/permission"
)

// EnvironmentsDir holds one YAML file per environment (ENV-002).
const EnvironmentsDir = "environments"

// Environment is one environment's settings. In M1 every environment is
// local: it runs on this Mac in the session's project folder, and several
// projects and sessions can share it.
type Environment struct {
	ID   string `yaml:"-" json:"id"` // the file name without .yaml
	Name string `yaml:"name" json:"name"`
	Kind string `yaml:"kind" json:"kind"`
	// Env is added to the environment of every command.
	Env map[string]string `yaml:"env,omitempty" json:"env"`
	// Secrets maps a variable name to a keychain:// reference; values are
	// resolved when a command starts (ENV-004).
	Secrets map[string]string `yaml:"secrets,omitempty" json:"-"`
	// DisabledTools are not offered to the model in this environment.
	DisabledTools []string `yaml:"disabledTools,omitempty" json:"disabledTools"`
	// DefaultMode is the permission mode new sessions start in.
	DefaultMode string `yaml:"defaultMode,omitempty" json:"defaultMode"`
	// Rules apply to every session in the environment, between the global
	// and the profile rules (SEC-002).
	Rules []permission.Rule `yaml:"rules,omitempty" json:"rules"`
}

var varName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate reports settings that cannot work.
func (e Environment) Validate() error {
	switch {
	case !idPattern.MatchString(e.ID):
		return fmt.Errorf("id %q must be lowercase letters, digits and dashes", e.ID)
	case strings.TrimSpace(e.Name) == "":
		return errors.New("name is required")
	case e.Kind != "local":
		return fmt.Errorf("kind %q is not supported yet; use local", e.Kind)
	case e.DefaultMode != "" && (!permission.Mode(e.DefaultMode).Valid() || e.DefaultMode == string(permission.ModeBypass)):
		return fmt.Errorf("defaultMode %q must be ask, accept_edits or plan", e.DefaultMode)
	}
	for k := range e.Env {
		if !varName.MatchString(k) {
			return fmt.Errorf("env: %q is not a valid variable name", k)
		}
	}
	for i, r := range e.Rules {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}
	for k, ref := range e.Secrets {
		if !varName.MatchString(k) {
			return fmt.Errorf("secrets: %q is not a valid variable name", k)
		}
		if _, ok := e.Env[k]; ok {
			return fmt.Errorf("%s is both a variable and a secret", k)
		}
		if !strings.HasPrefix(ref, "keychain://") {
			return fmt.Errorf("secrets: %s must be a keychain:// reference, never a value", k)
		}
	}
	return nil
}

func environmentsDir(dir string) string { return filepath.Join(dir, EnvironmentsDir) }

// LoadEnvironments reads every environments/*.yaml in dir, sorted by ID.
// Files with problems are skipped and reported.
func LoadEnvironments(dir string) ([]Environment, []Issue, error) {
	entries, err := os.ReadDir(environmentsDir(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	var out []Environment
	var issues []Issue
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name(), ".yaml")
		if !ok || de.IsDir() {
			continue
		}
		path := filepath.Join(environmentsDir(dir), de.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		var e Environment
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if err := dec.Decode(&e); err != nil && !errors.Is(err, io.EOF) {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		e.ID = id
		if e.Kind == "" {
			e.Kind = "local"
		}
		for i := range e.Rules {
			e.Rules[i].Scope, e.Rules[i].Source = permission.ScopeEnvironment, filepath.Join(EnvironmentsDir, id+".yaml")
		}
		if err := e.Validate(); err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Environment) int { return strings.Compare(a.ID, b.ID) })
	return out, issues, nil
}

// SaveEnvironment writes environments/<id>.yaml.
func SaveEnvironment(dir string, e Environment) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(environmentsDir(dir), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	b, err := yaml.Marshal(e)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(environmentsDir(dir), e.ID+".yaml"), b)
}

// DeleteEnvironment removes environments/<id>.yaml.
func DeleteEnvironment(dir, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("bad environment id %q", id)
	}
	err := os.Remove(filepath.Join(environmentsDir(dir), id+".yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
