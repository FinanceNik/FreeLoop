package engine

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// PolicyQuery names a call to judge (SEC-018): in a session, or in a project with
// the profile and environment a new session there would have. Empty fields take
// the session's or the defaults.
type PolicyQuery struct {
	SessionID     string `json:"sessionId"`
	ProjectRoot   string `json:"projectRoot"`
	ProfileID     string `json:"profileId"`
	EnvironmentID string `json:"environmentId"`
	Mode          string `json:"mode"`
	// Tool is shell, edit, read, search or fetch, or a tool's own name such as
	// write_file or mcp__github__create_issue.
	Tool    string `json:"tool"`
	Command string `json:"command"`
	Path    string `json:"path"`
}

// PolicyCheck is what the broker would do with the call, and what else holds.
type PolicyCheck struct {
	permission.Verdict
	Tool    string `json:"tool"`
	Mode    string `json:"mode"`
	Root    string `json:"root"`
	Harness string `json:"harness"`
	// Hooks would still run for the call and can deny it, never allow it; one
	// that fails puts the call to the user.
	Hooks []config.Hook `json:"hooks"`
	// Pending are allow rules that match but wait for the user's approval.
	Pending []permission.Rule `json:"pending"`
	// Agent says what the session's agent decides by itself, out of reach.
	Agent []string `json:"agent"`
}

// PolicyScope is one place rules come from, in the order they are read.
type PolicyScope struct {
	Scope permission.Scope  `json:"scope"`
	Name  string            `json:"name"`
	Note  string            `json:"note"`
	Rules []permission.Rule `json:"rules"`
}

// PolicyHook is a hook in force, or one that waits for approval.
type PolicyHook struct {
	config.Hook
	Approved bool `json:"approved"`
}

// PolicyOverview is everything that decides a call, for a session or a project.
type PolicyOverview struct {
	Root        string        `json:"root"`
	Trusted     bool          `json:"trusted"`
	Mode        string        `json:"mode"`
	Harness     string        `json:"harness"`
	HarnessName string        `json:"harnessName"`
	Sandboxed   bool          `json:"sandboxed"`
	Scopes      []PolicyScope `json:"scopes"`
	// RulesUnreadable says some rules could not be read, so the broker asks.
	RulesUnreadable string       `json:"rulesUnreadable"`
	Protected       []string     `json:"protected"`
	NeverWritten    []string     `json:"neverWritten"`
	ActedOnLater    []string     `json:"actedOnLater"`
	Hooks           []PolicyHook `json:"hooks"`
	Agent           []string     `json:"agent"`
}

// policyScene is what a query is judged in.
type policyScene struct {
	sid      string
	root     string
	mode     permission.Mode
	project  session.Project
	profile  config.Profile
	env      config.Environment
	harness  string
	preset   *config.HarnessPreset
	rules    []permission.Rule
	rulesErr error
}

func (e *Engine) policyScene(ctx context.Context, q PolicyQuery) (policyScene, error) {
	var sc policyScene
	envID, profileID, mode := q.EnvironmentID, q.ProfileID, q.Mode
	if q.SessionID != "" {
		ses, err := e.store.GetSession(ctx, q.SessionID)
		if err != nil {
			return sc, err
		}
		sc.sid, sc.project = ses.ID, e.sessionProject(ctx, ses)
		sc.root = cmpOr(ses.Worktree, ses.ProjectRoot)
		envID, profileID, mode = cmpOr(envID, ses.EnvironmentID), cmpOr(profileID, ses.ProfileID), cmpOr(mode, ses.Mode)
	} else {
		if q.ProjectRoot == "" {
			return sc, errors.New("name a session or a project folder")
		}
		root, err := filepath.Abs(q.ProjectRoot)
		if err != nil {
			return sc, err
		}
		root = e.projectRootOf(ctx, environment.Canonical(root))
		// A folder never opened has no trust and no settings, as a new project would.
		sc.project = session.Project{Root: root}
		if p, err := e.store.ProjectByRoot(ctx, root); err == nil {
			sc.project = p
		}
		sc.root = root
		envID = cmpOr(envID, sc.project.DefaultEnvironmentID)
		profileID = cmpOr(profileID, sc.project.DefaultProfileID, e.Config().Defaults.ProfileID)
	}
	env, err := e.environment(cmpOr(envID, DefaultEnvironmentID))
	if err != nil {
		return sc, err
	}
	prof, err := e.profileFor(sc.project, profileID)
	if err != nil {
		return sc, err
	}
	sc.env, sc.profile = env, prof
	if mode == "" {
		// As a new session starts: never in a mode only the user chooses (SEC-001).
		mode = cmpOr(e.startMode(prof), env.DefaultMode, string(permission.ModeAsk))
		if m := permission.Mode(mode); !m.Valid() || m.Unattended() {
			mode = string(permission.ModeAsk)
		}
	}
	sc.mode = permission.Mode(mode)
	if !sc.mode.Valid() {
		return sc, fmt.Errorf("no mode %q; the modes are ask, accept_edits, plan, auto and bypass", mode)
	}
	sc.harness = cmpOr(prof.Harness, builtin.ID)
	if sc.harness != builtin.ID {
		if p, err := e.preset(sc.harness); err == nil {
			sc.preset = &p
		}
	}
	if sc.sid != "" {
		sc.rules, sc.rulesErr = e.rules.all(sc.sid)
	} else {
		pc := e.projectConfig(sc.project)
		scoped := append(append(slices.Clone(env.Rules), prof.Rules...), pc.Rules...)
		sc.rules, sc.rulesErr = e.rules.withGlobal(scoped, pc.rulesErr)
	}
	return sc, nil
}

// projectRootOf is the project a folder belongs to: the nearest folder at or above it
// that is a project the app knows, else the nearest with a git repository, else the
// folder itself; so a check from a subfolder judges the project, not the subfolder.
func (e *Engine) projectRootOf(ctx context.Context, dir string) string {
	repo := ""
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := e.store.ProjectByRoot(ctx, d); err == nil {
			return d
		}
		if repo == "" && environment.IsDir(filepath.Join(d, ".git")) {
			repo = d
		}
		if filepath.Dir(d) == d || d == e.home {
			break
		}
	}
	return cmpOr(repo, dir)
}

// agentNotes says what the scene's agent decides by itself, in mode; "" for every mode.
func (sc policyScene) agentNotes(mode permission.Mode) []string {
	if sc.preset == nil {
		return []string{}
	}
	out := []string{sc.preset.Name + " runs its own tools in its own process: what it does without asking, under its own settings, never reaches Freeloop. Only what it asks about is judged here."}
	for _, n := range sc.preset.DecidesItself {
		if n.Mode == "" || mode == "" || permission.Mode(n.Mode) == mode {
			out = append(out, n.Text)
		}
	}
	return out
}

// toolKinds maps the short tool names a person types onto a tool and its kind,
// as the harnesses name them, so rules match the same way.
var toolKinds = map[string]struct {
	tool string
	kind permission.Kind
}{
	"shell": {"shell", permission.KindExecute}, "bash": {"shell", permission.KindExecute},
	"edit": {"edit_file", permission.KindEdit}, "edit_file": {"edit_file", permission.KindEdit},
	"write": {"write_file", permission.KindEdit}, "write_file": {"write_file", permission.KindEdit},
	"delete": {"edit_file", permission.KindDelete},
	"read":   {"read_file", permission.KindRead}, "read_file": {"read_file", permission.KindRead},
	"search": {"grep", permission.KindSearch}, "grep": {"grep", permission.KindSearch},
	"fetch": {"fetch", permission.KindFetch}, "web_fetch": {"web_fetch", permission.KindFetch},
}

// CheckPolicy says what the broker would do with a call, through the broker's own
// judgment, without making the call, asking anyone or running a hook (SEC-018).
func (e *Engine) CheckPolicy(ctx context.Context, q PolicyQuery) (PolicyCheck, error) {
	sc, err := e.policyScene(ctx, q)
	if err != nil {
		return PolicyCheck{}, err
	}
	name := strings.TrimSpace(q.Tool)
	if name == "" {
		return PolicyCheck{}, errors.New("name a tool: shell, edit, read, search, fetch or a tool's own name")
	}
	req := permission.Request{SessionID: sc.sid, Tool: name, Kind: permission.KindOther, Command: strings.TrimSpace(q.Command)}
	if k, ok := toolKinds[strings.ToLower(name)]; ok {
		req.Tool, req.Kind = k.tool, k.kind
	}
	if req.Kind == permission.KindExecute && req.Command == "" {
		return PolicyCheck{}, errors.New("name the command to check")
	}
	if p := strings.TrimSpace(q.Path); p != "" {
		if strings.HasPrefix(p, "~/") {
			p = filepath.Join(e.home, p[2:])
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(sc.root, p)
		}
		req.Paths = []string{filepath.Clean(p)}
	} else if req.Kind.Files() && req.Kind != permission.KindSearch {
		return PolicyCheck{}, errors.New("name the path to check")
	}
	// Only a command Freeloop runs itself can be held in the sandbox; an external
	// agent runs the commands it asked about in its own process (ENV-009).
	if req.Kind == permission.KindExecute && sc.harness == builtin.ID && sc.env.SandboxOn() {
		req.Contained = permission.ContainedProject
		if sc.mode == permission.ModePlan {
			req.Contained = permission.ContainedReadOnly
		}
	}
	inForce := slices.DeleteFunc(slices.Clone(sc.rules), e.rules.pending)
	out := PolicyCheck{
		Verdict: e.broker.Explain(permission.Context{Mode: sc.mode, Root: sc.root}, req, inForce, sc.rulesErr),
		Tool:    req.Tool, Mode: string(sc.mode), Root: sc.root, Harness: sc.harness,
		Hooks: []config.Hook{}, Pending: []permission.Rule{}, Agent: sc.agentNotes(sc.mode),
	}
	for _, r := range sc.rules {
		if e.rules.pending(r) && r.Matches(req, sc.root) {
			out.Pending = append(out.Pending, r)
		}
	}
	if out.Outcome != string(permission.Denied) {
		for _, h := range e.policyHooks(sc) {
			if !h.Approved || (h.Event != config.HookPreToolUse && h.Event != config.HookPermissionRequest) {
				continue
			}
			// A permission_request hook runs only when the user would be asked.
			if h.Event == config.HookPermissionRequest && out.Outcome != permission.Ask {
				continue
			}
			if ok, _ := path.Match(cmpOr(h.Match, "*"), req.Tool); ok {
				out.Hooks = append(out.Hooks, h.Hook)
			}
		}
	}
	return out, nil
}

// policyHooks lists the hooks for a scene, as hooksOf gathers them for a session.
func (e *Engine) policyHooks(sc policyScene) []PolicyHook {
	var out []PolicyHook
	add := func(h config.Hook, source string, needsApproval bool) {
		h.Source = source
		out = append(out, PolicyHook{Hook: h, Approved: !needsApproval || e.commandApproved(h.Fingerprint())})
	}
	for _, h := range e.Config().Hooks {
		add(h, config.FileName, true)
	}
	// A project's hooks, its profiles' included, run as the set approved.
	pc := e.projectConfig(sc.project)
	projectOK := e.projectHooksApproved(sc.project, pc)
	for _, h := range sc.profile.Hooks {
		if strings.HasPrefix(sc.profile.ID, config.ProjectProfilePrefix) {
			h.Source = "profile " + sc.profile.Name
			out = append(out, PolicyHook{Hook: h, Approved: projectOK})
			continue
		}
		add(h, "profile "+sc.profile.Name, true)
	}
	for _, h := range pc.Hooks {
		h.Source = cmpOr(h.Source, "the project")
		out = append(out, PolicyHook{Hook: h, Approved: projectOK})
	}
	return out
}

// PolicyOverview lists what decides a call in a session or a project (SEC-018):
// every scope in order, the protected paths, the mode, the hooks, and what the agent
// decides itself.
func (e *Engine) PolicyOverview(ctx context.Context, q PolicyQuery) (PolicyOverview, error) {
	sc, err := e.policyScene(ctx, q)
	if err != nil {
		return PolicyOverview{}, err
	}
	out := PolicyOverview{
		Root: sc.root, Trusted: sc.project.Trusted, Mode: string(sc.mode), Harness: sc.harness, HarnessName: "Built-in",
		Sandboxed: sc.env.SandboxOn(), Protected: e.broker.Policy().Protected(),
		NeverWritten: []string{".git", permission.ProjectSettingsDir}, ActedOnLater: slices.Clone(permission.ActedOnLater),
		Hooks: e.policyHooks(sc), Agent: sc.agentNotes(""),
	}
	if out.Hooks == nil {
		out.Hooks = []PolicyHook{}
	}
	if sc.preset != nil {
		out.HarnessName = sc.preset.Name
	}
	if sc.rulesErr != nil {
		out.RulesUnreadable = sc.rulesErr.Error()
	}
	scopes := []PolicyScope{
		{Scope: permission.ScopeGlobal, Name: "Global", Note: "rules.yaml, for every session"},
		{Scope: permission.ScopeEnvironment, Name: "Environment " + sc.env.Name, Note: "the environment's file"},
		{Scope: permission.ScopeProfile, Name: "Profile " + sc.profile.Name, Note: "the profile's file"},
		{Scope: permission.ScopeProject, Name: "Project", Note: "the project's .freeloop/rules.yaml: it can only deny, and only once the project is trusted"},
		{Scope: permission.ScopeSession, Name: "Session", Note: "what you allowed always in this session"},
	}
	for i := range scopes {
		scopes[i].Rules = []permission.Rule{}
		for _, r := range sc.rules {
			if r.Scope == scopes[i].Scope {
				r.Pending = e.rules.pending(r)
				scopes[i].Rules = append(scopes[i].Rules, r)
			}
		}
	}
	if sc.sid == "" {
		scopes = scopes[:len(scopes)-1]
	}
	out.Scopes = scopes
	return out, nil
}
