package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/permission"
)

func TestProfilesLoadAndReportLines(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ProfilesDir), 0o700)
	write := func(name, body string) {
		os.WriteFile(filepath.Join(dir, ProfilesDir, name), []byte(body), 0o600)
	}
	write("reviewer.yaml", `name: Reviewer
provider: anthropic
model: claude-sonnet-5-5
mode: plan
tools: [read_file, grep]
rules:
  - effect: deny
    tool: shell
    command: git push *
commands:
  - name: review
    description: Review the changes
    prompt: Review the uncommitted changes.
`)
	write("typo.yaml", "name: Typo\nmodel: x\nmodle: y\n")
	write("badmode.yaml", "name: Bad\n\nmode: bypass\n")
	write("badtool.yaml", "name: Tools\ntools:\n  - read_file\n  - read_fiel\n")
	write("badcmd.yaml", "name: Cmd\ncommands:\n  - name: ok\n    prompt: fine\n  - name: plan\n    prompt: clashes\n")

	list, issues, err := LoadProfiles(dir, []string{"read_file", "grep"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "reviewer" || list[0].Harness != "builtin" || len(list[0].Commands) != 1 {
		t.Fatalf("profiles = %+v", list)
	}
	if r := list[0].Rules[0]; r.Scope != permission.ScopeProfile || r.Source != "profiles/reviewer.yaml" {
		t.Errorf("rule = %+v", r)
	}
	got := map[string]Issue{}
	for _, is := range issues {
		got[filepath.Base(is.File)] = is
	}
	if is := got["typo.yaml"]; is.Line != 3 || !strings.Contains(is.Message, "modle") {
		t.Errorf("typo issue = %+v", is)
	}
	if is := got["badmode.yaml"]; is.Line != 3 || is.Field != "mode" {
		t.Errorf("mode issue = %+v", is)
	}
	if is := got["badtool.yaml"]; is.Line != 4 || is.Field != "tools[1]" {
		t.Errorf("tool issue = %+v", is)
	}
	if is := got["badcmd.yaml"]; is.Line != 5 || is.Field != "commands[1]" || !strings.Contains(is.Message, "/plan") {
		t.Errorf("command issue = %+v", is)
	}
}

func TestSaveProfileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := Profile{ID: "quick", Name: "Quick", Model: "m", MaxTurns: 5, Commands: []Command{{Name: "fix", Prompt: "Fix it"}}}
	if err := SaveProfile(dir, p); err != nil {
		t.Fatal(err)
	}
	list, issues, _ := LoadProfiles(dir, nil)
	if len(issues) != 0 || len(list) != 1 || list[0].MaxTurns != 5 || list[0].Commands[0].Prompt != "Fix it" {
		t.Fatalf("loaded %+v, %v", list, issues)
	}
	if err := SaveProfile(dir, Profile{ID: "Bad ID", Name: "x"}); err == nil {
		t.Error("saved a bad id")
	}
	if err := DeleteProfile(dir, "quick"); err != nil {
		t.Fatal(err)
	}
	if list, _, _ := LoadProfiles(dir, nil); len(list) != 0 {
		t.Errorf("after delete: %+v", list)
	}
}
