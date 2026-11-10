package engine

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Model sets (WFL-013): the user's named choices of model and effort for a workflow's
// planning stages, issues and reviews, kept in config.yaml with the set each workflow
// uses. A row whose provider is gone or has no key keeps its value and is reported,
// so restoring the key mends it; a model the provider no longer lists still counts.

// ModelSetInfo is a set with what stands in its way and who uses it.
type ModelSetInfo struct {
	config.ModelSet
	// Unavailable says, by row (planning, issues, reviews), why its model cannot be had.
	Unavailable map[string]string `json:"unavailable"`
	// Workflows are the IDs of the workflows that use it.
	Workflows []string `json:"workflows"`
}

// ListModelSets lists the sets in the order they were made.
func (e *Engine) ListModelSets(ctx context.Context) []ModelSetInfo {
	c := e.Config()
	out := []ModelSetInfo{}
	for _, s := range c.ModelSets {
		info := ModelSetInfo{ModelSet: s, Unavailable: map[string]string{}, Workflows: []string{}}
		for name, row := range s.Rows() {
			if why := e.modelUnavailable(row); why != "" {
				info.Unavailable[name] = why
			}
		}
		for _, wf := range slices.Sorted(maps.Keys(c.WorkflowModelSets)) {
			if c.WorkflowModelSets[wf] == s.ID {
				info.Workflows = append(info.Workflows, wf)
			}
		}
		out = append(out, info)
	}
	return out
}

// SaveModelSet adds a set, with an ID made from its name, or replaces the one with its
// ID.
func (e *Engine) SaveModelSet(ctx context.Context, set config.ModelSet) (config.Config, error) {
	set.Name = strings.TrimSpace(set.Name)
	c := e.Config()
	c.ModelSets = slices.Clone(c.ModelSets)
	if i := slices.IndexFunc(c.ModelSets, func(s config.ModelSet) bool { return s.ID == set.ID }); set.ID != "" && i >= 0 {
		c.ModelSets[i] = set
	} else {
		base := config.ModelSetID(set.Name)
		set.ID = base
		for n := 2; slices.ContainsFunc(c.ModelSets, func(s config.ModelSet) bool { return s.ID == set.ID }); n++ {
			set.ID = fmt.Sprintf("%s-%d", base, n)
		}
		c.ModelSets = append(c.ModelSets, set)
	}
	return e.SaveSettings(c)
}

// DeleteModelSet removes a set, and from the workflows that used it.
func (e *Engine) DeleteModelSet(ctx context.Context, id string) (config.Config, error) {
	c := e.Config()
	c.ModelSets = slices.DeleteFunc(slices.Clone(c.ModelSets), func(s config.ModelSet) bool { return s.ID == id })
	c.WorkflowModelSets = maps.Clone(c.WorkflowModelSets)
	maps.DeleteFunc(c.WorkflowModelSets, func(_, set string) bool { return set == id })
	return e.SaveSettings(c)
}

// SetWorkflowModelSet has a workflow use a set; empty, none. The workflow's file and
// its approval are not touched.
func (e *Engine) SetWorkflowModelSet(ctx context.Context, workflowID, setID string) (config.Config, error) {
	if _, ok := e.findWorkflow(workflowID); !ok {
		return config.Config{}, fmt.Errorf("there is no workflow %q", workflowID)
	}
	c := e.Config()
	c.WorkflowModelSets = maps.Clone(c.WorkflowModelSets)
	if c.WorkflowModelSets == nil {
		c.WorkflowModelSets = map[string]string{}
	}
	if setID == "" {
		delete(c.WorkflowModelSets, workflowID)
	} else {
		if !slices.ContainsFunc(c.ModelSets, func(s config.ModelSet) bool { return s.ID == setID }) {
			return config.Config{}, fmt.Errorf("there is no model set %q", setID)
		}
		c.WorkflowModelSets[workflowID] = setID
	}
	return e.SaveSettings(c)
}

// workflowModelSet is the set a workflow uses, if any.
func (e *Engine) workflowModelSet(workflowID string) (config.ModelSet, bool) {
	c := e.Config()
	id := c.WorkflowModelSets[workflowID]
	i := slices.IndexFunc(c.ModelSets, func(s config.ModelSet) bool { return s.ID == id })
	if id == "" || i < 0 {
		return config.ModelSet{}, false
	}
	return c.ModelSets[i], true
}

// startingPlanning is the planning row a new session starts on: its workflow was
// chosen for it, not taken by default, and the row names a built-in provider's model
// that can be had (WFL-014).
func (e *Engine) startingPlanning(req CreateSessionRequest) (config.ModelChoice, bool) {
	if req.Workflow == "" || req.Workflow == config.WorkflowOff {
		return config.ModelChoice{}, false
	}
	set, ok := e.workflowModelSet(req.Workflow)
	if !ok || !set.Planning.Fixed() || set.Planning.Provider == "" || e.modelUnavailable(set.Planning) != "" {
		return config.ModelChoice{}, false
	}
	return set.Planning, true
}

// AllModels are the models a set's rows can name: every model of every provider that
// can be called unasked, and any named by hand.
func (e *Engine) AllModels(ctx context.Context) ProfileModels {
	out := ProfileModels{Choices: []harness.OptionChoice{}}
	for _, o := range e.modelOptions(ctx, session.Session{HarnessID: builtin.ID}, true) {
		if o.ID == optionModelID {
			out.OptionID, out.Custom = o.ID, o.Custom
			for _, c := range o.Choices {
				// The empty session's own entry names no provider.
				if !strings.HasPrefix(c.Value, modelSeparator) {
					out.Choices = append(out.Choices, c)
				}
			}
		}
	}
	return out
}

// EffortsOf are the efforts the built-in agent offers on a model value.
func (e *Engine) EffortsOf(ctx context.Context, model string) ProfileEfforts {
	out := ProfileEfforts{Choices: []harness.OptionChoice{}, Default: "high"}
	providerID, name, ok := strings.Cut(model, modelSeparator)
	if !ok {
		return out
	}
	cctx, cancel := context.WithTimeout(ctx, modelListWait)
	defer cancel()
	if caps, err := e.ModelCapabilities(cctx, providerID, name, false); err == nil && caps.Effort {
		out.OptionID = optionEffortID
		for _, v := range efforts {
			out.Choices = append(out.Choices, harness.OptionChoice{Value: v, Name: v})
		}
	}
	return out
}

// modelUnavailable says why a row's model cannot be had: its provider is gone, or has
// no key. A row that is not fixed, or names an external agent's model, has nothing
// in its way that the app can know of.
func (e *Engine) modelUnavailable(row config.ModelChoice) string {
	if !row.Fixed() || row.Provider == "" {
		return ""
	}
	i := slices.IndexFunc(e.ListProviders(), func(p ProviderInfo) bool { return p.ID == row.Provider })
	if i < 0 {
		return fmt.Sprintf("%s on %s: the provider is gone", row.Model, row.Provider)
	}
	p := e.ListProviders()[i]
	if !canCallUnasked(p) {
		return fmt.Sprintf("%s on %s: the provider has no key", row.Model, p.Name)
	}
	e.mu.Lock()
	j := slices.IndexFunc(e.providers.configs, func(c config.Provider) bool { return c.ID == row.Provider })
	var cfg config.Provider
	if j >= 0 {
		cfg = e.providers.configs[j]
	}
	e.mu.Unlock()
	if cfg.APIKey != "" {
		if _, err := e.providerKey(cfg); err != nil {
			return fmt.Sprintf("%s on %s: the provider has no key", row.Model, p.Name)
		}
	}
	return ""
}
