package config

import "testing"

// WFL-013: a model set needs a name, an ID of its own and real efforts; a workflow can
// only name a set that exists.
func TestModelSets(t *testing.T) {
	if got := ModelSetID("Opus plans, Qwen works!"); got != "opus-plans-qwen-works" {
		t.Errorf("id = %q", got)
	}
	sets, issues := validateModelSets([]ModelSet{
		{ID: "split", Name: "Split", Planning: ModelChoice{Provider: "anthropic", Model: "claude-opus-5-5", Effort: "high"}, Issues: ModelChoice{Provider: "openrouter", Model: "qwen-3.8-flash", Effort: "low"}},
		{ID: "blank", Name: "  "},
		{ID: "split", Name: "Split again"},
		{ID: "loud", Name: "Loud", Reviews: ModelChoice{Provider: "a", Model: "m", Effort: "enormous"}},
		{ID: "half", Name: "Half", Issues: ModelChoice{Effort: "low"}},
	})
	if len(sets) != 1 || sets[0].ID != "split" || len(issues) != 4 {
		t.Errorf("kept %+v, issues %+v", sets, issues)
	}
	c := Default()
	c.ModelSets = sets
	c.WorkflowModelSets = map[string]string{BuiltinWorkflowID: "split", "other": "gone"}
	if issues := c.Validate(); len(issues) != 1 || c.WorkflowModelSets["other"] != "" || c.WorkflowModelSets[BuiltinWorkflowID] != "split" {
		t.Errorf("issues %+v, workflows %v", issues, c.WorkflowModelSets)
	}
	if (ModelChoice{Provider: "p", Model: "m"}).Value() != "p\nm" || (ModelChoice{Model: "opus"}).Value() != "opus" || (ModelChoice{}).Fixed() {
		t.Error("a choice's value")
	}
}
