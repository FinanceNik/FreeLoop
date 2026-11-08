package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/FinanceNik/Freeloop/internal/skill"
)

func TestBuiltinWorkflowIsValid(t *testing.T) {
	ws := BuiltinWorkflows()
	if len(ws) != 1 || ws[0].ID != BuiltinWorkflowID {
		t.Fatalf("built-in workflows = %+v", ws)
	}
	w := ws[0]
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"grill", "prd", "slice", "afk", "review", "cleanup"} {
		if _, ok := w.Stage(c); !ok {
			t.Errorf("no /%s", c)
		}
	}
	if w.AFKMode() != "accept_edits" || w.BacklogDir() != "backlog" || w.IssueCap() != DefaultMaxIssues {
		t.Errorf("afk defaults: %s %s %d", w.AFKMode(), w.BacklogDir(), w.IssueCap())
	}
	// Every skill a stage names ships with it, and parses as an Agent Skill.
	files := BuiltinWorkflowSkills(w.ID)
	named := append([]string{w.AFK.Skill}, w.Skills...)
	for _, s := range w.Stages {
		if s.Skill != "" {
			named = append(named, s.Skill)
		}
	}
	for _, name := range named {
		md, ok := files[name]["SKILL.md"]
		if !ok {
			t.Fatalf("skill %s does not ship", name)
		}
		front, _, err := skill.Split(md)
		var parsed struct{ Name, Description string }
		if err == nil {
			err = yaml.Unmarshal(front, &parsed)
		}
		if err != nil || parsed.Name != name || parsed.Description == "" || !skill.ValidName(name) {
			t.Errorf("skill %s: %+v %v", name, parsed, err)
		}
	}
	if len(files) != len(w.Skills) {
		t.Errorf("%d skills ship, %d listed", len(files), len(w.Skills))
	}
	if BuiltinWorkflowSkills("other") != nil {
		t.Error("a file's workflow has no built-in skills")
	}
}

func TestWorkflowValidation(t *testing.T) {
	ok := "name: Mine\nstages:\n  - command: go\n    title: Go\n    prompt: \"Do {{input}}\"\n"
	if _, err := ParseWorkflow("mine", []byte(ok)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, yaml, want string }{
		{"unknown field", ok + "colour: red\n", "colour"},
		{"bypass", "name: x\nstages:\n  - command: go\n    title: Go\n    mode: bypass\n    prompt: x\n", "never turns asking off"},
		{"auto", "name: x\nstages:\n  - command: go\n    title: Go\n    mode: auto\n    prompt: x\n", "never turns asking off"},
		{"afk bypass", ok + "afk:\n  mode: bypass\n", "afk.mode"},
		{"afk plan", ok + "afk:\n  mode: plan\n", "afk.mode"},
		{"twice", "name: x\nstages:\n  - {command: go, title: Go, prompt: x}\n  - {command: go, title: Again, prompt: y}\n", "used twice"},
		{"reserved", "name: x\nstages:\n  - {command: plan, title: Plan, prompt: x}\n", "app's own"},
		{"placeholder", "name: x\nstages:\n  - {command: go, title: Go, prompt: \"{{secret}}\"}\n", "{{secret}} is not a placeholder"},
		{"issue in a stage", "name: x\nstages:\n  - {command: go, title: Go, prompt: \"{{issueText}}\"}\n", "not a placeholder"},
		{"no prompt", "name: x\nstages:\n  - {command: go, title: Go}\n", "needs a prompt"},
		{"afk without prompt", "name: x\nstages:\n  - {command: afk, title: Run, kind: afk}\n", "afk.prompt"},
		{"two afk", "name: x\nstages:\n  - {command: a, title: A, kind: afk}\n  - {command: b, title: B, kind: afk}\nafk:\n  prompt: x\n", "one afk stage"},
		{"prompt on afk", "name: x\nstages:\n  - {command: a, title: A, kind: afk, prompt: x}\nafk:\n  prompt: x\n", "no prompt or mode"},
		{"every command", ok + "afk:\n  commands: [\"*\"]\n", "every command"},
		{"backlog outside", ok + "backlog: ../elsewhere\n", "inside the project"},
		{"backlog absolute", ok + "backlog: /tmp/x\n", "inside the project"},
		{"backlog in settings", ok + "backlog: .freeloop/issues\n", "agents may not write"},
		{"backlog in .github", ok + "backlog: .github/backlog\n", "agents may not write"},
		{"no stages", "name: x\n", "at least one stage"},
	} {
		_, err := ParseWorkflow("mine", []byte(c.yaml))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
	if _, err := ParseWorkflow("off", []byte(ok)); err == nil {
		t.Error("off names no workflow")
	}
}

func TestRenderTemplates(t *testing.T) {
	got := Render("Do {{ input }} in {{backlog}}; {{missing}}.", map[string]string{"input": "x", "backlog": "b"})
	if got != "Do x in b; ." {
		t.Errorf("Render = %q", got)
	}
	if p := Placeholders("{{a}} {{ b }} {{a}}"); strings.Join(p, ",") != "a,b" {
		t.Errorf("Placeholders = %v", p)
	}
}

func TestLoadWorkflowsReplacesAndReports(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, WorkflowsDir), 0o700)
	os.WriteFile(filepath.Join(dir, WorkflowsDir, BuiltinWorkflowID+".yaml"), []byte("name: Mine instead\nstages:\n  - {command: go, title: Go, prompt: x}\n"), 0o600)
	os.WriteFile(filepath.Join(dir, WorkflowsDir, "broken.yaml"), []byte("name: x\n"), 0o600)
	list, issues := LoadWorkflows(dir)
	if len(list) != 1 || list[0].Name != "Mine instead" || list[0].Source != filepath.Join(WorkflowsDir, BuiltinWorkflowID+".yaml") {
		t.Fatalf("list = %+v", list)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].File, "broken.yaml") {
		t.Errorf("issues = %+v", issues)
	}
	// What the user approves changes with anything the file says, and not with
	// where it is.
	a := list[0]
	b := a
	b.Source = "elsewhere"
	if a.Fingerprint() != b.Fingerprint() {
		t.Error("the source changed the fingerprint")
	}
	b.AFK.Commands = []string{"rm *"}
	if a.Fingerprint() == b.Fingerprint() {
		t.Error("a new command kept the fingerprint")
	}
}
