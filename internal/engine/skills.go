package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/skill"
)

// Skills (SKL-001, SKL-002, SKL-004) come from the app's skill store
// (skills/ in the data folder) and, in a trusted project, from
// .freeloop/skills/, .agents/skills/ and .claude/skills/, all read-only.
// A skill is on unless it is turned off globally (config.yaml), in the
// project, or left out of the session's profile.

// SkillsDir is the app's skill store, inside the data folder.
const SkillsDir = config.SkillsDir

// maxSkillFile caps a skill file load_skill returns.
const maxSkillFile = 256 << 10

// projectSkillDirs are a project's skill folders in precedence order.
var projectSkillDirs = []struct{ dir, source string }{
	{ProjectDir + "/skills", skill.SourceFreeloop},
	{".agents/skills", skill.SourceAgents},
	{".claude/skills", skill.SourceClaude},
}

// SkillInfo is a skill as the skills page shows it.
type SkillInfo struct {
	skill.Skill
	// On globally and, for a project, in the project.
	Global  bool `json:"global"`
	Project bool `json:"project"`
}

// SkillList is what ListSkills returns.
type SkillList struct {
	Skills []SkillInfo   `json:"skills"`
	Issues []skill.Issue `json:"issues"`
	// ProjectID is set when the list includes a trusted project's skills.
	ProjectID string `json:"projectId"`
}

type discovered struct {
	skills []skill.Skill
	issues []skill.Issue
	at     time.Time
}

type skillCache struct {
	mu     sync.Mutex
	byRoot map[string]discovered // "" is the app store alone
}

// insideGuard lets the app read only inside one folder: its skill store,
// which the protected-path policy keeps from the agent's file tools.
type insideGuard struct{ root string }

func (g insideGuard) CheckRead(p string) error {
	if p == g.root || strings.HasPrefix(p, g.root+string(filepath.Separator)) {
		return nil
	}
	return fmt.Errorf("%w: %s is outside %s", environment.ErrProtected, p, g.root)
}

func (g insideGuard) CheckWrite(p string) error {
	return fmt.Errorf("%w: the skill store is read-only here", environment.ErrProtected)
}

// appSkillsFS is the app store's file system, rooted at the data folder's
// skills/; nil when it does not exist.
func (e *Engine) appSkillsFS() (skill.FS, string) {
	root := filepath.Join(e.dataDir, SkillsDir)
	if !environment.IsDir(root) {
		return nil, ""
	}
	root = environment.Canonical(root)
	env, err := environment.NewLocal(environment.LocalOptions{ID: "skills", Root: root, Guard: insideGuard{root}})
	if err != nil {
		return nil, ""
	}
	return env.FS(), env.Root()
}

// fsFor returns the file system a discovered skill is read through.
func (e *Engine) fsFor(project session.Project, s skill.Skill) (skill.FS, error) {
	if s.Source == skill.SourceApp {
		fs, _ := e.appSkillsFS()
		if fs == nil {
			return nil, errors.New("the skill store is missing")
		}
		return fs, nil
	}
	env, err := e.localEnv(project.Root, nil)
	if err != nil {
		return nil, err
	}
	return env.FS(), nil
}

// discoverSkills lists the skills a project's sessions can see, before
// switches, cached for projectConfigTTL. An untrusted project has only the
// app store's.
func (e *Engine) discoverSkills(project session.Project) discovered {
	key := ""
	if project.Trusted {
		key = project.Root
	}
	e.skillCache.mu.Lock()
	defer e.skillCache.mu.Unlock()
	if d, ok := e.skillCache.byRoot[key]; ok && time.Since(d.at) < projectConfigTTL {
		return d
	}
	var lists [][]skill.Skill
	var issues []skill.Issue
	if key != "" && environment.IsDir(project.Root) {
		if env, err := e.localEnv(project.Root, nil); err == nil {
			for _, d := range projectSkillDirs {
				l, is := skill.Discover(env.FS(), d.dir, d.source, filepath.Join(env.Root(), filepath.FromSlash(d.dir)))
				lists, issues = append(lists, l), append(issues, is...)
			}
		}
	}
	if fs, root := e.appSkillsFS(); fs != nil {
		l, is := skill.Discover(fs, ".", skill.SourceApp, root)
		lists, issues = append(lists, l), append(issues, is...)
	}
	d := discovered{skills: skill.Merge(lists...), issues: issues, at: time.Now()}
	if d.skills == nil {
		d.skills = []skill.Skill{}
	}
	if d.issues == nil {
		d.issues = []skill.Issue{}
	}
	if e.skillCache.byRoot == nil {
		e.skillCache.byRoot = map[string]discovered{}
	}
	e.skillCache.byRoot[key] = d
	return d
}

func (e *Engine) forgetSkills() {
	e.skillCache.mu.Lock()
	e.skillCache.byRoot = nil
	e.skillCache.mu.Unlock()
}

// enabledSkills applies the global, project and profile switches.
func (e *Engine) enabledSkills(project session.Project, prof config.Profile) []skill.Skill {
	off := e.Config().Skills.Disabled
	var out []skill.Skill
	for _, s := range e.discoverSkills(project).skills {
		if slices.Contains(off, s.Name) || slices.Contains(project.DisabledSkills, s.Name) {
			continue
		}
		if len(prof.Skills) > 0 && !slices.Contains(prof.Skills, s.Name) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// ListSkills lists every skill a session in the project at root could
// use, with its switches; with an empty root, the app store's.
func (e *Engine) ListSkills(ctx context.Context, root string) (SkillList, error) {
	var project session.Project
	if root != "" && environment.IsDir(root) {
		if p, err := e.store.ProjectByRoot(ctx, environment.Canonical(root)); err == nil {
			project = p
		}
	}
	d := e.discoverSkills(project)
	off := e.Config().Skills.Disabled
	out := SkillList{Skills: []SkillInfo{}, Issues: d.issues}
	if project.Trusted {
		out.ProjectID = project.ID
	}
	for _, s := range d.skills {
		out.Skills = append(out.Skills, SkillInfo{Skill: s, Global: !slices.Contains(off, s.Name), Project: !slices.Contains(project.DisabledSkills, s.Name)})
	}
	return out, nil
}

// SetSkillEnabled turns a skill on or off globally (projectID "") or in a
// project.
func (e *Engine) SetSkillEnabled(ctx context.Context, name, projectID string, enabled bool) error {
	if !skill.ValidName(name) {
		return fmt.Errorf("%q is not a skill name", name)
	}
	toggle := func(list []string) []string {
		list = slices.DeleteFunc(slices.Clone(list), func(n string) bool { return n == name })
		if !enabled {
			list = append(list, name)
		}
		slices.Sort(list)
		return list
	}
	if projectID == "" {
		cfg := e.Config()
		cfg.Skills.Disabled = toggle(cfg.Skills.Disabled)
		_, err := e.SaveSettings(cfg)
		return err
	}
	p, err := e.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	p.DisabledSkills = toggle(p.DisabledSkills)
	return e.store.UpdateProject(ctx, p)
}

// sessionSkills is a session's harness.SkillSource. It looks up the
// project and profile at every call, so switches apply at once.
type sessionSkills struct {
	e         *Engine
	projectID string
	root      string
	profileID string
}

func (s sessionSkills) current() (session.Project, []skill.Skill) {
	project, err := s.e.store.GetProject(context.Background(), s.projectID)
	if err != nil {
		project = session.Project{ID: s.projectID, Root: s.root}
	}
	prof, err := s.e.profileFor(project, s.profileID)
	if err != nil {
		prof = config.BuiltinDefaultProfile()
	}
	return project, s.e.enabledSkills(project, prof)
}

func (s sessionSkills) Enabled() []harness.SkillEntry {
	_, list := s.current()
	out := make([]harness.SkillEntry, 0, len(list))
	for _, sk := range list {
		out = append(out, harness.SkillEntry{Name: sk.Name, Description: sk.Description})
	}
	return out
}

func (s sessionSkills) Load(name, file string) (harness.SkillContent, error) {
	project, list := s.current()
	i := slices.IndexFunc(list, func(sk skill.Skill) bool { return sk.Name == name })
	if i < 0 {
		return harness.SkillContent{}, fmt.Errorf("no enabled skill named %q", name)
	}
	sk := list[i]
	fs, err := s.e.fsFor(project, sk)
	if err != nil {
		return harness.SkillContent{}, err
	}
	out := harness.SkillContent{Dir: sk.Path}
	if file == "" {
		if out.Text, err = skill.Body(fs, sk); err != nil {
			return harness.SkillContent{}, err
		}
		out.Files = skill.Files(fs, sk, 100)
		return out, nil
	}
	file = filepath.ToSlash(filepath.Clean(file))
	if !filepath.IsLocal(file) {
		return harness.SkillContent{}, fmt.Errorf("%q is not a file inside the skill", file)
	}
	b, err := fs.ReadFile(path.Join(sk.Dir, file))
	if err != nil {
		return harness.SkillContent{}, err
	}
	if len(b) > maxSkillFile {
		return harness.SkillContent{}, fmt.Errorf("%s is %d bytes; load_skill reads files up to %d", file, len(b), maxSkillFile)
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return harness.SkillContent{}, fmt.Errorf("%s is not a text file", file)
	}
	out.Text = string(b)
	return out, nil
}
