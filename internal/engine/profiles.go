package engine

import (
	"context"
	"fmt"
	"slices"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
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

// profile returns the profile with id; "" is the default.
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
	if _, err := e.profile(profileID); err != nil {
		return err
	}
	p, err := e.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	p.DefaultProfileID = profileID
	return e.store.UpdateProject(ctx, p)
}

// profileRules returns the rules of sid's profile as it is now, so edits to
// the file apply at the next permission check.
func (e *Engine) profileRules(sid string) []permission.Rule {
	ses, err := e.store.GetSession(context.Background(), sid)
	if err != nil {
		return nil
	}
	p, err := e.profile(ses.ProfileID)
	if err != nil {
		// A session whose profile went away keeps no allow rules but must
		// not lose its denies; it cannot start anyway (runtimeFor).
		return nil
	}
	return p.Rules
}
