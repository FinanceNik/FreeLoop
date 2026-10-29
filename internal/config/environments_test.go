package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnvironmentsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Environment{ID: "work", Name: "Work", Kind: "local", Env: map[string]string{"GOFLAGS": "-mod=mod"},
		Secrets: map[string]string{"GITHUB_TOKEN": "keychain://freeloop/environments/work/GITHUB_TOKEN"}, DisabledTools: []string{"shell"}, DefaultMode: "plan"}
	if err := SaveEnvironment(dir, in); err != nil {
		t.Fatal(err)
	}
	out, issues, err := LoadEnvironments(dir)
	if err != nil || len(issues) != 0 || len(out) != 1 || !reflect.DeepEqual(out[0], in) {
		t.Fatalf("LoadEnvironments = %+v, %v, %v", out, issues, err)
	}
	if err := DeleteEnvironment(dir, "work"); err != nil {
		t.Fatal(err)
	}
	if out, _, _ := LoadEnvironments(dir); len(out) != 0 {
		t.Errorf("after delete: %+v", out)
	}
}

func TestEnvironmentValidation(t *testing.T) {
	ok := Environment{ID: "a", Name: "A", Kind: "local"}
	for name, mutate := range map[string]func(*Environment){
		"bad id":         func(e *Environment) { e.ID = "Bad ID" },
		"no name":        func(e *Environment) { e.Name = " " },
		"docker":         func(e *Environment) { e.Kind = "docker" },
		"bypass default": func(e *Environment) { e.DefaultMode = "bypass" },
		"bad var":        func(e *Environment) { e.Env = map[string]string{"1X": "y"} },
		"secret value":   func(e *Environment) { e.Secrets = map[string]string{"TOKEN": "ghp_123"} },
		"var and secret": func(e *Environment) {
			e.Env = map[string]string{"T": "1"}
			e.Secrets = map[string]string{"T": "keychain://freeloop/x"}
		},
	} {
		e := ok
		mutate(&e)
		if e.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, EnvironmentsDir), 0o700)
	os.WriteFile(filepath.Join(dir, EnvironmentsDir, "x.yaml"), []byte("name: X\nsecrets: {T: plain}\n"), 0o600)
	if out, issues, _ := LoadEnvironments(dir); len(out) != 0 || len(issues) != 1 || !strings.Contains(issues[0].Message, "never a value") {
		t.Errorf("out = %+v issues = %+v", out, issues)
	}
}
