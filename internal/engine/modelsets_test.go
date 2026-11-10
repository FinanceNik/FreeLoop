package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/harness"
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
