package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/acp/acptest"
)

// WFL-013: model sets are saved with an ID from their name, a row whose provider is
// gone or has no key is kept and reported unavailable, a workflow picks a set without
// its file or approval changing, and deleting a set clears it from its workflows.
func TestModelSets(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	twoProviders(t, e)
	_ = root
	// A provider whose key is configured but not in the keychain.
	b, _ := os.ReadFile(filepath.Join(e.dataDir, config.ProvidersFileName))
	os.WriteFile(filepath.Join(e.dataDir, config.ProvidersFileName), append(b, []byte("  - {id: keyed, name: Keyed, type: anthropic, baseUrl: \"https://api.example.com\", apiKey: \"keychain://freeloop/providers/keyed\", defaultModel: k}\n")...), 0o600)
	twoProvidersBuilt(t, e)

	set := config.ModelSet{Name: "Big plans, small works",
		Planning: config.ModelChoice{Provider: "big", Model: "b"},
		Issues:   config.ModelChoice{Provider: "small", Model: "tiny", Effort: "low"},
		Reviews:  config.ModelChoice{Provider: "keyed", Model: "k"}}
	if _, err := e.SaveModelSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	// A second with the same name gets an ID of its own.
	if _, err := e.SaveModelSet(ctx, config.ModelSet{Name: "Big plans, small works", Planning: config.ModelChoice{Provider: "gone", Model: "x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SaveModelSet(ctx, config.ModelSet{Name: "Bad", Issues: config.ModelChoice{Provider: "small", Model: "tiny", Effort: "enormous"}}); err == nil {
		t.Error("a set with an effort that is not one was saved")
	}
	sets := e.ListModelSets(ctx)
	if len(sets) != 2 || sets[0].ID != "big-plans-small-works" || sets[1].ID != "big-plans-small-works-2" {
		t.Fatalf("sets = %+v", sets)
	}
	if sets[0].Unavailable["reviews"] == "" || sets[0].Unavailable["issues"] != "" || sets[0].Unavailable["planning"] != "" {
		t.Errorf("the first set's rows: %v", sets[0].Unavailable)
	}
	if !strings.Contains(sets[1].Unavailable["planning"], "gone") || sets[1].Planning.Model != "x" {
		t.Errorf("a removed provider: %+v", sets[1])
	}
	// Editing keeps the ID.
	set.ID, set.Name = sets[0].ID, "Renamed"
	if _, err := e.SaveModelSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	if got := e.ListModelSets(ctx); got[0].ID != set.ID || got[0].Name != "Renamed" {
		t.Errorf("after editing: %+v", got[0])
	}

	// A file workflow, approved; choosing a set leaves the file and the approval alone.
	dir := filepath.Join(e.dataDir, config.WorkflowsDir)
	os.MkdirAll(dir, 0o700)
	file := filepath.Join(dir, "mine.yaml")
	body := []byte("name: Mine\nstages:\n  - {command: go, title: Go, prompt: \"Go: {{input}}\"}\n")
	os.WriteFile(file, body, 0o600)
	e.loadWorkflows()
	mine := func() WorkflowStatus {
		i := slices.IndexFunc(e.ListWorkflows(ctx), func(w WorkflowStatus) bool { return w.ID == "mine" })
		return e.ListWorkflows(ctx)[i]
	}
	if _, err := e.ApproveWorkflow(ctx, "mine", mine().Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetWorkflowModelSet(ctx, "mine", set.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetWorkflowModelSet(ctx, "mine", "nope"); err == nil {
		t.Error("a set that does not exist was chosen")
	}
	if got, _ := os.ReadFile(file); string(got) != string(body) || mine().NeedsApproval || mine().ModelSet != set.ID {
		t.Errorf("after choosing a set: file %q, needs approval %v, set %q", got, mine().NeedsApproval, mine().ModelSet)
	}
	if got := e.ListModelSets(ctx); !slices.Contains(got[0].Workflows, "mine") {
		t.Errorf("the set's workflows: %v", got[0].Workflows)
	}
	if _, err := e.DeleteModelSet(ctx, set.ID); err != nil {
		t.Fatal(err)
	}
	if mine().ModelSet != "" || len(e.ListModelSets(ctx)) != 1 {
		t.Errorf("after deleting: %q, %+v", mine().ModelSet, e.ListModelSets(ctx))
	}

	// The set editor's models are every callable provider's, and its efforts the model's.
	all := e.AllModels(ctx)
	if !slices.ContainsFunc(all.Choices, func(c harness.OptionChoice) bool { return c.Value == "small"+modelSeparator+"scripted" }) || !all.Custom {
		t.Errorf("all models: %+v", all)
	}
	if eff := e.EffortsOf(ctx, "small"+modelSeparator+"tiny"); len(eff.Choices) != 5 {
		t.Errorf("small's efforts: %+v", eff)
	}
	if eff := e.EffortsOf(ctx, "big"+modelSeparator+"b"); len(eff.Choices) != 0 {
		t.Errorf("big's efforts: %+v", eff)
	}
}

// WFL-014: a planning stage switches its session to the set's Planning model and
// effort before its prompt, with a notice, and not again once there; a model that
// cannot be had stops the stage, which can then run on the session's own model.
func TestPlanningStageSwitchesToTheSetsModel(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	twoProviders(t, e)
	b, _ := os.ReadFile(filepath.Join(e.dataDir, config.ProvidersFileName))
	os.WriteFile(filepath.Join(e.dataDir, config.ProvidersFileName), append(b, []byte("  - {id: keyed, name: Keyed, type: anthropic, baseUrl: \"https://api.example.com\", apiKey: \"keychain://freeloop/providers/keyed\", defaultModel: k}\n")...), 0o600)
	twoProvidersBuilt(t, e)
	if _, err := e.InstallWorkflowSkills(ctx, config.BuiltinWorkflowID); err != nil {
		t.Fatal(err)
	}
	set := config.ModelSet{Name: "Split", Planning: config.ModelChoice{Provider: "small", Model: "tiny", Effort: "low"}}
	if _, err := e.SaveModelSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetWorkflowModelSet(ctx, config.BuiltinWorkflowID, "split"); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "big"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetWorkflow(ctx, ses.ID, config.BuiltinWorkflowID); err != nil {
		t.Fatal(err)
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	// The scripted agent plays the stages, writing their files, which asks.
	stop := make(chan struct{})
	defer close(stop)
	approveAll(e, stop)
	count := func(typ event.Type, text string) int {
		evs, _ := e.store.Events(ctx, ses.ID, 0)
		n := 0
		for _, ev := range evs {
			if ev.Type == typ && strings.Contains(string(ev.Data), text) {
				n++
			}
		}
		return n
	}
	res, err := e.RunStage(ctx, ses.ID, "grill", "add dark mode")
	if err != nil || res.Blocked != "" {
		t.Fatalf("grill: %+v, %v", res, err)
	}
	waitIdle(t, ch, ses.ID)
	got, _ := e.store.GetSession(ctx, ses.ID)
	if got.ProviderID != "small" || got.Model != "tiny" || got.Effort != "low" {
		t.Errorf("after grill: %s/%s at %q", got.ProviderID, got.Model, got.Effort)
	}
	evs, _ := e.store.Events(ctx, ses.ID, 0)
	notice := slices.IndexFunc(evs, func(ev event.Event) bool {
		return ev.Type == event.Notice && strings.Contains(string(ev.Data), "Switched to tiny · low for this stage (model set Split)")
	})
	prompt := slices.IndexFunc(evs, func(ev event.Event) bool { return ev.Type == event.UserMessageChunk })
	if notice < 0 || notice > prompt {
		t.Errorf("the notice is at %d, the prompt at %d", notice, prompt)
	}
	if _, err := e.RunStage(ctx, ses.ID, "prd", ""); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	if n := count(event.ModelChanged, ""); n != 1 {
		t.Errorf("%d model changes after a second stage on the same model", n)
	}

	// The Planning provider's key is not in the keychain: the stage stops and says why.
	set.ID, set.Planning = "split", config.ModelChoice{Provider: "keyed", Model: "k"}
	if _, err := e.SaveModelSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	sent := count(event.UserMessageChunk, "")
	res, err = e.RunStage(ctx, ses.ID, "slice", "")
	if err != nil || !strings.Contains(res.Blocked, "k on Keyed") {
		t.Fatalf("a model that cannot be had: %+v, %v", res, err)
	}
	if count(event.UserMessageChunk, "") != sent {
		t.Error("the stage sent its prompt anyway")
	}
	if _, err := e.RunStageOnCurrent(ctx, ses.ID, "slice", ""); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	if got, _ := e.store.GetSession(ctx, ses.ID); got.ProviderID != "small" || count(event.UserMessageChunk, "") != sent+1 {
		t.Errorf("on the current model: %s, %d messages", got.ProviderID, count(event.UserMessageChunk, ""))
	}
}

// WFL-014: a session created with its workflow chosen starts on the workflow's
// Planning model and effort; one that only takes the default workflow, or whose
// profile runs an external agent, starts on its profile's.
func TestWorkflowSessionStartsOnThePlanningModel(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	twoProviders(t, e)
	if _, err := e.SaveModelSet(ctx, config.ModelSet{Name: "Split", Planning: config.ModelChoice{Provider: "small", Model: "tiny", Effort: "low"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetWorkflowModelSet(ctx, config.BuiltinWorkflowID, "split"); err != nil {
		t.Fatal(err)
	}
	// The new-session form learns it from the list of workflows.
	if i := slices.IndexFunc(e.Workflows(ctx), func(w WorkflowInfo) bool { return w.ID == config.BuiltinWorkflowID }); i < 0 || e.Workflows(ctx)[i].Planning == nil || e.Workflows(ctx)[i].Planning.Model != "tiny" || e.Workflows(ctx)[i].ModelSet != "Split" {
		t.Errorf("workflows: %+v", e.Workflows(ctx))
	}
	chosen, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "big", Workflow: config.BuiltinWorkflowID})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := e.store.GetSession(ctx, chosen.ID); got.ProviderID != "small" || got.Model != "tiny" || got.Effort != "low" {
		t.Errorf("chosen: %s/%s at %q", got.ProviderID, got.Model, got.Effort)
	}
	// A model asked for by name wins.
	named, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "big", Workflow: config.BuiltinWorkflowID, ProviderID: "big", Model: "b"})
	if named.ProviderID != "big" || named.Model != "b" {
		t.Errorf("named: %s/%s", named.ProviderID, named.Model)
	}
	// The planning model named, as the new-session form fills it in, takes its effort.
	filled, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "big", Workflow: config.BuiltinWorkflowID, ProviderID: "small", Model: "tiny"})
	if filled.Effort != "low" {
		t.Errorf("filled in: effort %q", filled.Effort)
	}
	if _, err := e.SetDefaultWorkflow(ctx, config.BuiltinWorkflowID); err != nil {
		t.Fatal(err)
	}
	byDefault, _ := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "big"})
	if byDefault.Workflow != config.BuiltinWorkflowID || byDefault.ProviderID != "big" || byDefault.Model != "b" {
		t.Errorf("by default: %q on %s/%s", byDefault.Workflow, byDefault.ProviderID, byDefault.Model)
	}

	agentProfile(t, e)
	agent, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: root, ProfileID: "agent", Workflow: config.BuiltinWorkflowID})
	if err != nil {
		t.Fatal(err)
	}
	if agent.HarnessID != "ref" || agent.ProviderID != "" || agent.Model != "ref-model" {
		t.Errorf("an external agent: %s, %s/%s", agent.HarnessID, agent.ProviderID, agent.Model)
	}
}

// agentProfile gives the engine a profile "agent" on the reference ACP agent.
func agentProfile(t *testing.T, e *Engine) {
	t.Helper()
	exe, _ := os.Executable()
	os.MkdirAll(filepath.Join(e.dataDir, config.PresetsDir), 0o700)
	os.WriteFile(filepath.Join(e.dataDir, config.PresetsDir, "ref.yaml"), []byte("name: Reference agent\ncommand: "+exe+"\nenv:\n  "+acptest.EnvVar+": \"1\"\n"), 0o600)
	e.loadPresets()
	approvePending(t, e)
	if err := e.SaveProfileYAML("agent", "name: Agent\nharness: ref\nmodel: ref-model\n"); err != nil {
		t.Fatal(err)
	}
}

// WFL-014: the AFK dialog starts from the workflow's model set; a run keeps the models
// it started with when the set is edited, says them, and is refused a model its
// profile cannot have.
func TestAFKRunStartsFromTheSet(t *testing.T) {
	ctx := context.Background()
	e, root := afkEngine(t)
	twoProviders(t, e)
	set := config.ModelSet{Name: "Split", Issues: config.ModelChoice{Provider: "small", Model: "tiny", Effort: "low"}, Reviews: config.ModelChoice{Provider: "gone", Model: "x"}}
	if _, err := e.SaveModelSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetWorkflowModelSet(ctx, config.BuiltinWorkflowID, "split"); err != nil {
		t.Fatal(err)
	}
	plan, err := e.PrepareAFK(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.ModelSet == nil || plan.ModelSet.Name != "Split" || plan.ModelSet.Issues.Model != "tiny" || plan.ModelSetUnavailable["reviews"] == "" || plan.ModelSetUnavailable["issues"] != "" {
		t.Fatalf("plan's set: %+v, unavailable %v", plan.ModelSet, plan.ModelSetUnavailable)
	}
	agentProfile(t, e)
	if _, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, ProfileID: "agent", Model: set.Issues.Value()}); err == nil {
		t.Error("an external agent's profile took a built-in provider's model")
	}
	run, err := e.StartAFK(ctx, AFKStart{ProjectRoot: root, ProfileID: "big", ReviewProfileID: "big", Allow: []string{"git add *", "git commit *"}, Model: set.Issues.Value(), Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	if run.ModelLabel != "tiny (Small)" {
		t.Errorf("label %q", run.ModelLabel)
	}
	set.ID, set.Issues = "split", config.ModelChoice{Provider: "big", Model: "b"}
	if _, err := e.SaveModelSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	run = waitRun(t, e, run.ID)
	if run.State != config.RunFinished || run.Model != "small"+modelSeparator+"tiny" || run.Effort != "low" {
		t.Fatalf("run = %+v", run)
	}
	for _, s := range run.Steps {
		if ses, _ := e.store.GetSession(ctx, s.SessionID); ses.Model != "tiny" || ses.Effort != "low" {
			t.Errorf("%s ran on %s at %q", s.Issue, ses.Model, ses.Effort)
		}
	}
}
