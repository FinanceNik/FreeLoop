package engine

import (
	"cmp"
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/skill"
)

// Workflows (WFL-001 to WFL-003): a method of work in stages, each a slash command
// that sends a prompt naming a skill, switches the mode, or starts an AFK run, a
// review or a clean-up. A workflow is a file, as a token optimizer is: listing them
// runs nothing, a file's runs only once the user approved it as it is, and its skills
// install only when the user asks. A stage's prompt is sent as the user's message, so
// every harness gets the same (HRN-001) and the broker asks as always (SEC-003).

func (e *Engine) loadWorkflows() {
	list, issues := config.LoadWorkflows(e.dataDir)
	e.mu.Lock()
	e.workflows = list
	e.issues[issueWorkflows] = issues
	e.mu.Unlock()
}

func (e *Engine) workflowList() []config.Workflow {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.workflows)
}

func (e *Engine) findWorkflow(id string) (config.Workflow, bool) {
	list := e.workflowList()
	i := slices.IndexFunc(list, func(w config.Workflow) bool { return w.ID == id })
	if i < 0 {
		return config.Workflow{}, false
	}
	return list[i], true
}

// workflowApproved reports whether a workflow may be used: the built-in one, and a
// file the user approved as it is now. A file arrives without asking, and its stages
// switch modes and its run allows commands for its sessions.
func (e *Engine) workflowApproved(w config.Workflow) bool {
	if w.Source == "" {
		return true
	}
	mark, err := e.approvalMark(w.Fingerprint())
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(mark), []byte(e.Config().WorkflowApprovals[w.ID]))
}

// usableWorkflow returns the workflow id names, or why it cannot be used.
func (e *Engine) usableWorkflow(id string) (config.Workflow, error) {
	w, ok := e.findWorkflow(id)
	if !ok {
		return w, fmt.Errorf("there is no workflow %q", id)
	}
	if !e.workflowApproved(w) {
		return w, fmt.Errorf("%s comes from %s and has not been approved as it is now. Approve it in Settings, under Workflows.", w.Name, w.Source)
	}
	return w, nil
}

// WorkflowSkill is one skill a workflow uses, as Settings shows it.
type WorkflowSkill struct {
	Name string `json:"name"`
	// State is installed (the shipped text, unchanged), changed (edited since it
	// was installed), outdated (an older shipped text), other (a skill of that name
	// from elsewhere, used as it is), or missing.
	State string `json:"state"`
	// Shipped says the app can install it.
	Shipped bool `json:"shipped"`
	// Off says it is turned off everywhere in Settings → Skills.
	Off bool `json:"off"`
}

// Skill states.
const (
	skillInstalled = "installed"
	skillChanged   = "changed"
	skillOutdated  = "outdated"
	skillOther     = "other"
	skillMissing   = "missing"
)

// WorkflowStatus is a workflow as the Settings page shows it.
type WorkflowStatus struct {
	config.Workflow
	Skills []WorkflowSkill `json:"skills"`
	// NeedsApproval is set for a file the user has not approved as it is now, with
	// Fingerprint what to approve.
	NeedsApproval bool   `json:"needsApproval"`
	Fingerprint   string `json:"fingerprint"`
	// ModelSet is the ID of the model set the user chose for it, if any (WFL-013).
	ModelSet string `json:"modelSet"`
}

// ListWorkflows lists the workflows with their skills. Nothing is run.
func (e *Engine) ListWorkflows(ctx context.Context) []WorkflowStatus {
	out := []WorkflowStatus{}
	for _, w := range e.workflowList() {
		out = append(out, WorkflowStatus{Workflow: w, Skills: e.workflowSkills(w), NeedsApproval: !e.workflowApproved(w), Fingerprint: w.Fingerprint(),
			ModelSet: e.Config().WorkflowModelSets[w.ID]})
	}
	return out
}

// skillNames are every skill a workflow names, in order.
func skillNames(w config.Workflow) []string {
	names := slices.Clone(w.Skills)
	add := func(n string) {
		if n != "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	for _, s := range w.Stages {
		add(s.Skill)
	}
	add(w.AFK.Skill)
	return names
}

// workflowSkills says where each of a workflow's skills stands in the app's store.
func (e *Engine) workflowSkills(w config.Workflow) []WorkflowSkill {
	shipped := config.BuiltinWorkflowSkills(w.ID)
	off := e.Config().Skills.Disabled
	fs, _ := e.appSkillsFS()
	var out []WorkflowSkill
	for _, name := range skillNames(w) {
		files, ships := shipped[name]
		ws := WorkflowSkill{Name: name, Shipped: ships, Off: slices.Contains(off, name), State: skillMissing}
		have := map[string][]byte{}
		if fs != nil {
			collect(fs, name, have)
		}
		if len(have) > 0 {
			prov, ok := config.ReadSkillProvenance(e.dataDir, name)
			now := map[string][]byte{}
			for p, b := range have {
				now[strings.TrimPrefix(p, name+"/")] = b
			}
			switch {
			case !ok || prov.Source != config.WorkflowSkillSource(w.ID):
				ws.State = skillOther
			case digest(now) != prov.Digest:
				ws.State = skillChanged
			case ships && prov.Digest != digest(files):
				ws.State = skillOutdated
			default:
				ws.State = skillInstalled
			}
		}
		out = append(out, ws)
	}
	return out
}

// InstallWorkflowSkills copies a built-in workflow's skills into the app's skill
// store, with their source (WFL-003). A skill of the same name from elsewhere, or
// one the user changed, is left as it is; one already current is not written
// again. It returns the names written.
func (e *Engine) InstallWorkflowSkills(ctx context.Context, id string) ([]string, error) {
	w, ok := e.findWorkflow(id)
	if !ok {
		return nil, fmt.Errorf("there is no workflow %q", id)
	}
	shipped := config.BuiltinWorkflowSkills(w.ID)
	if len(shipped) == 0 {
		return nil, fmt.Errorf("%s ships no skills; install them from Settings → Skills", w.Name)
	}
	var done []string
	for _, s := range e.workflowSkills(w) {
		files, ok := shipped[s.Name]
		if !ok || (s.State != skillMissing && s.State != skillOutdated) {
			continue
		}
		prov := config.SkillProvenance{Source: config.WorkflowSkillSource(w.ID), ImportedAt: time.Now().UTC(), Digest: digest(files)}
		if err := config.InstallSkill(e.dataDir, s.Name, files, prov); err != nil {
			return done, err
		}
		done = append(done, s.Name)
	}
	e.forgetSkills()
	return done, nil
}

// ApproveWorkflow approves a workflow file as the user saw it, which must still be
// what the file says.
func (e *Engine) ApproveWorkflow(ctx context.Context, id, fingerprint string) (config.Config, error) {
	w, ok := e.findWorkflow(id)
	if !ok {
		return config.Config{}, fmt.Errorf("there is no workflow %q", id)
	}
	if w.Fingerprint() != fingerprint {
		return config.Config{}, fmt.Errorf("the file of %s changed; look at it again", w.Name)
	}
	mark, err := e.approvalMark(fingerprint)
	if err != nil {
		return config.Config{}, err
	}
	c := e.Config()
	approvals := map[string]string{}
	maps.Copy(approvals, c.WorkflowApprovals)
	approvals[id] = mark
	c.WorkflowApprovals = approvals
	return e.SaveSettings(c)
}

// SetDefaultWorkflow sets the workflow of new sessions; "" is none. Choosing one runs
// nothing, and a file's workflow is still used only once approved.
func (e *Engine) SetDefaultWorkflow(ctx context.Context, id string) (config.Config, error) {
	if id == config.WorkflowOff {
		id = ""
	}
	if id != "" {
		if _, ok := e.findWorkflow(id); !ok {
			return config.Config{}, fmt.Errorf("there is no workflow %q", id)
		}
	}
	c := e.Config()
	c.Defaults.Workflow = id
	return e.SaveSettings(c)
}

// chooseWorkflow decides a new session's workflow: the one asked for, then the
// profile's, then the default; off on any of them is none.
func (e *Engine) chooseWorkflow(req CreateSessionRequest, prof config.Profile) string {
	id := cmp.Or(req.Workflow, prof.Workflow, e.Config().Defaults.Workflow)
	if id == config.WorkflowOff {
		return ""
	}
	if _, ok := e.findWorkflow(id); !ok {
		return ""
	}
	return id
}

// WorkflowInfo is a workflow as a session can use it.
type WorkflowInfo struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Stages []config.WorkflowStage `json:"stages"`
	// Reason says why it cannot be used yet; empty when it can.
	Reason string `json:"reason"`
	// Planning is the planning model a session chosen with it starts on, from its
	// model set, and ModelSet that set's name (WFL-014); none when not fixed or not
	// to be had.
	Planning *config.ModelChoice `json:"planning"`
	ModelSet string              `json:"modelSet"`
}

// Workflows lists the workflows a session can choose.
func (e *Engine) Workflows(ctx context.Context) []WorkflowInfo {
	var out []WorkflowInfo
	for _, w := range e.workflowList() {
		info := WorkflowInfo{ID: w.ID, Name: w.Name, Stages: w.Stages}
		if _, err := e.usableWorkflow(w.ID); err != nil {
			info.Reason = err.Error()
		}
		if row, ok := e.startingPlanning(CreateSessionRequest{Workflow: w.ID}); ok {
			set, _ := e.workflowModelSet(w.ID)
			info.Planning, info.ModelSet = &row, set.Name
		}
		out = append(out, info)
	}
	return out
}

// SetWorkflow switches a session's workflow; "" or off is none.
func (e *Engine) SetWorkflow(ctx context.Context, sid, id string) error {
	if id == config.WorkflowOff {
		id = ""
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return err
	}
	if ses.Workflow == id {
		return nil
	}
	name := ""
	if id != "" {
		w, ok := e.findWorkflow(id)
		if !ok {
			return fmt.Errorf("there is no workflow %q", id)
		}
		name = w.Name
	}
	e.emit(sid, event.WorkflowChanged, event.WorkflowChangedData{Workflow: id, Name: name})
	return nil
}

// StageResult is what invoking a stage did.
type StageResult struct {
	Kind string `json:"kind"`
	// Message says what happened, when it was not a message sent in the session.
	Message string `json:"message"`
	// SessionID is a session the stage started, such as a review.
	SessionID string `json:"sessionId"`
	// Blocked says why the stage did not start: the workflow's model set names a
	// planning model the session cannot have (WFL-014). RunStageOnCurrent runs it on
	// the session's own model instead.
	Blocked string `json:"blocked"`
}

// RunStage invokes the stage of the session's workflow whose command is command, with
// input what followed the command (WFL-002).
func (e *Engine) RunStage(ctx context.Context, sid, command, input string) (StageResult, error) {
	return e.runStage(ctx, sid, command, input, false)
}

// RunStageOnCurrent runs a stage on the session's own model, where its model set's
// planning model cannot be had (WFL-014).
func (e *Engine) RunStageOnCurrent(ctx context.Context, sid, command, input string) (StageResult, error) {
	return e.runStage(ctx, sid, command, input, true)
}

func (e *Engine) runStage(ctx context.Context, sid, command, input string, onCurrent bool) (StageResult, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return StageResult{}, err
	}
	if ses.Workflow == "" {
		return StageResult{}, errors.New("this session has no workflow; choose one beside the optimizer")
	}
	w, err := e.usableWorkflow(ses.Workflow)
	if err != nil {
		return StageResult{}, err
	}
	stage, ok := w.Stage(strings.TrimPrefix(command, "/"))
	if !ok {
		return StageResult{}, fmt.Errorf("%s has no /%s", w.Name, strings.TrimPrefix(command, "/"))
	}
	res := StageResult{Kind: stage.StageKind()}
	switch stage.StageKind() {
	case config.StageAFK:
		// It starts from its dialog, where the user sees what it will do and allows its
		// commands (PrepareAFK, StartAFK).
		return res, errors.New("an AFK run starts from its dialog: open the Board's backlog, or type /afk in the app")
	case config.StageReview:
		return e.reviewStage(ctx, ses, w, strings.TrimSpace(input))
	case config.StageCleanup:
		return e.cleanupStage(ctx, ses, w)
	}
	if stage.NeedsInput && strings.TrimSpace(input) == "" {
		return res, fmt.Errorf("/%s needs to know what it is about: type it after the command", stage.Command)
	}
	if err := e.skillsReady(ctx, ses, w, stage.Skill); err != nil {
		return res, err
	}
	if !onCurrent {
		if why, err := e.planningModel(ctx, ses, w); err != nil || why != "" {
			res.Blocked = why
			return res, err
		}
	}
	if stage.Mode != "" && stage.Mode != ses.Mode {
		if err := e.SetMode(ctx, sid, stage.Mode); err != nil {
			return res, err
		}
	}
	// The stage is recorded before its prompt, so the conversation shows where it
	// starts and the stage strip knows it ran (WFL-008).
	e.emit(sid, event.WorkflowStage, event.WorkflowStageData{Workflow: w.ID, Command: stage.Command, Title: stage.Title, Kind: stage.StageKind()})
	text := config.Render(stage.Prompt, e.stageVars(ctx, ses, w, strings.TrimSpace(input)))
	// The template and the backlog it quotes are not the user's typing: their @
	// words attach nothing.
	_, err = e.enqueue(ctx, sid, queued{text: strings.TrimSpace(text), noMentions: true})
	return res, err
}

// planningModel switches a session to its workflow's planning model and effort, from
// the workflow's model set, before a planning stage (WFL-014), and says so in the
// session. It says why when the session cannot have that model; nothing then changes.
func (e *Engine) planningModel(ctx context.Context, ses session.Session, w config.Workflow) (string, error) {
	set, ok := e.workflowModelSet(w.ID)
	if !ok || !set.Planning.Fixed() {
		return "", nil
	}
	row := set.Planning
	report, err := e.SessionOptions(ctx, ses.ID)
	if err != nil {
		return "", err
	}
	option := func(category string) (harness.Option, bool) {
		i := slices.IndexFunc(report.Options, func(o harness.Option) bool { return o.Category == category })
		if i < 0 {
			return harness.Option{}, false
		}
		return report.Options[i], true
	}
	want := row.Value()
	model, ok := option(harness.OptionModel)
	cannot := func(why string) string {
		return fmt.Sprintf("The model set %s plans on %s, which this session cannot have: %s.", set.Name, row.Model, why)
	}
	switch {
	case !ok:
		return cannot("its agent offers no choice of model"), nil
	case model.Current == want:
	case slices.ContainsFunc(model.Choices, func(c harness.OptionChoice) bool { return c.Value == want }):
	// A model named by hand is taken where the agent takes one, and of the same kind:
	// the built-in agent's names its provider, an external agent's does not.
	case model.Custom && strings.Contains(want, modelSeparator) == strings.Contains(model.Current, modelSeparator):
	default:
		return cannot("its agent does not offer it"), nil
	}
	if why := e.modelUnavailable(row); why != "" {
		return cannot(why), nil
	}
	changed := false
	if model.Current != want {
		if err := e.SetSessionOption(ctx, ses.ID, model.ID, want); err != nil {
			return "", err
		}
		changed = true
		if report, err = e.SessionOptions(ctx, ses.ID); err != nil {
			return "", err
		}
	}
	effort := ""
	if eff, ok := option(harness.OptionEffort); ok && row.Effort != "" && slices.ContainsFunc(eff.Choices, func(c harness.OptionChoice) bool { return c.Value == row.Effort }) {
		effort = row.Effort
		if eff.Current != row.Effort {
			if err := e.SetSessionOption(ctx, ses.ID, eff.ID, row.Effort); err != nil {
				return "", err
			}
			changed = true
		}
	}
	if changed {
		name := row.Model
		if effort != "" {
			name += " · " + effort
		}
		e.emit(ses.ID, event.Notice, event.NoticeData{Message: fmt.Sprintf("Switched to %s for this stage (model set %s).", name, set.Name)})
	}
	return "", nil
}

// skillsReady says why a stage cannot use its skill: missing, or turned off for the
// session.
func (e *Engine) skillsReady(ctx context.Context, ses session.Session, w config.Workflow, name string) error {
	if name == "" {
		return nil
	}
	project := e.sessionProject(ctx, ses)
	prof, err := e.profileFor(project, ses.ProfileID)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(e.enabledSkills(project, prof), func(s skill.Skill) bool { return s.Name == name }) {
		return nil
	}
	if config.BuiltinWorkflowSkills(w.ID)[name] != nil {
		return fmt.Errorf("this stage uses the skill %s, which is not installed or is turned off. Install %s's skills in Settings, under Workflows", name, w.Name)
	}
	return fmt.Errorf("this stage uses the skill %s, which is not installed or is turned off for this session", name)
}

// stageVars are what a stage's template can say.
func (e *Engine) stageVars(ctx context.Context, ses session.Session, w config.Workflow, input string) map[string]string {
	vars := map[string]string{"input": input, "backlog": w.BacklogDir(), "issues": "(none yet)", "commits": "(none)"}
	// An untrusted project's git config could run commands (SEC-006).
	env, err := e.localEnv(ses.WorkRoot(), nil)
	if err != nil || !e.trusted(ses.ProjectID) {
		return vars
	}
	if log := recentCommits(ctx, env); log != "" {
		vars["commits"] = log
	}
	vars["issues"] = e.backlogSummary(ctx, ses, w)
	return vars
}

// recentCommits are the last commits, one line each, when the project is a repository.
func recentCommits(ctx context.Context, env environment.Environment) string {
	out, ok := git(ctx, env, "log", "--oneline", "--no-decorate", "-n", "10")
	if !ok {
		return ""
	}
	return strings.TrimSpace(out)
}
