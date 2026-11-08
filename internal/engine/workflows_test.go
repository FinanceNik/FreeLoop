package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// sentText is the last user message a session's transcript holds.
func sentText(t *testing.T, e *Engine, sid string) string {
	t.Helper()
	evs, err := e.store.Events(context.Background(), sid, 0)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, ev := range evs {
		if ev.Type == event.UserMessageChunk {
			var d struct {
				Content struct{ Text string } `json:"content"`
			}
			json.Unmarshal(ev.Data, &d)
			text = d.Content.Text
		}
	}
	return text
}

// WFL-002: a new session takes the default workflow, recorded in its transcript; a
// profile's off beats the default, and the session's own choice beats both.
func TestWorkflowPrecedence(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	if _, err := e.SetDefaultWorkflow(ctx, config.BuiltinWorkflowID); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if ses.Workflow != config.BuiltinWorkflowID {
		t.Errorf("default: workflow = %q", ses.Workflow)
	}
	evs, _ := e.store.Events(ctx, ses.ID, 0)
	if !slices.ContainsFunc(evs, func(ev event.Event) bool { return ev.Type == event.WorkflowChanged }) {
		t.Error("no workflow_changed at creation")
	}
	if err := e.SaveProfileYAML("plain", "name: Plain\nprovider: s\nmodel: m\nworkflow: off\n"); err != nil {
		t.Fatal(err)
	}
	off, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "plain"})
	if off.Workflow != "" {
		t.Errorf("profile off: workflow = %q", off.Workflow)
	}
	asked, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "plain", Workflow: config.BuiltinWorkflowID})
	if asked.Workflow != config.BuiltinWorkflowID {
		t.Errorf("asked: workflow = %q", asked.Workflow)
	}
	if err := e.SetWorkflow(ctx, asked.ID, "off"); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.store.GetSession(ctx, asked.ID); got.Workflow != "" {
		t.Errorf("after off: %q", got.Workflow)
	}
}

// WFL-002, WFL-003: a stage needs its skill installed; once it is, /grill switches to
// plan mode and sends the stage's prompt with what followed the command. Installing
// writes only into the app's own store, with its source, and once.
func TestRunStageSendsItsPrompt(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "main", Workflow: config.BuiltinWorkflowID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.RunStage(ctx, ses.ID, "grill", "add dark mode"); err == nil || !strings.Contains(err.Error(), "Install") {
		t.Fatalf("without its skill: %v", err)
	}
	names, err := e.InstallWorkflowSkills(ctx, config.BuiltinWorkflowID)
	if err != nil || len(names) != 4 {
		t.Fatalf("installed %v, %v", names, err)
	}
	prov, ok := config.ReadSkillProvenance(e.dataDir, "grill-the-brief")
	if !ok || prov.Source != config.WorkflowSkillSource(config.BuiltinWorkflowID) {
		t.Errorf("provenance: %+v", prov)
	}
	if again, _ := e.InstallWorkflowSkills(ctx, config.BuiltinWorkflowID); len(again) != 0 {
		t.Errorf("a second install wrote %v", again)
	}
	for _, s := range e.ListWorkflows(ctx)[0].Skills {
		if s.State != skillInstalled {
			t.Errorf("%s is %s", s.Name, s.State)
		}
	}
	// Nothing of it lands in the project.
	if _, err := os.Stat(filepath.Join(root, ".claude")); err == nil {
		t.Error("a .claude folder appeared in the project")
	}

	ch, cancel := e.Subscribe()
	defer cancel()
	// WFL-008: the grill stage needs its brief.
	if _, err := e.RunStage(ctx, ses.ID, "/grill", "  "); err == nil || !strings.Contains(err.Error(), "needs to know what it is about") {
		t.Errorf("a grill without a brief: %v", err)
	}
	if _, err := e.RunStage(ctx, ses.ID, "/grill", "add dark mode"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	text := sentText(t, e, ses.ID)
	if !strings.Contains(text, "# Workflow stage: grill") || !strings.Contains(text, "The brief: add dark mode") || !strings.Contains(text, "grill-the-brief") {
		t.Errorf("sent %q", text)
	}
	// It asks before changes rather than plan, whose own approval would read as
	// approving code (WFL-008).
	if got, _ := e.store.GetSession(ctx, ses.ID); got.Mode != "ask" {
		t.Errorf("mode = %s", got.Mode)
	}
	// The stage is recorded before its prompt, once, for the stage strip.
	evs, _ := e.store.Events(ctx, ses.ID, 0)
	var stages []string
	for i, ev := range evs {
		if ev.Type != event.WorkflowStage {
			continue
		}
		var d event.WorkflowStageData
		json.Unmarshal(ev.Data, &d)
		stages = append(stages, d.Command+"/"+d.Kind)
		if !slices.ContainsFunc(evs[i:], func(x event.Event) bool { return x.Type == event.UserMessageChunk }) {
			t.Error("the stage came after its prompt")
		}
	}
	if strings.Join(stages, ",") != "grill/prompt" {
		t.Errorf("stages recorded: %v", stages)
	}
	if _, err := e.RunStage(ctx, ses.ID, "nope", ""); err == nil {
		t.Error("a command the workflow lacks ran")
	}

	// A skill edited after installing shows as changed, and is not overwritten.
	md := filepath.Join(e.dataDir, config.SkillsDir, "write-a-prd", "SKILL.md")
	b, _ := os.ReadFile(md)
	os.WriteFile(md, append(b, "\nMine.\n"...), 0o600)
	e.forgetSkills()
	for _, s := range e.ListWorkflows(ctx)[0].Skills {
		if s.Name == "write-a-prd" && s.State != skillChanged {
			t.Errorf("edited skill is %s", s.State)
		}
	}
	e.InstallWorkflowSkills(ctx, config.BuiltinWorkflowID)
	if b2, _ := os.ReadFile(md); !strings.Contains(string(b2), "Mine.") {
		t.Error("install overwrote the user's edit")
	}
}

// WFL-001: a workflow file is used only once approved as it is, and an edit ends the
// approval.
func TestWorkflowFileNeedsApproval(t *testing.T) {
	ctx := context.Background()
	e, root := queueEngine(t)
	dir := filepath.Join(e.dataDir, config.WorkflowsDir)
	os.MkdirAll(dir, 0o700)
	file := filepath.Join(dir, "mine.yaml")
	os.WriteFile(file, []byte("name: Mine\nstages:\n  - {command: go, title: Go, prompt: \"Go: {{input}}\"}\n"), 0o600)
	e.loadWorkflows()
	ses, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "main", Workflow: "mine"})
	if _, err := e.RunStage(ctx, ses.ID, "go", "x"); err == nil || !strings.Contains(err.Error(), "approved") {
		t.Fatalf("unapproved: %v", err)
	}
	var mine WorkflowStatus
	for _, w := range e.ListWorkflows(ctx) {
		if w.ID == "mine" {
			mine = w
		}
	}
	if !mine.NeedsApproval {
		t.Fatal("a file's workflow did not need approval")
	}
	if _, err := e.ApproveWorkflow(ctx, "mine", "stale"); err == nil {
		t.Error("approved with a stale fingerprint")
	}
	if _, err := e.ApproveWorkflow(ctx, "mine", mine.Fingerprint); err != nil {
		t.Fatal(err)
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	if _, err := e.RunStage(ctx, ses.ID, "go", "x"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	if got := sentText(t, e, ses.ID); got != "Go: x" {
		t.Errorf("sent %q", got)
	}
	os.WriteFile(file, []byte("name: Mine\nstages:\n  - {command: go, title: Go, mode: accept_edits, prompt: \"Go: {{input}}\"}\n"), 0o600)
	e.loadWorkflows()
	if _, err := e.RunStage(ctx, ses.ID, "go", "x"); err == nil {
		t.Error("an edited file ran on the old approval")
	}
}
