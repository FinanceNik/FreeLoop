package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness/templates"
)

// Custom harnesses (HRN-011, EXT-003): "New harness" writes an ACP agent
// from the Python or Go template into a folder the user picks and registers
// it as a harness preset that runs it. The preset watches the folder: when
// its files change, idle sessions on that harness stop their agent, which
// starts again, with the new code, on their next message.

// ScaffoldRequest asks for a new harness.
type ScaffoldRequest struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	// Folder is where the agent's own folder is created.
	Folder string `json:"folder"`
}

// ScaffoldResult is the harness made: its preset and the file to edit.
type ScaffoldResult struct {
	Preset config.HarnessPreset `json:"preset"`
	Dir    string               `json:"dir"`
	Entry  string               `json:"entry"`
}

// ScaffoldHarness writes an agent from a template and registers it.
func (e *Engine) ScaffoldHarness(ctx context.Context, req ScaffoldRequest) (ScaffoldResult, error) {
	name := strings.TrimSpace(req.Name)
	id := slug(name)
	switch {
	case name == "" || id == "":
		return ScaffoldResult{}, errors.New("give the harness a name")
	case !slices.Contains(templates.Languages, req.Language):
		return ScaffoldResult{}, fmt.Errorf("there is no %q template; choose python or go", req.Language)
	case !filepath.IsAbs(req.Folder) || !environment.IsDir(req.Folder):
		return ScaffoldResult{}, errors.New("choose an existing folder for the agent")
	}
	if _, err := e.preset(id); err == nil {
		return ScaffoldResult{}, fmt.Errorf("a harness %s exists", id)
	}
	env, err := e.localEnv(req.Folder, nil)
	if err != nil {
		return ScaffoldResult{}, err
	}
	dir := filepath.Join(env.Root(), id)
	if _, err := env.FS().Stat(dir); err == nil {
		return ScaffoldResult{}, fmt.Errorf("%s exists; choose another name or folder", dir)
	}
	files, err := templates.Render(req.Language, name, "example.com/"+id)
	if err != nil {
		return ScaffoldResult{}, err
	}
	for rel, b := range files {
		if err := env.FS().WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), b); err != nil {
			return ScaffoldResult{}, err
		}
	}
	cmd, args := templates.Command(req.Language, dir)
	entry := templates.Entry(req.Language)
	p := config.HarnessPreset{
		ID: id, Name: name, Command: cmd, Args: args, Watch: dir,
		Description: fmt.Sprintf("Your ACP agent from the %s template.", req.Language),
		Notes:       fmt.Sprintf("Edit %s in %s; sessions start the agent again when its files change.", entry, dir),
	}
	if err := config.SavePreset(e.dataDir, p); err != nil {
		return ScaffoldResult{}, err
	}
	e.loadPresets()
	e.loadProfiles()
	return ScaffoldResult{Preset: p, Dir: dir, Entry: filepath.Join(dir, entry)}, nil
}

// agentWatch remembers the watched folders' fingerprints.
type agentWatch struct {
	seen map[string]string // preset ID -> fingerprint
}

// watchAgents restarts idle sessions' agents whose preset folder changed;
// the config watch calls it on every poll.
func (e *Engine) watchAgents() {
	e.mu.Lock()
	presets := slices.Clone(e.presets)
	if e.agentWatch.seen == nil {
		e.agentWatch.seen = map[string]string{}
	}
	e.mu.Unlock()
	for _, p := range presets {
		if p.Watch == "" {
			continue
		}
		fp := config.DirFingerprint(p.Watch)
		e.mu.Lock()
		before, known := e.agentWatch.seen[p.ID]
		e.agentWatch.seen[p.ID] = fp
		running := map[string]*runtime{}
		if known && before != fp {
			for sid, r := range e.runtimes {
				running[sid] = r
			}
		}
		e.mu.Unlock()
		for sid, r := range running {
			ses, err := e.store.GetSession(context.Background(), sid)
			if err != nil || ses.HarnessID != p.ID {
				continue
			}
			r.mu.Lock()
			busy := r.busy
			r.mu.Unlock()
			if busy {
				continue
			}
			e.stopRuntime(sid)
			e.emit(sid, event.Notice, event.NoticeData{Message: fmt.Sprintf("The agent's files in %s changed; it starts again with your next message.", p.Watch)})
		}
	}
}

// HarnessEntry returns the file to edit of a custom harness, for the
// editor; "" for a preset without a watched folder.
func (e *Engine) HarnessEntry(id string) string {
	p, err := e.preset(id)
	if err != nil || p.Watch == "" {
		return ""
	}
	for _, lang := range templates.Languages {
		f := filepath.Join(p.Watch, templates.Entry(lang))
		if env, err := e.localEnv(p.Watch, nil); err == nil {
			if _, err := env.FS().Stat(f); err == nil {
				return f
			}
		}
	}
	return ""
}
