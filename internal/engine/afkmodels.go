package engine

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

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
}

// ProfileModels lists the models a profile's sessions in the project at root can be
// switched to.
func (e *Engine) ProfileModels(ctx context.Context, root, profileID string) (ProfileModels, error) {
	project, err := e.store.ProjectByRoot(ctx, environment.Canonical(root))
	if err != nil {
		if project, err = e.projectOfWorktree(ctx, root); err != nil {
			return ProfileModels{}, err
		}
	}
	prof, err := e.profileFor(project, profileID)
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
		for _, o := range e.builtinOptions(ctx, ses) {
			if o.Category == harness.OptionModel {
				out.OptionID, out.Choices = o.ID, slices.Clone(o.Choices)
			}
		}
		out.Default = model
		return out, nil
	}
	name := prof.Harness
	if p, err := e.preset(prof.Harness); err == nil {
		name = p.Name
	}
	o, ok := e.reportedModels(ctx, prof.Harness)
	if !ok {
		out.Note = fmt.Sprintf("%s says which models it offers only once it runs. Open one of its sessions and choose its model there once; its models are then listed here.", name)
		return out, nil
	}
	out.OptionID, out.Choices = o.ID, slices.Clone(o.Choices)
	out.Note = fmt.Sprintf("As %s listed them in your latest session with it.", name)
	return out, nil
}

// reportedModels is the model option an external agent reported most recently in
// one of the user's sessions.
func (e *Engine) reportedModels(ctx context.Context, harnessID string) (harness.Option, bool) {
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
		events, err := e.store.Events(ctx, ses.ID, 0)
		if err != nil {
			continue
		}
		for _, o := range optionsInEvents(events) {
			if o.Category == harness.OptionModel && len(o.Choices) > 0 {
				return o, true
			}
		}
	}
	return harness.Option{}, false
}

// modelName is a model option's value as a person reads it: the built-in agent's
// value names the provider too.
func modelName(value string) string {
	if _, model, ok := strings.Cut(value, modelSeparator); ok {
		return model
	}
	return value
}
