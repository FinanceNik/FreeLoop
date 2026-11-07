package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// Project trust (SEC-006) and project settings (SET-002). Until the user
// trusts a project the app reads none of its instruction files or
// .freeloop/ settings and runs no git command its config can extend. A
// trusted project's .freeloop/ can add profiles and deny rules; it can
// tighten permissions but never loosen them. Its files are read through the
// project's environment, like every project file.

// ProjectDir is the project-level settings folder.
const ProjectDir = permission.ProjectSettingsDir

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
	// Hooks are the commands the project's .freeloop/ defines in
	// hooks.yaml and its profiles (EXT-005), listed whether or not the
	// project is trusted so the trust prompt can show them.
	Hooks []config.Hook `json:"hooks"`
	// Worktree is the project's .freeloop/worktree.yaml (SES-016), shown
	// whether or not the project is trusted, like Hooks.
	Worktree config.WorktreeScripts `json:"worktree"`
	// AgentConfig is what the repository brings for the agents themselves, shown
	// like Hooks: an agent reads it, and once the project is trusted Claude Code runs
	// the hooks in it (SEC-006, SEC-015).
	AgentConfig []AgentConfigFile `json:"agentConfig"`
	Issues      []config.Issue    `json:"issues"`
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
		return projectConfig{ProjectSettings: ProjectSettings{ProjectID: p.ID, Profiles: []config.Profile{}, Rules: []permission.Rule{}, Hooks: []config.Hook{}, Issues: []config.Issue{}}}
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
	c := projectConfig{ProjectSettings: ProjectSettings{ProjectID: p.ID, Trusted: true, Profiles: []config.Profile{}, Rules: []permission.Rule{}, Hooks: []config.Hook{}, Issues: []config.Issue{}}, at: time.Now()}
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
	hooksFile := path.Join(ProjectDir, config.HooksFileName)
	if b, err := fs.ReadFile(hooksFile); err == nil {
		hooks, issues := config.ParseHooks(b, hooksFile)
		c.Hooks = append(c.Hooks, hooks...)
		c.Issues = append(c.Issues, issues...)
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
	empty := ProjectSettings{AgentConfig: []AgentConfigFile{}, Profiles: []config.Profile{}, Rules: []permission.Rule{}, Hooks: []config.Hook{}, Issues: []config.Issue{}}
	if root == "" || !environment.IsDir(root) {
		return empty, nil
	}
	p, err := e.store.ProjectByRoot(ctx, environment.Canonical(root))
	if err != nil {
		// Not a project yet: nothing applies, but the trust prompt shows
		// what trusting would run.
		empty.Hooks = e.previewHooks(environment.Canonical(root))
		empty.Worktree, _ = e.repoScripts(environment.Canonical(root))
		empty.AgentConfig = e.agentConfig(environment.Canonical(root))
		return empty, nil
	}
	s := e.projectConfig(p).ProjectSettings
	s.Hooks = e.previewHooks(p.Root)
	s.Worktree, _ = e.repoScripts(p.Root)
	s.AgentConfig = e.agentConfig(p.Root)
	return s, nil
}

// AgentConfigFile is one file a repository brings for an agent, with what matters in
// it for the user deciding whether to trust the project.
type AgentConfigFile struct {
	Path string `json:"path"`
	// Hooks are the events whose commands Claude Code runs, once trusted, without
	// asking anyone.
	Hooks []string `json:"hooks,omitempty"`
	// Allows says the file lets the agent run commands without asking it. The app's
	// policy still decides; this says the repository tried.
	Allows bool `json:"allows,omitempty"`
}

// agentConfigFiles are the files agents read from a project.
var agentConfigFiles = []string{".claude/settings.json", ".claude/settings.local.json", ".mcp.json", ".codex/config.toml", ".gemini/settings.json", "opencode.json"}

// agentConfig reads what a repository brings for the agents. Reading shows it; only
// trust lets an agent use it.
func (e *Engine) agentConfig(root string) []AgentConfigFile {
	out := []AgentConfigFile{}
	env, err := e.localEnv(root, nil)
	if err != nil {
		return out
	}
	for _, name := range agentConfigFiles {
		b, err := env.FS().ReadFile(name)
		if err != nil {
			continue
		}
		f := AgentConfigFile{Path: name}
		var claude struct {
			Hooks       map[string]json.RawMessage `json:"hooks"`
			Permissions struct {
				Allow       []string `json:"allow"`
				DefaultMode string   `json:"defaultMode"`
			} `json:"permissions"`
		}
		if strings.HasPrefix(name, ".claude/") && json.Unmarshal(b, &claude) == nil {
			f.Hooks = slices.Sorted(maps.Keys(claude.Hooks))
			f.Allows = len(claude.Permissions.Allow) > 0 || claude.Permissions.DefaultMode == "bypassPermissions" || claude.Permissions.DefaultMode == "acceptEdits"
		}
		text := string(b)
		switch name {
		case ".codex/config.toml":
			f.Allows = strings.Contains(text, `approval_policy = "never"`) || strings.Contains(text, "danger-full-access")
		case ".gemini/settings.json":
			f.Allows = strings.Contains(text, "yolo") || strings.Contains(text, `"autoAccept": true`) || strings.Contains(text, `"autoAccept":true`)
		case "opencode.json":
			f.Allows = strings.Contains(text, `"allow"`)
		}
		out = append(out, f)
	}
	return out
}

// previewHooks lists the hooks a project's .freeloop/ defines, in
// .freeloop/hooks.yaml and its profiles, for the trust prompt and the
// new-session form. Reading them shows them; only trust makes them run.
func (e *Engine) previewHooks(root string) []config.Hook {
	hooks := []config.Hook{}
	env, err := e.localEnv(root, nil)
	if err != nil {
		return hooks
	}
	fs := env.FS()
	file := path.Join(ProjectDir, config.HooksFileName)
	if b, err := fs.ReadFile(file); err == nil {
		h, _ := config.ParseHooks(b, file)
		hooks = append(hooks, h...)
	}
	dir := path.Join(ProjectDir, config.ProfilesDir)
	entries, _ := fs.ReadDir(dir)
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name, ".yaml")
		if !ok || de.IsDir {
			continue
		}
		b, err := fs.ReadFile(path.Join(dir, de.Name))
		if err != nil {
			continue
		}
		prof, issue := config.ParseProjectProfile(id, b, builtin.ToolNames(), path.Join(dir, de.Name))
		if issue != nil {
			continue
		}
		for _, h := range prof.Hooks {
			h.Source = path.Join(dir, de.Name)
			hooks = append(hooks, h)
		}
	}
	return hooks
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
	if !trusted {
		e.dropBypass(ctx)
	}
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
//
// What cannot be read may hold deny rules, so each failure is an error: the broker
// then asks for everything but reads in the project, as for a broken rules.yaml,
// instead of judging by what is left (SEC-002). A session from before projects
// existed has none, and counts as untrusted.
func (e *Engine) scopedRules(sid string) ([]permission.Rule, error) {
	ctx := context.Background()
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("the session's rules: %w", err)
	}
	project, err := e.store.GetProject(ctx, ses.ProjectID)
	if errors.Is(err, store.ErrProjectNotFound) {
		project, err = session.Project{ID: ses.ProjectID, Root: ses.ProjectRoot}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the project's rules: %w", err)
	}
	pc := e.projectConfig(project)
	var out []permission.Rule
	envID := ses.EnvironmentID
	if envID == "" {
		envID = DefaultEnvironmentID
	}
	env, err := e.environment(envID)
	if err != nil {
		return nil, fmt.Errorf("the environment's rules: %w", err)
	}
	out = append(out, env.Rules...)
	prof, err := e.profileFor(project, ses.ProfileID)
	if err != nil {
		return nil, fmt.Errorf("the profile's rules: %w", err)
	}
	out = append(out, prof.Rules...)
	return append(out, pc.Rules...), pc.rulesErr
}
