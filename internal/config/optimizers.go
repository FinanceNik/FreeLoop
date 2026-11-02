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
	"unicode"

	"go.yaml.in/yaml/v3"
)

// OptimizersDir holds one YAML file per token optimizer (HRN-019).
const OptimizersDir = "optimizers"

// OptimizerOff, as a session's or profile's choice, keeps a session from
// taking the optimizer it would otherwise inherit.
const OptimizerOff = "off"

// Optimizer is a program that rewrites shell commands so their output costs
// fewer tokens (HRN-019). It runs after the broker has approved the original
// command and has no say in what may run.
type Optimizer struct {
	ID   string `yaml:"-"` // the file name without .yaml
	Name string `yaml:"name"`
	// Program is looked up the way the session's commands find it. A rewrite
	// may only put this word in front of a command.
	Program string `yaml:"program"`
	// Rewrite are the arguments before the command to rewrite. A run that
	// exits with one of RewriteExits printed the new command; any other exit
	// leaves the command as it is.
	Rewrite      []string `yaml:"rewrite"`
	RewriteExits []int    `yaml:"rewriteExits"`
	// ProbeCommand must rewrite to ProbeWant, which tells this program from
	// another with the same name.
	ProbeCommand string `yaml:"probeCommand"`
	ProbeWant    string `yaml:"probeWant"`
	// Version are the arguments that print its version.
	Version []string `yaml:"version,omitempty"`
	// Env is added to every command while the optimizer is on, and DataEnv
	// points variables at files in the session's own folder.
	Env     map[string]string `yaml:"env,omitempty"`
	DataEnv map[string]string `yaml:"dataEnv,omitempty"`
	// Install tells the user how to get the program. It is typed into a
	// terminal for them to read and run.
	Install string `yaml:"install,omitempty"`
	// Source is the file it came from; empty for a built-in one.
	Source string `yaml:"-"`
}

// BuiltinOptimizers are the optimizers every install offers.
func BuiltinOptimizers() []Optimizer {
	return []Optimizer{{
		ID: "rtk", Name: "RTK", Program: "rtk",
		Rewrite: []string{"rewrite"}, RewriteExits: []int{0, 3},
		ProbeCommand: "git status", ProbeWant: "rtk git status",
		Version: []string{"--version"},
		Env: map[string]string{
			"RTK_TELEMETRY_DISABLED":    "1",
			"RTK_ONRAMP_DISABLED":       "1",
			"RTK_SUPPRESS_HOOK_WARNING": "1",
		},
		// Full outputs are kept here, for `rtk recall`, so they go when the
		// session goes.
		DataEnv: map[string]string{"RTK_DB_PATH": "tracking.db", "RTK_RECALL_DB": "recall.db"},
		Install: "brew install rtk",
	}}
}

var programName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// Validate reports settings that cannot work.
func (o Optimizer) Validate() error {
	switch {
	case !idPattern.MatchString(o.ID):
		return fmt.Errorf("id %q must be lowercase letters, digits and dashes", o.ID)
	case o.ID == OptimizerOff:
		return fmt.Errorf("%q means no optimizer, so it cannot name one", OptimizerOff)
	case strings.TrimSpace(o.Name) == "":
		return errors.New("name is required")
	case !programName.MatchString(o.Program):
		return fmt.Errorf("program %q must be a program's name, without a path", o.Program)
	case len(o.Rewrite) == 0:
		return errors.New("rewrite: the arguments that ask for a rewrite are required")
	case len(o.RewriteExits) == 0:
		return errors.New("rewriteExits: the exit codes of a rewrite are required")
	case strings.TrimSpace(o.ProbeCommand) == "":
		return errors.New("probeCommand: a command to try it with is required")
	case !strings.HasPrefix(o.ProbeWant, o.Program+" "):
		return fmt.Errorf("probeWant must be what probeCommand becomes, starting with %s", o.Program)
	case strings.ContainsFunc(o.Install, unicode.IsControl):
		// A newline would run it the moment it is typed into a terminal.
		return errors.New("install must be one line")
	}
	for k := range o.Env {
		if !varName.MatchString(k) {
			return fmt.Errorf("env: %q is not a valid variable name", k)
		}
	}
	for k, file := range o.DataEnv {
		if !varName.MatchString(k) {
			return fmt.Errorf("dataEnv: %q is not a valid variable name", k)
		}
		if file == "" || file == "." || file == ".." || file != filepath.Base(file) {
			return fmt.Errorf("dataEnv: %s must name a file, without a path", k)
		}
	}
	return nil
}

// LoadOptimizers returns the built-in optimizers with those in optimizers/ on
// top, sorted by ID. Files with problems are skipped and reported.
func LoadOptimizers(dir string) ([]Optimizer, []Issue) {
	out := BuiltinOptimizers()
	var issues []Issue
	odir := filepath.Join(dir, OptimizersDir)
	entries, err := os.ReadDir(odir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		issues = append(issues, Issue{File: odir, Message: err.Error()})
	}
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name(), ".yaml")
		if !ok || de.IsDir() {
			continue
		}
		path := filepath.Join(odir, de.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		var o Optimizer
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if err := dec.Decode(&o); err != nil && !errors.Is(err, io.EOF) {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		o.ID, o.Source = id, filepath.Join(OptimizersDir, de.Name())
		if err := o.Validate(); err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		out = slices.DeleteFunc(out, func(x Optimizer) bool { return x.ID == id })
		out = append(out, o)
	}
	slices.SortFunc(out, func(a, b Optimizer) int { return strings.Compare(a.ID, b.ID) })
	return out, issues
}
