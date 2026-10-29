package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/permission"
)

func TestRulesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if !RulesModTime(dir).IsZero() {
		t.Error("mod time of a missing file is not zero")
	}
	in := []permission.Rule{{Effect: permission.Allow, Tool: "shell", Command: "git *"}, {Effect: permission.Deny, Tool: "write_file", Path: "**/*.lock"}}
	if err := SaveRules(dir, in); err != nil {
		t.Fatal(err)
	}
	out, issues, err := LoadRules(dir)
	if err != nil || len(issues) != 0 || len(out) != 2 {
		t.Fatalf("LoadRules = %+v, %v, %v", out, issues, err)
	}
	if out[0].Scope != permission.ScopeGlobal || out[0].Source != filepath.Join(dir, RulesFileName) || out[1].Path != "**/*.lock" {
		t.Errorf("rules = %+v", out)
	}
}

func TestInvalidRulesAreReported(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, RulesFileName), []byte("version: 1\nrules:\n  - {effect: allow, tool: shell, command: ls}\n  - {effect: perhaps, tool: shell}\n"), 0o600)
	out, issues, _ := LoadRules(dir)
	if len(out) != 1 || len(issues) != 1 || issues[0].Field != "rules[1]" {
		t.Errorf("out = %+v issues = %+v", out, issues)
	}
}
