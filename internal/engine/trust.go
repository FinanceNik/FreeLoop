package engine

import (
	"context"
	"errors"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Project trust (SEC-006) and project settings (SET-002). Until the user
// trusts a project the app reads none of its instruction files or
// .freeloop/ settings and runs no git command its config can extend. A
// trusted project's .freeloop/ can add profiles and deny rules; it can
// tighten permissions but never loosen them. Its files are read through the
// project's environment, like every project file.

// ProjectDir is the project-level settings folder.
const ProjectDir = ".freeloop"

// projectConfigTTL is how long a project's settings are reused before its
// files are read again; permission checks read them on every tool call.
const projectConfigTTL = 2 * time.Second

// ProjectSettings is what a project adds, as the new-session form shows it.
type ProjectSettings struct {
	// ProjectID is empty for a folder that is not a project yet.
	ProjectID string            `json:"projectId"`
	Trusted   bool              `json:"trusted"`
	Profiles  []config.Profile  `json:"profiles"`
	Rules     []permission.Rule `json:"rules"`
	Issues    []config.Issue    `json:"issues"`
}

type projectConfig struct {
	ProjectSettings
	// rulesErr is set when .freeloop/rules.yaml has errors; the broker
	// then fails closed for the project's sessions.
	rulesErr error
	at       time.Time
}

type projectCache struct {
	mu    sync.Mutex
	byDir map[string]projectConfig
}

// projectConfig returns a project's settings, read at most every
// projectConfigTTL; an untrusted project has none.
func (e *Engine) projectConfig(p session.Project) projectConfig {
	if !p.Trusted {
		return projectConfig{ProjectSettings: ProjectSettings{ProjectID: p.ID, Profiles: []config.Profile{}, Rules: []permission.Rule{}, Issues: []config.Issue{}}}
	}
	e.projects.mu.Lock()
	defer e.projects.mu.Unlock()
	if c, ok := e.projects.byDir[p.Root]; ok && time.Since(c.at) < projectConfigTTL {
		return c
	}
	c := readProjectConfig(e, p)
	if e.projects.byDir == nil {
		e.projects.byDir = map[string]projectConfig{}
	}
	e.projects.byDir[p.Root] = c
	return c
}

func readProjectConfig(e *Engine, p session.Project) projectConfig {
	c := projectConfig{ProjectSettings: ProjectSettings{ProjectID: p.ID, Trusted: true, Profiles: []config.Profile{}, Rules: []permission.Rule{}, Issues: []config.Issue{}}, at: time.Now()}
	env, err := e.localEnv(p.Root, nil)
	if err != nil || !environment.IsDir(p.Root) {
		return c
	}
	fs := env.FS()
	rulesFile := path.Join(ProjectDir, config.RulesFileName)
	if b, err := fs.ReadFile(rulesFile); err == nil {
		rules, issues, err := config.ParseRules(b, permission.ScopeProject, rulesFile)
		c.Issues = append(c.Issues, issues...)
		if err != nil {
			c.rulesErr = errors.New(rulesFile + " has errors; the project's sessions ask for everything until it is fixed")
		}
		for _, r := range rules {
			if r.Effect != permission.Deny {
				c.Issues = append(c.Issues, config.Issue{File: rulesFile, Message: "project rules can only deny; the allow rule " + r.String() + " is ignored"})
				continue
			}
			c.Rules = append(c.Rules, r)
		}
	}
	dir := path.Join(ProjectDir, config.ProfilesDir)
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return c
	}
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name, ".yaml")
		if !ok || de.IsDir {
			continue
		}
		file := path.Join(dir, de.Name)
		b, err := fs.ReadFile(file)
		if err != nil {
			c.Issues = append(c.Issues, config.Issue{File: file, Message: err.Error()})
			continue
		}
		prof, issue := config.ParseProjectProfile(id, b, builtin.ToolNames(), file)
		if issue != nil {
			c.Issues = append(c.Issues, *issue)
			continue
		}
		c.Profiles = append(c.Profiles, prof)
	}
	return c
}

// ProjectSettings returns the trust state and .freeloop/ settings of the
// project at root, for the new-session form. A folder that is not a project
// yet is untrusted.
func (e *Engine) ProjectSettings(ctx context.Context, root string) (ProjectSettings, error) {
	empty := ProjectSettings{Profiles: []config.Profile{}, Rules: []permission.Rule{}, Issues: []config.Issue{}}
	if root == "" || !environment.IsDir(root) {
		return empty, nil
	}
	p, err := e.store.ProjectByRoot(ctx, environment.Canonical(root))
	if err != nil {
		return empty, nil
	}
	return e.projectConfig(p).ProjectSettings, nil
}

// SetProjectTrust trusts or distrusts the project at root, adding it first
// when needed. Running sessions see the change at their next model call
// (instruction files), permission check (rules) and git refresh; a project
// profile a session started with stays until the session restarts.
func (e *Engine) SetProjectTrust(ctx context.Context, root string, trusted bool) (ProjectInfo, error) {
	p, err := e.AddProject(ctx, root)
	if err != nil {
		return ProjectInfo{}, err
	}
	p.Trusted = trusted
	if err := e.store.UpdateProject(ctx, p); err != nil {
		return ProjectInfo{}, err
	}
	e.projects.mu.Lock()
	delete(e.projects.byDir, p.Root)
	e.projects.mu.Unlock()
	e.forgetSkills()
	list, err := e.store.ListSessions(ctx)
	if err == nil {
		for _, s := range list {
			if s.ProjectID == p.ID {
				e.gitChanged(s.ID)
			}
		}
	}
	return ProjectInfo{Project: p, Missing: false}, nil
}

// trusted reports whether the project with id is trusted.
func (e *Engine) trusted(projectID string) bool {
	p, err := e.store.GetProject(context.Background(), projectID)
	return err == nil && p.Trusted
}

// sessionProject returns a session's project; a session from before
// projects existed has none and counts as untrusted.
func (e *Engine) sessionProject(ctx context.Context, ses session.Session) session.Project {
	p, err := e.store.GetProject(ctx, ses.ProjectID)
	if err != nil {
		return session.Project{ID: ses.ProjectID, Root: ses.ProjectRoot}
	}
	return p
}

// scopedRules returns a session's environment, profile and project rules for the
// broker, with the error that makes it fail closed.
func (e *Engine) scopedRules(sid string) ([]permission.Rule, error) {
	ctx := context.Background()
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, nil
	}
	project := e.sessionProject(ctx, ses)
	pc := e.projectConfig(project)
	var out []permission.Rule
	envID := ses.EnvironmentID
	if envID == "" {
		envID = DefaultEnvironmentID
	}
	if env, err := e.environment(envID); err == nil {
		out = append(out, env.Rules...)
	}
	if prof, err := e.profileFor(project, ses.ProfileID); err == nil {
		out = append(out, prof.Rules...)
	}
	return append(out, pc.Rules...), pc.rulesErr
}
