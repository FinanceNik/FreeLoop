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

// ErrRulesInvalid is returned with the valid rules when rules.yaml has
// errors, so callers can fail closed and refuse to overwrite the file.
var ErrRulesInvalid = errors.New("rules.yaml has errors")

// LoadRules reads rules.yaml from dir. A missing file has no rules. When the
// file cannot be parsed or holds invalid rules, the valid rules come back
// together with the issues and ErrRulesInvalid.
func LoadRules(dir string) ([]permission.Rule, []Issue, error) {
	path := filepath.Join(dir, RulesFileName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	return ParseRules(b, permission.ScopeGlobal, path)
}

// ParseRules decodes a rules file's contents, giving each rule scope and
// source. Like LoadRules it returns the valid rules with the issues and
// ErrRulesInvalid when any rule is bad.
func ParseRules(b []byte, scope permission.Scope, source string) ([]permission.Rule, []Issue, error) {
	var f rulesFile
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, []Issue{{File: source, Line: yamlErrorLine(err), Message: err.Error()}}, ErrRulesInvalid
	}
	var out []permission.Rule
	var issues []Issue
	for i, r := range f.Rules {
		if err := r.Validate(); err != nil {
			issues = append(issues, Issue{File: source, Field: fmt.Sprintf("rules[%d]", i), Line: fieldLine(b, fmt.Sprintf("rules[%d]", i)), Message: err.Error()})
			continue
		}
		r.Scope, r.Source = scope, source
		out = append(out, r)
	}
	if len(issues) > 0 {
		return out, issues, ErrRulesInvalid
	}
	return out, nil, nil
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
