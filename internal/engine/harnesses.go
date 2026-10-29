package engine

import (
	"fmt"
	"slices"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
)

// External harnesses (HRN-005, HRN-006) are ACP agents described by presets:
// the built-in Claude Code preset and any presets/harnesses/*.yaml. Each
// preset also brings a profile of the same ID, so it can be chosen at once.

// HarnessInfo is a harness as the new-session form lists it.
type HarnessInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"` // builtin | acp
	Description string `json:"description"`
	Notes       string `json:"notes"`
	// UsesProvider is set for harnesses that run on one of the app's
	// providers; an external agent brings its own model access.
	UsesProvider bool `json:"usesProvider"`
}

func (e *Engine) loadPresets() {
	list, issues := config.LoadPresets(e.dataDir)
	e.mu.Lock()
	e.presets = list
	e.issues[issuePresets] = issues
	e.mu.Unlock()
}

func (e *Engine) preset(id string) (config.HarnessPreset, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.presets {
		if p.ID == id {
			return p, nil
		}
	}
	return config.HarnessPreset{}, fmt.Errorf("no harness %q; its preset may have been removed", id)
}

// ListHarnesses returns the built-in harness and every preset.
func (e *Engine) ListHarnesses() []HarnessInfo {
	out := []HarnessInfo{{ID: builtin.ID, Name: "Built-in", Kind: "builtin", Description: "Freeloop's own agent loop on your providers.", UsesProvider: true}}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.presets {
		out = append(out, HarnessInfo{ID: p.ID, Name: p.Name, Kind: "acp", Description: p.Description, Notes: p.Notes})
	}
	return out
}

// presetProfiles are the profiles the presets bring, except where a
// profile file of the same ID exists.
func (e *Engine) presetProfiles(files []config.Profile) []config.Profile {
	e.mu.Lock()
	presets := slices.Clone(e.presets)
	e.mu.Unlock()
	var out []config.Profile
	for _, p := range presets {
		if !slices.ContainsFunc(files, func(f config.Profile) bool { return f.ID == p.ID }) {
			out = append(out, config.PresetProfile(p))
		}
	}
	return out
}

// agentServers are the session's approved MCP servers with their secrets,
// for an external agent's session/new (MCP-004).
func (e *Engine) agentServers(prof config.Profile) []harness.MCPServerSpec {
	var out []harness.MCPServerSpec
	for _, s := range e.sessionServers(prof) {
		l, err := e.mcpLaunch(s)
		if err != nil {
			continue
		}
		spec := harness.MCPServerSpec{Name: s.ID, Transport: s.Transport, Command: s.Command, Args: s.Args, Env: map[string]string{}, URL: s.URL, Headers: map[string]string{}}
		for k, v := range s.Env {
			spec.Env[k] = v
		}
		for k, v := range l.Env {
			spec.Env[k] = v
		}
		for k, v := range s.Headers {
			spec.Headers[k] = v
		}
		for k, v := range l.Headers {
			spec.Headers[k] = v
		}
		out = append(out, spec)
	}
	return out
}
