package engine

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// The models an AFK run's sessions can take (WFL-010), so its issues can go to a
// smaller model than the profile's. A model is chosen through the session's own model
// option, as the session's model list does, so the same works for every agent
// (HRN-001): the built-in agent's are its provider's models; an external agent says
// what it offers only once it runs, so its are what it last said in one of the user's
// sessions. Nothing is started to find out.

// howManyToLook bounds the sessions read for an external agent's models.
const howManyToLook = 20

// ProfileModels are the models a profile's sessions can be switched to.
type ProfileModels struct {
	// OptionID is the session option that sets the model.
	OptionID string                 `json:"optionId"`
	Choices  []harness.OptionChoice `json:"choices"`
	// Default names what the profile's sessions start on, when known.
	Default string `json:"default"`
	// Note says where the choices come from, or why there are none.
	Note string `json:"note"`
	// Custom is set when a model the choices lack can be named (WFL-011).
	Custom bool `json:"custom"`
}

// ProfileModels lists the models a profile's sessions in the project at root can be
// switched to.
func (e *Engine) ProfileModels(ctx context.Context, root, profileID string) (ProfileModels, error) {
	project, prof, err := e.projectProfile(ctx, root, profileID)
	if err != nil {
		return ProfileModels{}, err
	}
	out := ProfileModels{Choices: []harness.OptionChoice{}}
	if prof.Harness == "" || prof.Harness == builtin.ID {
		providerID := cmp.Or(prof.Provider, e.Config().Defaults.ProviderID)
		if providerID == "" {
			out.Note = "The profile names no provider, so its model is chosen with the session."
			return out, nil
		}
		p, err := e.provider(providerID)
		if err != nil {
			return out, err
		}
		model := cmp.Or(prof.Model, p.cfg.DefaultModel)
		ses := session.Session{ProviderID: providerID, Model: model, ProfileID: prof.ID, ProjectID: project.ID, HarnessID: builtin.ID}
		for _, o := range e.modelOptions(ctx, ses, true) {
			if o.Category == harness.OptionModel {
				out.OptionID, out.Choices, out.Custom = o.ID, slices.Clone(o.Choices), o.Custom
			}
		}
		out.Default = model
		return out, nil
	}
	name := prof.Harness
	if p, err := e.preset(prof.Harness); err == nil {
		name = p.Name
	}
	o, ok := e.reportedOption(ctx, prof.Harness, harness.OptionModel)
	if !ok {
		out.Note = fmt.Sprintf("%s says which models it offers only once it runs. Open one of its sessions and choose its model there once; its models are then listed here.", name)
		return out, nil
	}
	out.OptionID, out.Choices, out.Custom = o.ID, slices.Clone(o.Choices), o.Custom
	out.Note = fmt.Sprintf("As %s listed them in your latest session with it.", name)
	return out, nil
}

// reportedOption is the option of a category an external agent reported most recently
// in one of the user's sessions.
func (e *Engine) reportedOption(ctx context.Context, harnessID string, category string) (harness.Option, bool) {
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		return harness.Option{}, false
	}
	looked := 0
	for _, ses := range list {
		if ses.HarnessID != harnessID {
			continue
		}
		if looked++; looked > howManyToLook {
			break
		}
		options, err := e.reportedOptions(ctx, ses.ID)
		if err != nil {
			continue
		}
		for _, o := range options {
			if o.Category == category && len(o.Choices) > 0 {
				return o, true
			}
		}
	}
	return harness.Option{}, false
}

// projectProfile is the project at root, or the one whose worktree it is, and the
// profile its sessions would have.
func (e *Engine) projectProfile(ctx context.Context, root, profileID string) (session.Project, config.Profile, error) {
	project, err := e.store.ProjectByRoot(ctx, environment.Canonical(root))
	if err != nil {
		if project, err = e.projectOfWorktree(ctx, root); err != nil {
			return session.Project{}, config.Profile{}, err
		}
	}
	prof, err := e.profileFor(project, profileID)
	return project, prof, err
}

// ProfileEfforts are the efforts a profile's sessions on a model can be switched to
// (WFL-012); none when the model takes no effort.
type ProfileEfforts struct {
	OptionID string                 `json:"optionId"`
	Choices  []harness.OptionChoice `json:"choices"`
	// Default is the profile's effort, which "the profile's" keeps.
	Default string `json:"default"`
	Note    string `json:"note"`
	// External is set for an external agent, whose effort is whatever it reports.
	External bool `json:"external"`
}

// ModelEfforts lists the efforts a profile's sessions in the project at root can take
// on model, a value of ProfileModels' choices, or the profile's own model when empty.
func (e *Engine) ModelEfforts(ctx context.Context, root, profileID, model string) (ProfileEfforts, error) {
	_, prof, err := e.projectProfile(ctx, root, profileID)
	if err != nil {
		return ProfileEfforts{}, err
	}
	out := ProfileEfforts{Choices: []harness.OptionChoice{}}
	if prof.Harness == "" || prof.Harness == builtin.ID {
		providerID, name := cmp.Or(prof.Provider, e.Config().Defaults.ProviderID), prof.Model
		if p, m, ok := strings.Cut(model, modelSeparator); ok {
			providerID, name = p, m
		}
		if name == "" {
			if p, err := e.provider(providerID); err == nil {
				name = p.cfg.DefaultModel
			}
		}
		cctx, cancel := context.WithTimeout(ctx, modelListWait)
		defer cancel()
		caps, err := e.ModelCapabilities(cctx, providerID, name, false)
		if err != nil || !caps.Effort {
			out.Note = "This model has no effort setting."
			return out, nil
		}
		out.OptionID, out.Default = optionEffortID, cmp.Or(prof.Effort, "high")
		for _, v := range efforts {
			out.Choices = append(out.Choices, harness.OptionChoice{Value: v, Name: v})
		}
		return out, nil
	}
	out.External = true
	o, ok := e.reportedOption(ctx, prof.Harness, harness.OptionEffort)
	if !ok {
		out.Note = "This agent has reported no effort setting."
		return out, nil
	}
	out.OptionID, out.Choices, out.Default = o.ID, slices.Clone(o.Choices), o.Current
	return out, nil
}

// runEffort is the effort option and level a run stores for a profile on a model: none
// when no level was chosen or the model takes no effort, an error for a level the
// model does not offer.
func (e *Engine) runEffort(ctx context.Context, root, profileID, model, level string) (string, string, error) {
	if level == "" {
		return "", "", nil
	}
	eff, err := e.ModelEfforts(ctx, root, profileID, model)
	if err != nil {
		return "", "", err
	}
	if len(eff.Choices) == 0 {
		return "", "", nil
	}
	if !slices.ContainsFunc(eff.Choices, func(c harness.OptionChoice) bool { return c.Value == level }) {
		return "", "", fmt.Errorf("%s is not an effort that model offers", level)
	}
	return eff.OptionID, level, nil
}

// offers reports whether a run can take value: one of the choices, or, where a model
// can be named, one on a provider the choices come from (WFL-011).
func (m ProfileModels) offers(value string) bool {
	if slices.ContainsFunc(m.Choices, func(c harness.OptionChoice) bool { return c.Value == value }) {
		return true
	}
	providerID, model, ok := strings.Cut(value, modelSeparator)
	if !m.Custom || !ok || strings.TrimSpace(model) == "" || strings.ContainsAny(model, "\r\n") {
		return false
	}
	return slices.ContainsFunc(m.Choices, func(c harness.OptionChoice) bool {
		return strings.HasPrefix(c.Value, providerID+modelSeparator)
	})
}

// label names a value for a person: the model, with its provider's name when the
// choices give one.
func (m ProfileModels) label(value string) string {
	name, group := modelName(value), ""
	providerID, _, _ := strings.Cut(value, modelSeparator)
	for _, c := range m.Choices {
		if c.Value == value && c.Name != "" {
			name = c.Name
		}
		if group == "" && strings.HasPrefix(c.Value, providerID+modelSeparator) {
			group = c.Group
		}
	}
	if group == "" || !strings.Contains(value, modelSeparator) {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, group)
}

// modelName is a model option's value as a person reads it: the built-in agent's
// value names the provider too.
func modelName(value string) string {
	if _, model, ok := strings.Cut(value, modelSeparator); ok {
		return model
	}
	return value
}
