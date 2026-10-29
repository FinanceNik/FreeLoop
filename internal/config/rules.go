package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/FinanceNik/Freeloop/internal/permission"
)

// RulesFileName holds global permission rules (SEC-002).
const RulesFileName = "rules.yaml"

type rulesFile struct {
	Version int               `yaml:"version"`
	Rules   []permission.Rule `yaml:"rules"`
}

// LoadRules reads rules.yaml from dir. Invalid rules are skipped and
// reported; a missing file has no rules.
func LoadRules(dir string) ([]permission.Rule, []Issue, error) {
	path := filepath.Join(dir, RulesFileName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	var f rulesFile
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, []Issue{{File: path, Message: err.Error()}}, nil
	}
	var out []permission.Rule
	var issues []Issue
	for i, r := range f.Rules {
		if err := r.Validate(); err != nil {
			issues = append(issues, Issue{File: path, Field: fmt.Sprintf("rules[%d]", i), Message: err.Error()})
			continue
		}
		r.Scope, r.Source = permission.ScopeGlobal, path
		out = append(out, r)
	}
	return out, issues, nil
}

// SaveRules replaces rules.yaml.
func SaveRules(dir string, rules []permission.Rule) error {
	b, err := yaml.Marshal(rulesFile{Version: 1, Rules: rules})
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, RulesFileName), b)
}

// RulesModTime is rules.yaml's modification time, zero when it is missing,
// so callers can reload after a hand edit.
func RulesModTime(dir string) time.Time {
	fi, err := os.Stat(filepath.Join(dir, RulesFileName))
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
