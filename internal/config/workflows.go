package config

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/FinanceNik/Freeloop/internal/permission"
)

// WorkflowsDir holds one YAML file per workflow (WFL-001).
const WorkflowsDir = "workflows"

// WorkflowOff, as a session's or profile's choice, keeps a session from taking the
// workflow it would otherwise inherit.
const WorkflowOff = "off"

// DefaultBacklog is the project folder a workflow's PRDs and issues are in when it
// names none (OQ-039).
const DefaultBacklog = "backlog"

// DefaultMaxIssues is how many issues an AFK run does at most unless set.
const DefaultMaxIssues = 10

// Kinds of a workflow stage.
const (
	// StagePrompt sends the stage's prompt in the session, as the user's message.
	StagePrompt = "prompt"
	// StageAFK starts an AFK run of the project's backlog (WFL-005).
	StageAFK = "afk"
	// StageReview has the workflow's review profile review the session (HRN-018).
	StageReview = "review"
	// StageCleanup archives the backlog's finished PRDs and issues (WFL-004).
	StageCleanup = "cleanup"
)

// Workflow is a method of work in stages (WFL-001): what each stage sends, in which
// mode and with which skill, where its backlog is, and how its AFK run and review go.
// Nothing in it runs until the user invokes a stage.
type Workflow struct {
	ID          string `yaml:"-" json:"id"` // the file name without .yaml
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description"`
	// Backlog is the project folder with prds/ and issues/; DefaultBacklog when empty.
	Backlog string `yaml:"backlog,omitempty" json:"backlog"`
	// Skills are the skills its stages use, by name, for Settings to show.
	Skills []string        `yaml:"skills,omitempty" json:"skills"`
	Stages []WorkflowStage `yaml:"stages" json:"stages"`
	AFK    WorkflowAFK     `yaml:"afk,omitempty" json:"afk"`
	Review WorkflowReview  `yaml:"review,omitempty" json:"review"`
	// Source is the file it came from; empty for the built-in one.
	Source string `yaml:"-" json:"source"`
}

// WorkflowStage is one step, invoked as a slash command.
type WorkflowStage struct {
	// Command is the slash command, without the slash.
	Command     string `yaml:"command" json:"command"`
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description,omitempty" json:"description"`
	// Kind is StagePrompt when empty.
	Kind string `yaml:"kind,omitempty" json:"kind"`
	// Mode is switched to before the prompt is sent; empty leaves it.
	Mode string `yaml:"mode,omitempty" json:"mode"`
	// Skill is the skill the prompt names, which must be installed and on.
	Skill  string `yaml:"skill,omitempty" json:"skill"`
	Prompt string `yaml:"prompt,omitempty" json:"prompt"`
	// NeedsInput says the stage needs what follows its command, such as a brief:
	// it is refused without, and its Next button asks for it rather than run (WFL-008).
	NeedsInput bool `yaml:"needsInput,omitempty" json:"needsInput"`
}

// WorkflowAFK is how an AFK run's sessions go (WFL-005).
type WorkflowAFK struct {
	// Profile is the sessions' profile when the run is not given one; the
	// project's default when empty or not found.
	Profile string `yaml:"profile,omitempty" json:"profile"`
	// Mode is ask or accept_edits; accept_edits when empty.
	Mode string `yaml:"mode,omitempty" json:"mode"`
	// Sandbox runs the sessions in the sandboxed environment where there is one.
	Sandbox bool `yaml:"sandbox,omitempty" json:"sandbox"`
	// MaxIssues caps the issues of one run; DefaultMaxIssues when 0.
	MaxIssues int `yaml:"maxIssues,omitempty" json:"maxIssues"`
	// Commands are shell rule patterns the run will want allowed, such as
	// "git commit *", offered for the user to allow up front.
	Commands []string `yaml:"commands,omitempty" json:"commands"`
	Skill    string   `yaml:"skill,omitempty" json:"skill"`
	// Prompt is each session's first message.
	Prompt string `yaml:"prompt,omitempty" json:"prompt"`
}

// WorkflowReview is how each finished issue is reviewed (WFL-006).
type WorkflowReview struct {
	// Off skips the review; the issue is ready for QA as soon as it is done.
	Off bool `yaml:"off,omitempty" json:"off"`
	// Profile reviews; the run's AFK profile when empty or not found.
	Profile string `yaml:"profile,omitempty" json:"profile"`
	// Focus is what the reviewer is asked to look at, a template.
	Focus string `yaml:"focus,omitempty" json:"focus"`
}

// Placeholders a template may use: what follows the stage's command, the backlog
// folder, the backlog's issues with their state, recent commits, and for an AFK
// session or a review its issue's path and text.
var (
	stagePlaceholders = []string{"input", "backlog", "issues", "commits"}
	issuePlaceholders = []string{"input", "backlog", "issues", "commits", "issue", "issueText"}
	placeholder       = regexp.MustCompile(`\{\{\s*([^{}]*?)\s*\}\}`)
)

// Placeholders returns the names a template uses, in order of first use.
func Placeholders(tmpl string) []string {
	var out []string
	for _, m := range placeholder.FindAllStringSubmatch(tmpl, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

// Render fills a template's placeholders from vars; a name vars lacks becomes empty.
func Render(tmpl string, vars map[string]string) string {
	return placeholder.ReplaceAllStringFunc(tmpl, func(m string) string {
		return vars[placeholder.FindStringSubmatch(m)[1]]
	})
}

func checkTemplate(field, tmpl string, allowed []string) error {
	for _, name := range Placeholders(tmpl) {
		if !slices.Contains(allowed, name) {
			return fmt.Errorf("%s: {{%s}} is not a placeholder; use %s", field, name, "{{"+strings.Join(allowed, "}}, {{")+"}}")
		}
	}
	return nil
}

// workflowMode reports a mode a workflow may switch to: never one in which nothing
// asks (SEC-003), and never auto, whose agent decides out of the broker's sight.
func workflowMode(m string) bool {
	return m == "" || m == string(permission.ModeAsk) || m == string(permission.ModePlan) || m == string(permission.ModeAcceptEdits)
}

// BacklogDir is the workflow's backlog folder.
func (w Workflow) BacklogDir() string {
	if w.Backlog == "" {
		return DefaultBacklog
	}
	return w.Backlog
}

// IssueCap is the most issues one AFK run does.
func (w Workflow) IssueCap() int {
	if w.AFK.MaxIssues <= 0 {
		return DefaultMaxIssues
	}
	return w.AFK.MaxIssues
}

// AFKMode is the mode the AFK run's sessions start in.
func (w Workflow) AFKMode() string {
	if w.AFK.Mode == "" {
		return string(permission.ModeAcceptEdits)
	}
	return w.AFK.Mode
}

// Stage returns the stage invoked by command.
func (w Workflow) Stage(command string) (WorkflowStage, bool) {
	i := slices.IndexFunc(w.Stages, func(s WorkflowStage) bool { return s.Command == command })
	if i < 0 {
		return WorkflowStage{}, false
	}
	return w.Stages[i], true
}

// StageKind is the stage's kind, StagePrompt when empty.
func (s WorkflowStage) StageKind() string {
	if s.Kind == "" {
		return StagePrompt
	}
	return s.Kind
}

var backlogPart = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9._-]*$`)

// Validate reports settings that cannot work.
func (w Workflow) Validate() error {
	switch {
	case !idPattern.MatchString(w.ID):
		return fmt.Errorf("id %q must be lowercase letters, digits and dashes", w.ID)
	case w.ID == WorkflowOff:
		return fmt.Errorf("%q means no workflow, so it cannot name one", WorkflowOff)
	case strings.TrimSpace(w.Name) == "":
		return errors.New("name is required")
	case len(w.Stages) == 0:
		return errors.New("stages: a workflow needs at least one stage")
	}
	// The backlog is a folder inside the project, and not one the app or other
	// tools act on: agents write it.
	if w.Backlog != "" {
		parts := strings.Split(w.Backlog, "/")
		for _, p := range parts {
			if !backlogPart.MatchString(p) || p == "." || p == ".." {
				return fmt.Errorf("backlog: %q must be a folder inside the project, such as backlog or docs/backlog", w.Backlog)
			}
		}
		for _, p := range parts {
			if f := strings.ToLower(p); f == permission.ProjectSettingsDir || f == ".git" || slices.ContainsFunc(permission.ActedOnLater, func(a string) bool { return strings.EqualFold(a, p) }) {
				return fmt.Errorf("backlog: %s is a folder agents may not write or that other tools act on", p)
			}
		}
	}
	for _, name := range w.Skills {
		if !skillName.MatchString(name) {
			return fmt.Errorf("skills: %q is not a skill's name", name)
		}
	}
	seen := map[string]bool{}
	kinds := map[string]bool{}
	for i, s := range w.Stages {
		field := fmt.Sprintf("stages[%d]", i)
		switch {
		case !commandName.MatchString(s.Command):
			return fmt.Errorf("%s: command %q must start with a letter and use lowercase letters, digits and dashes", field, s.Command)
		case slices.Contains(ReservedCommands, s.Command):
			return fmt.Errorf("%s: /%s is one of the app's own commands", field, s.Command)
		case seen[s.Command]:
			return fmt.Errorf("%s: /%s is used twice", field, s.Command)
		case strings.TrimSpace(s.Title) == "":
			return fmt.Errorf("%s: /%s needs a title", field, s.Command)
		case !slices.Contains([]string{StagePrompt, StageAFK, StageReview, StageCleanup}, s.StageKind()):
			return fmt.Errorf("%s: kind %q must be prompt, afk, review or cleanup", field, s.Kind)
		case !workflowMode(s.Mode):
			return fmt.Errorf("%s: mode %q must be ask, plan or accept_edits; a workflow never turns asking off", field, s.Mode)
		case s.Skill != "" && !skillName.MatchString(s.Skill):
			return fmt.Errorf("%s: skill %q is not a skill's name", field, s.Skill)
		case s.StageKind() == StagePrompt && strings.TrimSpace(s.Prompt) == "":
			return fmt.Errorf("%s: /%s needs a prompt", field, s.Command)
		case s.StageKind() != StagePrompt && (s.Prompt != "" || s.Mode != ""):
			return fmt.Errorf("%s: /%s is an %s stage, which has no prompt or mode of its own", field, s.Command, s.StageKind())
		case s.StageKind() != StagePrompt && kinds[s.StageKind()]:
			return fmt.Errorf("%s: there can be one %s stage", field, s.StageKind())
		}
		if err := checkTemplate(field+".prompt", s.Prompt, stagePlaceholders); err != nil {
			return err
		}
		seen[s.Command], kinds[s.StageKind()] = true, true
	}
	a := w.AFK
	switch {
	case a.Mode != "" && a.Mode != string(permission.ModeAsk) && a.Mode != string(permission.ModeAcceptEdits):
		return fmt.Errorf("afk.mode: %q must be ask or accept_edits; an unattended run never turns asking off", a.Mode)
	case a.MaxIssues < 0 || a.MaxIssues > 100:
		return fmt.Errorf("afk.maxIssues: %d must be between 1 and 100", a.MaxIssues)
	case a.Profile != "" && !idPattern.MatchString(a.Profile):
		return fmt.Errorf("afk.profile: %q is not a profile's ID", a.Profile)
	case a.Skill != "" && !skillName.MatchString(a.Skill):
		return fmt.Errorf("afk.skill: %q is not a skill's name", a.Skill)
	case kinds[StageAFK] && strings.TrimSpace(a.Prompt) == "":
		return errors.New("afk.prompt: an afk stage needs the prompt each session starts with")
	case w.Review.Profile != "" && !idPattern.MatchString(w.Review.Profile):
		return fmt.Errorf("review.profile: %q is not a profile's ID", w.Review.Profile)
	}
	if err := checkTemplate("afk.prompt", a.Prompt, issuePlaceholders); err != nil {
		return err
	}
	if err := checkTemplate("review.focus", w.Review.Focus, issuePlaceholders); err != nil {
		return err
	}
	for i, c := range a.Commands {
		r := permission.Rule{Tool: "shell", Command: c, Effect: permission.Allow, Scope: permission.ScopeSession}
		if strings.TrimSpace(c) == "" || strings.ContainsAny(c, "\n\r") || r.Validate() != nil {
			return fmt.Errorf("afk.commands[%d]: %q must be one shell command or a prefix such as \"git commit *\"", i, c)
		}
		// A rule matching every command would allow anything up front.
		if strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(c), "*")) == "" {
			return fmt.Errorf("afk.commands[%d]: %q would allow every command", i, c)
		}
	}
	return nil
}

// Fingerprint identifies what the workflow sends and allows. A file's workflow is
// used only after the user approved exactly this (WFL-001).
func (w Workflow) Fingerprint() string {
	w.Source = ""
	b, err := json.Marshal(w)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ParseWorkflow reads a workflow file's contents.
func ParseWorkflow(id string, b []byte) (Workflow, error) {
	var w Workflow
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&w); err != nil && !errors.Is(err, io.EOF) {
		return Workflow{}, err
	}
	w.ID = id
	return w, w.Validate()
}

// LoadWorkflows returns the built-in workflows with those in workflows/ on top,
// sorted by ID. Files with problems are skipped and reported.
func LoadWorkflows(dir string) ([]Workflow, []Issue) {
	out := BuiltinWorkflows()
	var issues []Issue
	wdir := filepath.Join(dir, WorkflowsDir)
	entries, err := os.ReadDir(wdir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		issues = append(issues, Issue{File: wdir, Message: err.Error()})
	}
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name(), ".yaml")
		if !ok || de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		p := filepath.Join(wdir, de.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			issues = append(issues, Issue{File: p, Message: err.Error()})
			continue
		}
		w, err := ParseWorkflow(id, b)
		if err != nil {
			issues = append(issues, Issue{File: p, Message: err.Error()})
			continue
		}
		w.Source = filepath.Join(WorkflowsDir, de.Name())
		out = slices.DeleteFunc(out, func(x Workflow) bool { return x.ID == id })
		out = append(out, w)
	}
	slices.SortFunc(out, func(a, b Workflow) int { return strings.Compare(a.ID, b.ID) })
	return out, issues
}

// BuiltinWorkflowID is the workflow that ships with the app.
const BuiltinWorkflowID = "plan-slice-afk"

//go:embed workflowskills
var workflowSkills embed.FS

// WorkflowSkillSource is what a built-in workflow's skill records as its source.
func WorkflowSkillSource(id string) string { return "freeloop:workflows/" + id }

// BuiltinWorkflowSkills are the files of the skills a built-in workflow ships with,
// by skill name, then path inside the skill. None for a file's workflow: its skills
// come from wherever the user installs them.
func BuiltinWorkflowSkills(id string) map[string]map[string][]byte {
	if id != BuiltinWorkflowID {
		return nil
	}
	out := map[string]map[string][]byte{}
	fs.WalkDir(workflowSkills, "workflowskills", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "workflowskills/")
		name, file, _ := strings.Cut(rel, "/")
		b, err := workflowSkills.ReadFile(p)
		if err != nil {
			return nil
		}
		if out[name] == nil {
			out[name] = map[string][]byte{}
		}
		out[name][path.Clean(file)] = b
		return nil
	})
	return out
}

// BuiltinWorkflows are the workflows every install offers.
func BuiltinWorkflows() []Workflow {
	return []Workflow{{
		ID:          BuiltinWorkflowID,
		Name:        "Plan, slice, run AFK",
		Description: "Grill the brief, write a PRD, slice it into issues, then let agents do the unattended ones, one fresh session each, each reviewed before you QA it. After Matt Pocock's workflow for AI coding.",
		Skills:      []string{"grill-the-brief", "write-a-prd", "slice-into-issues", "work-an-issue"},
		Stages: []WorkflowStage{
			{Command: "grill", Title: "Grill the brief", Description: "The agent interviews you about a brief, one question at a time, until you share the design", Mode: string(permission.ModeAsk), Skill: "grill-the-brief", NeedsInput: true,
				Prompt: "# Workflow stage: grill\n\nUse the skill grill-the-brief. Interview me about the brief below until we share one design: one question at a time, each with the answer you suggest. Look in the code for anything it can answer. Write nothing yet.\n\nThe brief: {{input}}\n"},
			{Command: "prd", Title: "Write the PRD", Description: "The agent writes the design you settled as a PRD in the backlog", Mode: string(permission.ModeAsk), Skill: "write-a-prd",
				Prompt: "# Workflow stage: prd\n\nUse the skill write-a-prd. Write the PRD for the design we settled in this conversation as one file in {{backlog}}/prds/: problem, solution, user stories, decisions, how it is tested, and what is out of scope.\n\n{{input}}\n"},
			{Command: "slice", Title: "Slice into issues", Description: "The agent splits the PRD into vertical slices, each marked AFK or human, with what blocks it", Mode: string(permission.ModeAsk), Skill: "slice-into-issues",
				Prompt: "# Workflow stage: slice\n\nUse the skill slice-into-issues. Split the PRD into vertical slices, one issue file each in {{backlog}}/issues/, each cutting through every layer, with `blockedBy` and `type: afk` or `type: human`. Then commit {{backlog}}.\n\nThe backlog now:\n{{issues}}\n\n{{input}}\n"},
			{Command: "afk", Title: "Run the AFK issues", Description: "Start a run: a fresh session per unblocked AFK issue, each reviewed when done", Kind: StageAFK},
			{Command: "review", Title: "Review this session", Description: "A second agent on the review profile reviews this session's changes", Kind: StageReview},
			{Command: "cleanup", Title: "Archive what is finished", Description: "Move done issues and finished PRDs out of the project", Kind: StageCleanup},
		},
		AFK: WorkflowAFK{
			Mode:     string(permission.ModeAcceptEdits),
			Sandbox:  true,
			Commands: []string{"git status *", "git diff *", "git log *", "git add *", "git commit *"},
			Skill:    "work-an-issue",
			Prompt:   "# Workflow stage: afk\n\nUse the skill work-an-issue. You are working unattended on one issue of this project's backlog. Nobody is watching: anything that needs an approval waits until someone answers, so keep to what you need.\n\nYour issue is {{issue}}:\n\n{{issueText}}\n\nThe backlog:\n{{issues}}\n\nRecent commits:\n{{commits}}\n\nWork test-first. Run the project's tests and type checks. Commit your change, and set `status: done` in {{issue}} when, and only when, the issue is done. Work on nothing else.\n",
		},
		Review: WorkflowReview{
			Focus: "whether the changes do what the issue {{issue}} asks, and nothing else. The issue:\n\n{{issueText}}",
		},
	}}
}
