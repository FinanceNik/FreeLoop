package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
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
	// Missing says why the agent cannot start here, such as its command
	// not being on the login PATH (Node.js for npx); empty when it can.
	Missing string `json:"missing,omitempty"`
	// Custom marks a harness made from a template, whose source the user
	// edits (HRN-011).
	Custom bool `json:"custom"`
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
	presets := slices.Clone(e.presets)
	e.mu.Unlock()
	for _, p := range presets {
		missing := e.presetBlocked(p)
		if missing == "" {
			missing = e.missingCommand(p.Command)
		}
		out = append(out, HarnessInfo{ID: p.ID, Name: p.Name, Kind: "acp", Description: p.Description, Notes: p.Notes, Missing: missing, Custom: p.Watch != ""})
	}
	return out
}

// commandCache remembers for a minute whether commands exist, since
// asking the login shell takes a moment.
type commandCache struct {
	mu   sync.Mutex
	seen map[string]commandSeen
}

type commandSeen struct {
	missing string
	at      time.Time
}

// missingCommand checks that cmd starts from the user's login shell, as an
// agent does, and says what to install when it does not.
func (e *Engine) missingCommand(cmd string) string {
	e.commands.mu.Lock()
	if c, ok := e.commands.seen[cmd]; ok && time.Since(c.at) < time.Minute {
		e.commands.mu.Unlock()
		return c.missing
	}
	e.commands.mu.Unlock()
	missing := ""
	env, err := e.localEnv(e.home, nil)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		res, err := env.Exec(ctx, environment.ExecSpec{Command: "command -v " + shellQuote(cmd), Timeout: 10 * time.Second})
		cancel()
		if err != nil || res.ExitCode != 0 {
			missing = fmt.Sprintf("%s is not on your login PATH", cmd)
			switch filepath.Base(cmd) {
			case "npx", "node", "npm":
				missing += "; install Node.js (for example with Homebrew: brew install node)"
			case "uvx", "uv":
				missing += "; install uv (brew install uv)"
			}
		}
	}
	e.commands.mu.Lock()
	if e.commands.seen == nil {
		e.commands.seen = map[string]commandSeen{}
	}
	e.commands.seen[cmd] = commandSeen{missing: missing, at: time.Now()}
	e.commands.mu.Unlock()
	return missing
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
