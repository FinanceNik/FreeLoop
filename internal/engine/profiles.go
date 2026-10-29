package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Profiles (HRN-003) live in profiles/<id>.yaml. The built-in default
// applies when no default.yaml exists, so every session has a profile.

func (e *Engine) loadProfiles() {
	list, issues, err := config.LoadProfiles(e.dataDir, builtin.ToolNames())
	if err != nil {
		issues = append(issues, config.Issue{File: config.ProfilesDir, Message: err.Error()})
	}
	if !slices.ContainsFunc(list, func(p config.Profile) bool { return p.ID == config.DefaultProfileID }) {
		list = append([]config.Profile{config.BuiltinDefaultProfile()}, list...)
	}
	list = append(list, e.presetProfiles(list)...)
	e.mu.Lock()
	e.profiles = list
	e.issues[issueProfiles] = issues
	e.mu.Unlock()
}

// ListProfiles returns every profile, the default first.
func (e *Engine) ListProfiles() []config.Profile {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.profiles)
}

// profileFor returns the profile with id for a session in project: a
// project profile (from a trusted project's .freeloop/profiles/) when id
// has the project prefix, else an app profile; "" is the default.
func (e *Engine) profileFor(project session.Project, id string) (config.Profile, error) {
	if strings.HasPrefix(id, config.ProjectProfilePrefix) {
		for _, p := range e.projectConfig(project).Profiles {
			if p.ID == id {
				return p, nil
			}
		}
		return config.Profile{}, fmt.Errorf("no profile %q in %s; the project may be untrusted, or the file deleted or invalid", id, project.Root)
	}
	return e.profile(id)
}

// profile returns the app profile with id; "" is the default.
func (e *Engine) profile(id string) (config.Profile, error) {
	if id == "" {
		id = config.DefaultProfileID
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return config.Profile{}, fmt.Errorf("no profile %q; it may have been deleted or have errors (see the problems listed in the app)", id)
}

// SaveProfile writes a profile and reloads the list.
func (e *Engine) SaveProfile(p config.Profile) (config.Profile, error) {
	for i, t := range p.Tools {
		if !slices.Contains(builtin.ToolNames(), t) {
			return config.Profile{}, fmt.Errorf("tools[%d]: unknown tool %q", i, t)
		}
	}
	if err := config.SaveProfile(e.dataDir, p); err != nil {
		return config.Profile{}, err
	}
	e.loadProfiles()
	return e.profile(p.ID)
}

// DeleteProfile removes a profile's file. Deleting default.yaml brings back
// the built-in default. Sessions of a deleted profile fail to start until
// it exists again.
func (e *Engine) DeleteProfile(id string) error {
	if err := config.DeleteProfile(e.dataDir, id); err != nil {
		return err
	}
	e.loadProfiles()
	return nil
}

// SetProjectProfile changes the profile new sessions of a project start
// with (HRN-004).
func (e *Engine) SetProjectProfile(ctx context.Context, projectID, profileID string) error {
	p, err := e.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if _, err := e.profileFor(p, profileID); err != nil {
		return err
	}
	p.DefaultProfileID = profileID
	return e.store.UpdateProject(ctx, p)
}

// commandsOf returns a profile's slash commands as the harness takes them
// and as the session advertises them.
func commandsOf(p config.Profile) ([]harness.Command, event.AvailableCommandsData) {
	cmds := []harness.Command{}
	data := event.AvailableCommandsData{AvailableCommands: []event.AvailableCommand{}}
	for _, c := range p.Commands {
		cmds = append(cmds, harness.Command{Name: c.Name, Description: c.Description, Prompt: c.Prompt})
		data.AvailableCommands = append(data.AvailableCommands, event.AvailableCommand{Name: c.Name, Description: c.Description})
	}
	return cmds, data
}
