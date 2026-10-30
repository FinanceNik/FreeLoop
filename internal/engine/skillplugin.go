package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/session"
	"github.com/FinanceNik/Freeloop/internal/skill"
)

// Skills for external agents (SKL-008, OQ-006). An agent whose preset says
// it takes a Claude Code plugin gets one per session: a folder in the app's
// data folder (session-skills/<id>/) with a plugin manifest and a copy of
// each skill enabled for the session, passed in session/new. Nothing is
// written where the agent keeps its own configuration (~/.claude, the
// project's .claude/). Skills Claude Code finds by itself are left out: a
// project's .claude/skills, and store skills imported from ~/.claude/skills.
// The folder is built when the agent starts, so a switch flipped later
// reaches it at the next start.

// Caps on what a session's plugin copies.
const (
	maxPluginBytes = 32 << 20
	maxPluginFile  = 4 << 20
)

// skillPluginName names the plugin; its skills show as freeloop:<name>.
const skillPluginName = "freeloop"

// skillPlugin builds session sid's plugin folder and returns it, or "" when
// no skill goes into it.
func (e *Engine) skillPlugin(sid string, project session.Project, prof config.Profile) (string, error) {
	fromClaude := filepath.Join(e.home, ".claude", "skills") + string(filepath.Separator)
	files := map[string][]byte{}
	var names []string
	total := 0
	for _, s := range e.enabledSkills(project, prof) {
		if s.Source == skill.SourceClaude {
			continue
		}
		if s.Source == skill.SourceApp {
			if prov, ok := config.ReadSkillProvenance(e.dataDir, s.Name); ok && strings.HasPrefix(prov.Source, fromClaude) {
				continue
			}
		}
		fs, err := e.fsFor(project, s)
		if err != nil {
			log.Printf("engine: skill %s for session %s: %v", s.Name, sid, err)
			continue
		}
		one := map[string][]byte{}
		size := 0
		if err := copySkillTree(fs, s.Dir, s.Path, s.Path, "skills/"+s.Name, one, &size); err != nil {
			log.Printf("engine: skill %s for session %s: %v", s.Name, sid, err)
			continue
		}
		if total+size > maxPluginBytes {
			log.Printf("engine: skill %s left out of session %s's plugin: over %d MB in all", s.Name, sid, maxPluginBytes>>20)
			continue
		}
		total += size
		for p, b := range one {
			files[p] = b
		}
		names = append(names, s.Name)
	}
	if len(names) == 0 {
		// The folder stays, empty, for a container that mounts it.
		_, err := config.WriteSessionPlugin(e.dataDir, sid, nil)
		return "", err
	}
	manifest, _ := json.MarshalIndent(map[string]any{
		"name":        skillPluginName,
		"version":     "1.0.0",
		"description": "Skills enabled for this session in Freeloop: " + strings.Join(names, ", "),
	}, "", "  ")
	files[".claude-plugin/plugin.json"] = manifest
	return config.WriteSessionPlugin(e.dataDir, sid, files)
}

// copySkillTree reads a skill's folder (dir in fs, abs on disk, inside the
// skill at root) into files
// under prefix, leaving out the store's bookkeeping and anything a symlink
// leads out of the skill's folder: the copy can reach a container that
// could not otherwise see it.
func copySkillTree(fs skill.FS, dir, abs, root, prefix string, files map[string][]byte, size *int) error {
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, de := range entries {
		if de.Name == config.ProvenanceFile || de.Name == ".previous" || de.Name == ".git" {
			continue
		}
		src, srcAbs, dst := path.Join(dir, de.Name), filepath.Join(abs, de.Name), prefix+"/"+de.Name
		if root != "" && !environment.Within(srcAbs, root) {
			log.Printf("engine: left %s out of a skill plugin: it leads outside the skill", srcAbs)
			continue
		}
		if de.IsDir {
			if err := copySkillTree(fs, src, srcAbs, root, dst, files, size); err != nil {
				return err
			}
			continue
		}
		if de.Size > maxPluginFile {
			return fmt.Errorf("%s is over %d MB", src, maxPluginFile>>20)
		}
		b, err := fs.ReadFile(src)
		if err != nil {
			return err
		}
		*size += len(b)
		files[dst] = b
	}
	return nil
}
