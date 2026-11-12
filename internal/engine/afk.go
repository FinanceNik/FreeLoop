package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	goruntime "runtime"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/backlog"
	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// The AFK run (WFL-005, WFL-006): a workflow's unattended issues done one at a time,
// each by a fresh session queued through SES-025, so the queue's limit, provider
// slots, account limits and budgets hold. The sessions share one worktree of the run
// (OQ-040). Nothing is approved for them but the commands the user allowed when
// starting it, as each session's own rules (SEC-003); every other request waits in
// the attention view. An issue is done when its file says so in the worktree, for
// every agent alike (HRN-001). Then the run commits what is left, has the review
// profile review it, and lists it as ready for the user's QA.

// AFKTag marks the sessions an AFK run starts.
const AFKTag = "afk"

// runs loads the runs.
func (e *Engine) runs() (config.AFKRuns, error) {
	e.afkMu.Lock()
	defer e.afkMu.Unlock()
	return config.LoadAFKRuns(e.dataDir)
}

// saveRuns writes the runs; the caller holds afkMu.
func (e *Engine) saveRuns(all config.AFKRuns) error {
	if e.afkSave != nil {
		return e.afkSave(e.dataDir, all)
	}
	return config.SaveAFKRuns(e.dataDir, all)
}

// changeRun loads the runs, lets fn change the one with id and saves them.
func (e *Engine) changeRun(id string, fn func(r *config.AFKRun) error) (config.AFKRun, error) {
	e.afkMu.Lock()
	defer e.afkMu.Unlock()
	all, err := config.LoadAFKRuns(e.dataDir)
	if err != nil {
		return config.AFKRun{}, err
	}
	i := slices.IndexFunc(all.Runs, func(r config.AFKRun) bool { return r.ID == id })
	if i < 0 {
		return config.AFKRun{}, errors.New("no such run")
	}
	if err := fn(&all.Runs[i]); err != nil {
		return all.Runs[i], err
	}
	if err := e.saveRuns(all); err != nil {
		return all.Runs[i], err
	}
	if e.onQueue != nil {
		e.onQueue()
	}
	return all.Runs[i], nil
}

// AFKRuns lists the runs, newest first.
func (e *Engine) AFKRuns(ctx context.Context) ([]config.AFKRun, error) {
	all, err := e.runs()
	if err != nil {
		return nil, err
	}
	slices.Reverse(all.Runs)
	return all.Runs, nil
}

// lastRun is a project's run going on, else its latest.
func (e *Engine) lastRun(projectID string) *config.AFKRun {
	all, err := e.runs()
	if err != nil {
		return nil
	}
	var last *config.AFKRun
	for i := range all.Runs {
		r := &all.Runs[i]
		if r.ProjectID != projectID {
			continue
		}
		if r.Active() {
			return r
		}
		last = r
	}
	return last
}

// AFKCommand is a command an AFK run will want, and what the policy says of it now.
type AFKCommand struct {
	// Command is a shell rule pattern, as "git commit *".
	Command string `json:"command"`
	// From is "workflow", "project" or "project-more": the workflow names it, the
	// project's files suggest it as its tests or checks, or as another check
	// target, which is offered but not ticked.
	From string `json:"from"`
	// Outcome is allowed, ask or denied, for a session of the run as it would start.
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

// AFKPlan is what starting a run would do, for the user to look at first.
type AFKPlan struct {
	ProjectRoot  string `json:"projectRoot"`
	ProjectName  string `json:"projectName"`
	WorkflowID   string `json:"workflowId"`
	WorkflowName string `json:"workflowName"`
	// Next is the issue it would do first; Ready counts the AFK issues ready now,
	// Waiting those blocked, and Human the open issues for a person.
	Next    *backlog.Item `json:"next"`
	Ready   int           `json:"ready"`
	Waiting int           `json:"waiting"`
	Human   int           `json:"human"`
	// Problem says why it cannot start; empty when it can.
	Problem         string       `json:"problem"`
	ProfileID       string       `json:"profileId"`
	ReviewProfileID string       `json:"reviewProfileId"`
	ReviewOff       bool         `json:"reviewOff"`
	EnvironmentID   string       `json:"environmentId"`
	EnvironmentNote string       `json:"environmentNote"`
	Mode            string       `json:"mode"`
	MaxIssues       int          `json:"maxIssues"`
	Commands        []AFKCommand `json:"commands"`
	// Running is the project's run going on, if any.
	Running *config.AFKRun `json:"running"`
	// QueuePaused says the queue is paused, so the run's issues wait until it is
	// resumed (SES-025).
	QueuePaused bool `json:"queuePaused"`
	// Held says why the run's first issue would wait now, such as its agent's
	// limit (SES-025); empty when it would start.
	Held string `json:"held"`
	// ModelSet is the workflow's model set, which the dialog starts from, with why
	// a row's model cannot be had, by row (WFL-014).
	ModelSet            *config.ModelSet  `json:"modelSet"`
	ModelSetUnavailable map[string]string `json:"modelSetUnavailable"`
}

// projectChecks are the commands a project's files suggest for its tests and type
// checks, as rule patterns. Only files at its top are read.
func projectChecks(env environment.Environment) []string {
	read := func(p string) string {
		b, err := env.FS().ReadFile(p)
		if err != nil || len(b) > 1<<20 {
			return ""
		}
		return string(b)
	}
	has := func(p string) bool { _, err := env.FS().Stat(p); return err == nil }
	var out []string
	if mk := read("Makefile"); mk != "" {
		for _, target := range []string{"test", "check", "lint", "typecheck"} {
			if strings.HasPrefix(mk, target+":") || strings.Contains(mk, "\n"+target+":") {
				out = append(out, "make "+target)
			}
		}
	}
	if pkg := read("package.json"); pkg != "" {
		var p struct {
			Scripts map[string]string `json:"scripts"`
		}
		json.Unmarshal([]byte(pkg), &p)
		pm := "npm"
		switch {
		case has("pnpm-lock.yaml"):
			pm = "pnpm"
		case has("yarn.lock"):
			pm = "yarn"
		}
		for _, s := range []string{"test", "typecheck", "lint", "check"} {
			if p.Scripts[s] == "" {
				continue
			}
			if s == "test" && pm != "yarn" {
				out = append(out, pm+" test")
			} else if pm == "yarn" {
				out = append(out, "yarn "+s)
			} else {
				out = append(out, pm+" run "+s)
			}
		}
	}
	if has("go.mod") {
		out = append(out, "go test *", "go vet *", "go build *")
	}
	if has("Cargo.toml") {
		out = append(out, "cargo test *", "cargo check *")
	}
	if has("pyproject.toml") || has("pytest.ini") || has("setup.cfg") {
		out = append(out, "pytest *")
	}
	return out
}

// extraCommand checks a command the user typed in for a run to allow: one line, and
// never everything at once.
func extraCommand(c string) (string, error) {
	c = strings.Join(strings.Fields(c), " ")
	switch {
	case c == "":
		return "", errors.New("an empty command cannot be allowed")
	case strings.HasPrefix(c, "*"):
		return "", fmt.Errorf("%q would allow any command; name the program it starts with", c)
	case strings.ContainsAny(c, "\n\r"):
		return "", errors.New("a command to allow is one line")
	}
	return c, nil
}

// moreTargets are Makefile targets named after a check, such as check-app or
// test-keychain: offered for a run to allow, not ticked, since what they touch is
// not known.
var moreTargets = regexp.MustCompile(`(?m)^((?:test|check|lint|typecheck)-[A-Za-z0-9_-]+):`)

// projectMoreChecks are the project's other check targets, as rule patterns.
func projectMoreChecks(env environment.Environment) []string {
	b, err := env.FS().ReadFile("Makefile")
	if err != nil || len(b) > 1<<20 {
		return nil
	}
	var out []string
	for _, m := range moreTargets.FindAllStringSubmatch(string(b), -1) {
		if c := "make " + m[1]; !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// example is a command a rule pattern matches: "git commit *" matches "git commit".
func example(pattern string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(pattern), "*"))
}

// afkEnvironment is where a run's sessions go: the sandboxed environment when the
// workflow asks for it and the project's sessions run locally, else the project's.
func (e *Engine) afkEnvironment(project session.Project, w config.Workflow) (string, string) {
	envID := cmp.Or(project.DefaultEnvironmentID, e.Config().Defaults.EnvironmentID, DefaultEnvironmentID)
	if !w.AFK.Sandbox {
		return envID, "Not sandboxed: the workflow does not ask for it."
	}
	if envID != DefaultEnvironmentID && envID != SandboxedEnvironmentID {
		return envID, "Not sandboxed: the project's sessions run in " + envID + "."
	}
	if e.confiner == nil {
		return envID, "Not sandboxed: " + noSandboxReason(goruntime.GOOS) + "."
	}
	if _, err := e.usableEnvironment(SandboxedEnvironmentID); err != nil {
		return envID, "Not sandboxed: " + err.Error()
	}
	return SandboxedEnvironmentID, "Sandboxed: commands write only in the run's worktree and reach no network."
}

// PrepareAFK says what a run of the project's backlog would do, without starting it.
func (e *Engine) PrepareAFK(ctx context.Context, root, workflowID string) (AFKPlan, error) {
	project, err := e.store.ProjectByRoot(ctx, environment.Canonical(root))
	if err != nil {
		project, err = e.projectOfWorktree(ctx, root)
		if err != nil {
			return AFKPlan{}, err
		}
	}
	w, err := e.backlogWorkflow(project, workflowID)
	if err != nil {
		return AFKPlan{}, err
	}
	plan := AFKPlan{ProjectRoot: project.Root, ProjectName: project.Name, WorkflowID: w.ID, WorkflowName: w.Name, Mode: w.AFKMode(), MaxIssues: w.IssueCap(), Commands: []AFKCommand{}, ReviewOff: w.Review.Off}
	if !slices.ContainsFunc(w.Stages, func(s config.WorkflowStage) bool { return s.StageKind() == config.StageAFK }) {
		plan.Problem = w.Name + " has no AFK stage."
		return plan, nil
	}
	if set, ok := e.workflowModelSet(w.ID); ok {
		plan.ModelSet, plan.ModelSetUnavailable = &set, map[string]string{}
		for name, row := range set.Rows() {
			if why := e.modelUnavailable(row); why != "" {
				plan.ModelSetUnavailable[name] = why
			}
		}
	}
	plan.ProfileID = e.afkProfile(project, w.AFK.Profile)
	plan.ReviewProfileID = e.afkProfile(project, cmp.Or(w.Review.Profile, plan.ProfileID))
	plan.EnvironmentID, plan.EnvironmentNote = e.afkEnvironment(project, w)
	if q, err := e.Queue(); err == nil {
		plan.QueuePaused = q.Paused
	}
	plan.Held = e.limitNote(ctx, project.Root, plan.ProfileID)
	if r := e.lastRun(project.ID); r != nil && r.Active() {
		plan.Running = r
		plan.Problem = "A run is going on in this project; pause or stop it first."
	}
	info, err := e.projectBacklog(ctx, project, project.Root, w)
	if err != nil {
		plan.Problem = cmp.Or(plan.Problem, err.Error())
		return plan, nil
	}
	for _, it := range info.Issues {
		switch {
		case it.State == backlog.Done:
		case it.Type != backlog.TypeAFK || it.Problem != "":
			plan.Human++
		case it.State == backlog.Ready:
			plan.Ready++
		default:
			plan.Waiting++
		}
	}
	if next, ok := backlog.NextAFK(info.Issues, nil); ok {
		plan.Next = &next
	}
	switch {
	case plan.Problem != "":
	case info.Uncommitted:
		plan.Problem = fmt.Sprintf("Commit %s first: the run works on a branch from the last commit, and would not see what is not committed.", w.BacklogDir())
	case plan.Next == nil:
		plan.Problem = "No AFK issue is ready: each is done, blocked, or for a person."
	}
	env, err := e.localEnv(project.Root, nil)
	if err != nil {
		return plan, nil
	}
	seen := map[string]bool{}
	add := func(c, from string) {
		if seen[c] {
			return
		}
		seen[c] = true
		cmd := AFKCommand{Command: c, From: from}
		check, err := e.CheckPolicy(ctx, PolicyQuery{ProjectRoot: project.Root, ProfileID: plan.ProfileID, EnvironmentID: plan.EnvironmentID, Mode: plan.Mode, Tool: "shell", Command: example(c)})
		if err == nil {
			cmd.Outcome, cmd.Reason = check.Outcome, check.Reason
		}
		plan.Commands = append(plan.Commands, cmd)
	}
	for _, c := range w.AFK.Commands {
		add(c, "workflow")
	}
	for _, c := range projectChecks(env) {
		add(c, "project")
	}
	for _, c := range projectMoreChecks(env) {
		add(c, "project-more")
	}
	return plan, nil
}

// afkProfile is the profile a run's sessions use: id when it exists, else the
// project's default, else the app's.
func (e *Engine) afkProfile(project session.Project, id string) string {
	if id != "" {
		if _, err := e.profileFor(project, id); err == nil {
			return id
		}
	}
	return cmp.Or(project.DefaultProfileID, e.Config().Defaults.ProfileID, config.DefaultProfileID)
}

// AFKStart starts a run.
type AFKStart struct {
	ProjectRoot     string `json:"projectRoot"`
	WorkflowID      string `json:"workflowId"`
	ProfileID       string `json:"profileId"`
	ReviewProfileID string `json:"reviewProfileId"`
	// Allow are commands of the plan the user allows for the run's sessions.
	Allow []string `json:"allow"`
	// Extra are commands the user typed in to allow as well, such as a check the
	// plan did not find (WFL-009).
	Extra []string `json:"extra"`
	// Model and ReviewModel are values of ProfileModels' choices for the two
	// profiles; empty keeps each profile's own model (WFL-010).
	Model       string `json:"model"`
	ReviewModel string `json:"reviewModel"`
	// Effort and ReviewEffort are levels ModelEfforts offers for those models; empty
	// keeps each profile's (WFL-012).
	Effort       string `json:"effort"`
	ReviewEffort string `json:"reviewEffort"`
	// StartedBy says where it was started, for the record.
	StartedBy string `json:"startedBy"`
	// FromSession is the session it was started from, where it was planned (WFL-015).
	FromSession string `json:"fromSession"`
}

// StartAFK starts a run of a project's backlog: its first issue is queued at once.
func (e *Engine) StartAFK(ctx context.Context, req AFKStart) (config.AFKRun, error) {
	plan, err := e.PrepareAFK(ctx, req.ProjectRoot, req.WorkflowID)
	if err != nil {
		return config.AFKRun{}, err
	}
	if plan.Problem != "" {
		return config.AFKRun{}, errors.New(plan.Problem)
	}
	project, err := e.store.ProjectByRoot(ctx, environment.Canonical(plan.ProjectRoot))
	if err != nil {
		return config.AFKRun{}, err
	}
	// Only what the plan offered can be allowed up front: it is what the user saw.
	for _, c := range req.Allow {
		if !slices.ContainsFunc(plan.Commands, func(x AFKCommand) bool { return x.Command == c }) {
			return config.AFKRun{}, fmt.Errorf("%q is not among the commands the run offered to allow", c)
		}
	}
	allow := slices.Clone(req.Allow)
	for _, c := range req.Extra {
		c, err := extraCommand(c)
		if err != nil {
			return config.AFKRun{}, err
		}
		if !slices.Contains(allow, c) {
			allow = append(allow, c)
		}
	}
	prof := plan.ProfileID
	if req.ProfileID != "" {
		if _, err := e.profileFor(project, req.ProfileID); err != nil {
			return config.AFKRun{}, err
		}
		prof = req.ProfileID
	}
	review := plan.ReviewProfileID
	if req.ReviewProfileID != "" {
		if _, err := e.profileFor(project, req.ReviewProfileID); err != nil {
			return config.AFKRun{}, err
		}
		review = req.ReviewProfileID
	}
	// A model is one the profile's sessions can be switched to, as offered; its label
	// names it, and its provider, for the Board.
	pick := func(profileID, value string) (string, string, error) {
		if value == "" {
			return "", "", nil
		}
		m, err := e.ProfileModels(ctx, project.Root, profileID)
		if err != nil {
			return "", "", err
		}
		if !m.offers(value) {
			return "", "", fmt.Errorf("%s is not among the models offered for that profile", modelName(value))
		}
		return m.OptionID, m.label(value), nil
	}
	modelOption, modelLabel, err := pick(prof, req.Model)
	if err != nil {
		return config.AFKRun{}, err
	}
	reviewOption, reviewLabel, err := pick(review, req.ReviewModel)
	if err != nil {
		return config.AFKRun{}, err
	}
	effortOption, effort, err := e.runEffort(ctx, project.Root, prof, req.Model, req.Effort)
	if err != nil {
		return config.AFKRun{}, err
	}
	reviewEffortOption, reviewEffort, err := e.runEffort(ctx, project.Root, review, req.ReviewModel, req.ReviewEffort)
	if err != nil {
		return config.AFKRun{}, err
	}
	run := config.AFKRun{
		ModelOption: modelOption, Model: req.Model, ReviewModelOption: reviewOption, ReviewModel: req.ReviewModel,
		EffortOption: effortOption, Effort: effort, ReviewEffortOption: reviewEffortOption, ReviewEffort: reviewEffort,
		ModelLabel: modelLabel, ReviewModelLabel: reviewLabel,
		PlanningSession: e.planningSession(ctx, req.FromSession, project.ID),
		ID:              fmt.Sprintf("afk_%d", time.Now().UnixNano()), WorkflowID: plan.WorkflowID, ProjectID: project.ID, ProjectRoot: project.Root,
		ProfileID: prof, ReviewProfileID: review, EnvironmentID: plan.EnvironmentID, Mode: plan.Mode, Allow: allow,
		MaxIssues: plan.MaxIssues, State: config.RunRunning, Steps: []config.AFKStep{}, StartedBy: req.StartedBy, StartedAt: time.Now().UTC(),
	}
	w, _ := e.findWorkflow(plan.WorkflowID)
	run.Backlog = w.BacklogDir()
	e.afkMu.Lock()
	all, err := config.LoadAFKRuns(e.dataDir)
	if err == nil {
		if slices.ContainsFunc(all.Runs, func(r config.AFKRun) bool { return r.ProjectID == project.ID && r.Active() }) {
			err = errors.New("a run is going on in this project")
		} else {
			all.Runs = append(all.Runs, run)
			err = e.saveRuns(all)
		}
	}
	e.afkMu.Unlock()
	if err != nil {
		return config.AFKRun{}, err
	}
	e.KickRuns()
	return run, nil
}

// PauseAFK pauses a run, which then starts no further issue, or resumes it. The
// issue being worked on goes on.
func (e *Engine) PauseAFK(ctx context.Context, id string, paused bool) (config.AFKRun, error) {
	r, err := e.changeRun(id, func(r *config.AFKRun) error {
		if !r.Active() {
			return errors.New("the run has ended")
		}
		r.State = config.RunRunning
		if paused {
			r.State = config.RunPaused
		}
		return nil
	})
	if err == nil && !paused {
		e.KickRuns()
	}
	return r, err
}

// StopAFK ends a run: a queued issue that has not started is taken off the queue,
// and a session already working goes on until its turn ends, unfollowed.
func (e *Engine) StopAFK(ctx context.Context, id string) (config.AFKRun, error) {
	// Not between a tick's two saves of a step: a task made after this would stay.
	e.afkStep.Lock()
	defer e.afkStep.Unlock()
	var task, starting string
	r, err := e.changeRun(id, func(r *config.AFKRun) error {
		if !r.Active() {
			return errors.New("the run has ended")
		}
		if s := r.Current(); s != nil && s.SessionID == "" {
			task = s.TaskID
			if s.Starting {
				starting = "AFK " + s.Issue + ": "
			}
			s.State, s.Note, s.EndedAt = config.StepFailed, "the run was stopped before it started", time.Now().UTC()
		}
		endRun(r, config.RunStopped, "You stopped it.")
		return nil
	})
	if err == nil && task == "" && starting != "" {
		// Saved as starting: its task may have been made before the save of it.
		if q, qerr := e.Queue(); qerr == nil {
			for _, t := range q.Tasks {
				if t.RunID == id && strings.HasPrefix(t.Title, starting) {
					task = t.ID
				}
			}
		}
	}
	if err == nil && task != "" {
		_, rerr := e.RemoveTask(task)
		afkWarn(rerr)
	}
	return r, err
}

// QADone marks a finished issue as checked by the user, which takes it out of the
// attention view.
func (e *Engine) QADone(ctx context.Context, runID, issue string) error {
	_, err := e.changeRun(runID, func(r *config.AFKRun) error {
		for i := range r.Steps {
			if r.Steps[i].Issue == issue && r.Steps[i].State == config.StepQA {
				r.Steps[i].State = config.StepQADone
				return nil
			}
		}
		return errors.New("that issue is not waiting for QA")
	})
	return err
}

func endRun(r *config.AFKRun, state, reason string) {
	r.State, r.Reason, r.EndedAt = state, reason, time.Now().UTC()
}

// KickRuns has the schedule loop look at the queue and the runs now, rather than at
// its next tick.
func (e *Engine) KickRuns() {
	select {
	case e.afkKick <- struct{}{}:
	default:
	}
}

// runAFK moves every active run on: follows its issue's session, then its review,
// then queues the next issue, or ends the run.
func (e *Engine) runAFK(ctx context.Context, now time.Time) {
	e.afkStep.Lock()
	defer e.afkStep.Unlock()
	all, err := e.runs()
	if err != nil {
		log.Printf("engine: afk runs: %v", err)
		return
	}
	for _, r := range all.Runs {
		if r.Active() {
			e.stepRun(ctx, r.ID, now)
		}
	}
}

// stepRun moves one run on by what has happened since.
func (e *Engine) stepRun(ctx context.Context, id string, now time.Time) {
	all, err := e.runs()
	if err != nil {
		return
	}
	i := slices.IndexFunc(all.Runs, func(r config.AFKRun) bool { return r.ID == id })
	if i < 0 {
		return
	}
	r := all.Runs[i]
	w, werr := e.usableWorkflow(r.WorkflowID)
	if werr != nil {
		e.endRunNow(id, config.RunStopped, werr.Error())
		return
	}
	cur := r.Current()
	switch {
	case cur != nil && cur.State == config.StepWorking:
		e.followIssue(ctx, r, *cur, w, now)
	case cur != nil && cur.State == config.StepReviewing:
		e.followReview(ctx, r, *cur, w)
	case r.State == config.RunRunning:
		e.nextIssue(ctx, r, w, now)
	}
}

// afkWarn logs a save that failed where the next tick looks again and tries again.
func afkWarn(err error) {
	if err != nil {
		log.Printf("engine: afk runs: %v", err)
	}
}

// endRunNow ends the run with a reason; a save that fails is logged, and the run
// goes on, so the next tick ends it again.
func (e *Engine) endRunNow(id, state, reason string) {
	_, err := e.changeRun(id, func(r *config.AFKRun) error { endRun(r, state, reason); return nil })
	afkWarn(err)
}

// trySetStep changes the run's current step and says whether it was saved.
func (e *Engine) trySetStep(id string, fn func(r *config.AFKRun, s *config.AFKStep)) error {
	_, err := e.changeRun(id, func(r *config.AFKRun) error {
		if s := r.Current(); s != nil {
			fn(r, s)
		}
		return nil
	})
	return err
}

// setStep changes the run's current step where a save that fails can wait for the
// next tick, which looks at the step again.
func (e *Engine) setStep(id string, fn func(r *config.AFKRun, s *config.AFKStep)) {
	afkWarn(e.trySetStep(id, fn))
}

// recoverStarting settles a step saved as Starting whose task was not recorded: the
// run's task for the issue is the step's if the queue has it, and if not the step is
// taken back so the issue is queued once, now. It says whether the step has its task.
func (e *Engine) recoverStarting(r config.AFKRun, s config.AFKStep, q config.Queue) bool {
	prefix := "AFK " + s.Issue + ": "
	if i := slices.IndexFunc(q.Tasks, func(t config.Task) bool { return t.RunID == r.ID && strings.HasPrefix(t.Title, prefix) }); i >= 0 {
		task := q.Tasks[i].ID
		e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) { st.TaskID, st.Starting = task, false })
		return true
	}
	afkWarn(e.changeRunErr(r.ID, func(r *config.AFKRun) error {
		if n := len(r.Steps); n > 0 && r.Steps[n-1].Starting && r.Steps[n-1].Issue == s.Issue && r.Steps[n-1].TaskID == "" {
			r.Steps = r.Steps[:n-1]
		}
		return nil
	}))
	return false
}

// changeRunErr is changeRun for a caller that wants only the error.
func (e *Engine) changeRunErr(id string, fn func(r *config.AFKRun) error) error {
	_, err := e.changeRun(id, fn)
	return err
}

// followIssue follows the session working on an issue, through the queue's task.
func (e *Engine) followIssue(ctx context.Context, r config.AFKRun, s config.AFKStep, w config.Workflow, now time.Time) {
	q, err := e.Queue()
	if err != nil {
		return
	}
	if s.Starting {
		// Saved before its task was made, and the run was cut off since: find the
		// task, or take the step back so the issue is queued once.
		if e.recoverStarting(r, s, q) {
			e.KickRuns()
		}
		return
	}
	i := slices.IndexFunc(q.Tasks, func(t config.Task) bool { return t.ID == s.TaskID })
	if i < 0 {
		e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) {
			st.State, st.Note, st.EndedAt = config.StepFailed, "its task left the queue", time.Now().UTC()
			endRun(r, config.RunStopped, fmt.Sprintf("The task for %s was taken off the queue.", s.Issue))
		})
		return
	}
	t := q.Tasks[i]
	if t.State == config.TaskWaiting {
		// Why it has not started, on the run where the Board shows it.
		waiting := cmp.Or(t.Note, "waiting for room in the queue")
		if q.Paused {
			waiting = "the queue is paused"
		}
		if waiting != s.Waiting {
			e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) { st.Waiting = waiting })
		}
	} else if s.Waiting != "" {
		e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) { st.Waiting = "" })
	}
	switch t.State {
	case config.TaskWaiting:
		// A budget reached holds the task; the run stops rather than wait for a new
		// period, which may be next month.
		budgets, _ := e.budgetStates(ctx, now)
		if note := budgetHeld(budgets, r.ProfileID); note != "" {
			e.RemoveTask(t.ID)
			e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) {
				st.State, st.Note, st.EndedAt = config.StepFailed, "not started: "+note, time.Now().UTC()
				endRun(r, config.RunStopped, "A budget is reached: "+note+".")
			})
		}
	case config.TaskRunning:
		if t.SessionID != "" && s.SessionID == "" {
			e.adoptSession(ctx, r.ID, t.SessionID)
		}
	case config.TaskFailed:
		e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) {
			st.SessionID = cmp.Or(st.SessionID, t.SessionID)
			st.State, st.Note, st.EndedAt = config.StepFailed, t.Note, time.Now().UTC()
			endRun(r, config.RunStopped, fmt.Sprintf("The session for %s failed: %s", s.Issue, cmp.Or(t.Note, "see the session")))
		})
	case config.TaskDone:
		if s.SessionID == "" {
			e.adoptSession(ctx, r.ID, t.SessionID)
		}
		e.finishIssue(ctx, r.ID, t.SessionID, w)
	}
}

// planningSession is the session a run was started from, when it is of the run's
// project.
func (e *Engine) planningSession(ctx context.Context, sid, projectID string) string {
	if sid == "" {
		return ""
	}
	if ses, err := e.store.GetSession(ctx, sid); err == nil && ses.ProjectID == projectID {
		return sid
	}
	return ""
}

// adoptSession records the session working on the current issue, and the first
// one's worktree as the run's.
func (e *Engine) adoptSession(ctx context.Context, id, sid string) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return
	}
	afkWarn(e.changeRunErr(id, func(r *config.AFKRun) error {
		if s := r.Current(); s != nil {
			s.SessionID = sid
		}
		if r.Anchor == "" && ses.Worktree != "" {
			r.Anchor, r.Worktree, r.Branch = sid, ses.Worktree, ses.OwnBranch()
		}
		return nil
	}))
}

// finishIssue looks at an issue whose session has ended: done by its file, its
// leftovers committed, then reviewed. A session that ended without marking it done
// stops the run: the next issue may depend on it.
func (e *Engine) finishIssue(ctx context.Context, id, sid string, w config.Workflow) {
	all, _ := e.runs()
	i := slices.IndexFunc(all.Runs, func(r config.AFKRun) bool { return r.ID == id })
	if i < 0 {
		return
	}
	r := all.Runs[i]
	s := r.Current()
	if s == nil {
		return
	}
	stop := func(note, reason string) {
		e.setStep(id, func(r *config.AFKRun, st *config.AFKStep) {
			st.State, st.Note, st.EndedAt = config.StepFailed, note, time.Now().UTC()
			endRun(r, config.RunStopped, reason)
		})
	}
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		stop(err.Error(), fmt.Sprintf("The session for %s is gone.", s.Issue))
		return
	}
	env, err := e.localEnv(ses.WorkRoot(), nil)
	if err != nil {
		stop(err.Error(), fmt.Sprintf("The folder of %s cannot be read.", s.Issue))
		return
	}
	b, err := env.FS().ReadFile(s.Path)
	if err != nil {
		stop("its file is gone", fmt.Sprintf("%s's file is gone from the run's worktree.", s.Issue))
		return
	}
	if is := backlog.ParseIssue(s.Path, b); is.Status != backlog.StatusDone {
		why := e.endedBecause(ctx, sid)
		stop("its session "+why, fmt.Sprintf("%s's session %s; see the session for more.", s.Issue, why))
		return
	}
	commit, note := e.commitLeftovers(ctx, env, *s)
	e.stopFinished(sid)
	// Then a second agent reviews it, before the next issue changes the worktree it
	// reads (WFL-006).
	if !w.Review.Off {
		// The step is saved as reviewing before the review is made, and a save that
		// fails makes no review: a second one would be a second paid session.
		if err := e.trySetStep(id, func(r *config.AFKRun, st *config.AFKStep) {
			st.Commit, st.Note, st.State, st.Starting = commit, note, config.StepReviewing, true
		}); err != nil {
			stop("its review could not be recorded: "+err.Error(), fmt.Sprintf("The run could not save %s's review step: %v", s.Issue, err))
			return
		}
		e.startReview(ctx, id, *s, sid, w, env, false)
		return
	}
	e.setStep(id, func(r *config.AFKRun, st *config.AFKStep) {
		st.Commit, st.Note, st.State, st.EndedAt = commit, note, config.StepQA, time.Now().UTC()
	})
	e.KickRuns()
}

// startReview makes the review of the step saved as Starting and records it; with
// recovering, a review its session already has for the issue is the step's.
func (e *Engine) startReview(ctx context.Context, id string, s config.AFKStep, sid string, w config.Workflow, env environment.Environment, recovering bool) {
	if recovering {
		for _, c := range e.subagentsOf(ctx, sid) {
			if c.Title == "Review: "+s.Issue && slices.Contains(c.Tags, ReviewTag) {
				e.setStep(id, func(r *config.AFKRun, st *config.AFKStep) { st.ReviewID, st.Starting = c.ID, false })
				return
			}
		}
	}
	all, err := e.runs()
	if err != nil {
		return
	}
	i := slices.IndexFunc(all.Runs, func(r config.AFKRun) bool { return r.ID == id })
	if i < 0 {
		return
	}
	reviewer, err := e.reviewIssue(ctx, all.Runs[i], s, sid, w, env)
	if err == nil {
		e.setStep(id, func(r *config.AFKRun, st *config.AFKStep) { st.ReviewID, st.Starting = reviewer, false })
		return
	}
	e.setStep(id, func(r *config.AFKRun, st *config.AFKStep) {
		st.Note = strings.TrimSpace(st.Note + " Not reviewed: " + err.Error())
		st.State, st.Starting, st.EndedAt = config.StepQA, false, time.Now().UTC()
	})
	e.KickRuns()
}

// endedBecause says, for an issue that is not done, how its session's last turn ended,
// so the run does not call a cancelled or failed turn one that merely forgot to mark
// the issue done.
func (e *Engine) endedBecause(ctx context.Context, sid string) string {
	evs, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return "ended without marking it done"
	}
	stop, problem := "", ""
	for _, ev := range evs {
		switch ev.Type {
		case event.UserMessageChunk:
			problem = ""
		case event.Error:
			var d event.ErrorData
			if json.Unmarshal(ev.Data, &d) == nil {
				problem = d.Message
			}
		case event.TurnEnded:
			var d event.TurnEndedData
			if json.Unmarshal(ev.Data, &d) == nil {
				stop = d.StopReason
			}
		}
	}
	switch harness.StopReason(stop) {
	case harness.StopCancelled:
		return "was stopped before it marked the issue done"
	case harness.StopError:
		return "failed before it marked the issue done" + withProblem(problem)
	case harness.StopMaxTokens:
		return "ran out of output room before it marked the issue done"
	case harness.StopMaxTurns:
		return "reached its limit of steps before it marked the issue done"
	case harness.StopRefusal:
		return "was refused by the model before it marked the issue done"
	}
	if problem != "" {
		return "hit a problem before it marked the issue done" + withProblem(problem)
	}
	return "ended without marking it done"
}

// withProblem is an error's text as a clause that follows a sentence.
func withProblem(problem string) string {
	if problem = strings.TrimSpace(problem); problem == "" {
		return ""
	}
	return " (" + strings.TrimRight(problem, ".") + ")"
}

// commitLeftovers commits what an issue's session left uncommitted and returns the
// commit the issue ends at, with a note when it committed.
func (e *Engine) commitLeftovers(ctx context.Context, env environment.Environment, s config.AFKStep) (string, string) {
	note := ""
	if blocked(ctx, env) == "" {
		if out, ok := git(ctx, env, "status", "--porcelain"); ok && strings.TrimSpace(out) != "" {
			msg := fmt.Sprintf("%s: %s", s.Issue, s.Title)
			before, _ := git(ctx, env, "rev-parse", "--verify", "--quiet", "HEAD")
			if _, err := gitOK(ctx, env, "add", "--all"); err == nil {
				if _, err := gitOK(ctx, env, "commit", "--quiet", "-m", msg); err == nil {
					note = "Its session left changes uncommitted; the run committed them."
					// The issue's work, so its session's (GUI-051).
					if s.SessionID != "" {
						e.committed(ctx, s.SessionID, env, strings.TrimSpace(before), true)
					}
				}
			}
		}
	}
	head, _ := git(ctx, env, "rev-parse", "HEAD")
	return strings.TrimSpace(head), note
}

// followReview follows the review of the current issue; once it has answered, or
// failed, the issue waits for the user's QA.
func (e *Engine) followReview(ctx context.Context, r config.AFKRun, s config.AFKStep, w config.Workflow) {
	if s.Starting {
		// Saved before the review was made, and the run was cut off since (or the
		// save of its session failed): the session the issue has for it, or one.
		ses, err := e.store.GetSession(ctx, s.SessionID)
		if err == nil {
			env, err := e.localEnv(ses.WorkRoot(), nil)
			if err == nil {
				e.startReview(ctx, r.ID, s, s.SessionID, w, env, true)
				return
			}
		}
		e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) {
			st.Note = strings.TrimSpace(st.Note + " Not reviewed: its session cannot be read.")
			st.State, st.Starting, st.EndedAt = config.StepQA, false, time.Now().UTC()
		})
		return
	}
	ses, err := e.store.GetSession(ctx, s.ReviewID)
	note := ""
	switch {
	case err != nil:
		note = "Its review session is gone."
	case ses.Status == session.StatusError:
		note = "Its review failed; see the review session."
	case ses.Status == session.StatusIdle || ses.Status == session.StatusArchived:
		if e.taskState(ctx, s.ReviewID) != config.TaskDone {
			return
		}
	default:
		return
	}
	e.stopFinished(s.ReviewID)
	e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) {
		st.State, st.EndedAt = config.StepQA, time.Now().UTC()
		st.Note = strings.TrimSpace(st.Note + " " + note)
	})
	e.KickRuns()
}

// nextIssue queues the next ready AFK issue, or ends the run.
func (e *Engine) nextIssue(ctx context.Context, r config.AFKRun, w config.Workflow, now time.Time) {
	end := func(state, reason string) {
		e.endRunNow(r.ID, state, reason)
	}
	if len(r.Steps) >= r.MaxIssues {
		end(config.RunFinished, fmt.Sprintf("It did its cap of %d issues.", r.MaxIssues))
		return
	}
	project, err := e.store.GetProject(ctx, r.ProjectID)
	if err != nil {
		end(config.RunStopped, "Its project is gone.")
		return
	}
	root := project.Root
	if r.Anchor != "" {
		anchor, err := e.store.GetSession(ctx, r.Anchor)
		if err != nil || anchor.Worktree == "" || !environment.IsDir(anchor.Worktree) {
			end(config.RunStopped, "Its worktree is gone.")
			return
		}
		root = anchor.Worktree
	}
	info, err := e.projectBacklog(ctx, project, root, w)
	if err != nil {
		end(config.RunStopped, "Its backlog cannot be read: "+err.Error())
		return
	}
	tried := func(id string) bool {
		return slices.ContainsFunc(r.Steps, func(s config.AFKStep) bool { return s.Issue == id })
	}
	next, ok := backlog.NextAFK(info.Issues, tried)
	if !ok {
		left := 0
		for _, it := range info.Issues {
			if it.State != backlog.Done {
				left++
			}
		}
		reason := "Every AFK issue is done."
		if left > 0 {
			reason = fmt.Sprintf("No AFK issue is ready; %d open issues are blocked or for a person.", left)
		}
		end(config.RunFinished, reason)
		return
	}
	budgets, _ := e.budgetStates(ctx, now)
	if note := budgetHeld(budgets, r.ProfileID); note != "" {
		end(config.RunStopped, "A budget is reached: "+note+".")
		return
	}
	env, err := e.localEnv(root, nil)
	if err != nil {
		end(config.RunStopped, err.Error())
		return
	}
	b, _ := env.FS().ReadFile(next.Path)
	commits := cmp.Or(recentCommits(ctx, env), "(none)")
	prompt := config.Render(w.AFK.Prompt, map[string]string{
		"input": "", "backlog": w.BacklogDir(), "issues": backlog.Summary(info.Issues), "commits": commits,
		"issue": next.Path, "issueText": strings.TrimSpace(backlog.Body(b)),
	})
	task := config.Task{
		Title: fmt.Sprintf("AFK %s: %s", next.ID, next.Title), Prompt: strings.TrimSpace(prompt), ProjectRoot: project.Root, ProfileID: r.ProfileID,
		Mode: r.Mode, EnvironmentID: r.EnvironmentID, Workflow: r.WorkflowID, Allow: slices.Clone(r.Allow), RunID: r.ID, Tags: []string{AFKTag},
		ModelOption: r.ModelOption, Model: r.Model, EffortOption: r.EffortOption, Effort: r.Effort,
	}
	if r.Anchor != "" {
		task.ShareWorktreeOf = r.Anchor
	} else {
		task.Worktree = true
	}
	// The step is saved as starting before the task is made: a save that fails
	// queues nothing, and a run cut off after it finds the task before it makes one.
	if err := e.changeRunErr(r.ID, func(r *config.AFKRun) error {
		r.Steps = append(r.Steps, config.AFKStep{Issue: next.ID, Path: next.Path, Title: next.Title, State: config.StepWorking, Starting: true, StartedAt: now.UTC()})
		return nil
	}); err != nil {
		end(config.RunStopped, "Its next issue could not be recorded, so it was not started: "+err.Error())
		return
	}
	q, err := e.AddTask(task)
	if err != nil {
		e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) {
			st.State, st.Starting, st.Note, st.EndedAt = config.StepFailed, false, "not queued: "+err.Error(), time.Now().UTC()
			endRun(r, config.RunStopped, "Its next issue could not be queued: "+err.Error())
		})
		return
	}
	added := q.Tasks[len(q.Tasks)-1]
	e.setStep(r.ID, func(r *config.AFKRun, st *config.AFKStep) { st.TaskID, st.Starting = added.ID, false })
}

// sessionRules are a run's commands as a session's allow rules.
func sessionRules(allow []string) []permission.Rule {
	var out []permission.Rule
	for _, c := range allow {
		out = append(out, permission.Rule{Tool: "shell", Command: c, Effect: permission.Allow, Scope: permission.ScopeSession})
	}
	return out
}

// afkAttention lists the issues waiting for the user's QA, for the attention view.
func (e *Engine) afkAttention(ctx context.Context, names map[string]string) []AttentionItem {
	all, err := e.runs()
	if err != nil {
		return nil
	}
	var out []AttentionItem
	for _, r := range all.Runs {
		for _, s := range r.Steps {
			if s.State != config.StepQA || s.SessionID == "" {
				continue
			}
			ses, err := e.store.GetSession(ctx, s.SessionID)
			if err != nil {
				continue
			}
			qa := &QAItem{RunID: r.ID, Issue: s.Issue, Title: s.Title, Path: s.Path, ReviewID: s.ReviewID, Branch: r.Branch, Note: s.Note}
			if s.ReviewID != "" {
				qa.Reply = e.lastReply(ctx, s.ReviewID)
			}
			out = append(out, AttentionItem{Session: ses, ProjectName: names[ses.ProjectID], QA: qa})
		}
	}
	return out
}

// QAItem is an issue an AFK run finished, waiting for the user's QA (WFL-006).
type QAItem struct {
	RunID    string `json:"runId"`
	Issue    string `json:"issue"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	ReviewID string `json:"reviewId"`
	Branch   string `json:"branch"`
	Note     string `json:"note"`
	// Reply is the start of the review's answer.
	Reply string `json:"reply"`
}
