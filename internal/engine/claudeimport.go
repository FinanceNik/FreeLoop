package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

// Importing from Claude Code (MCP-002, SET-003, OQ-010) is a one-way copy:
// MCP servers from ~/.claude.json (global and per project) and permission
// rules from ~/.claude/settings.json are read, previewed with their
// destination, and written into mcp.yaml and rules.yaml. Nothing is ever
// written back to Claude Code's files.

// ImportItem is one thing the preview offers.
type ImportItem struct {
	// Key identifies the item when applying.
	Key string `json:"key"`
	// Kind is mcp or rule.
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Detail      string `json:"detail"`
	Destination string `json:"destination"`
	// Status is new, same (already there), changed (replaces the existing
	// server, which then needs approval again) or unsupported.
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
	// Secrets are the variables or headers moved to the Keychain.
	Secrets []string `json:"secrets"`
	Source  string   `json:"source"`

	server *SaveMCPServerRequest
	rules  []permission.Rule
}

// ClaudeImport is the preview.
type ClaudeImport struct {
	Items []ImportItem `json:"items"`
	// Sources lists the files read, and Missing the ones not found.
	Sources []string `json:"sources"`
	Missing []string `json:"missing"`
}

type claudeServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// secretName matches variables and headers whose values are secrets.
var secretName = regexp.MustCompile(`(?i)(token|secret|password|passwd|api[_-]?key|access[_-]?key|private[_-]?key|credential|auth|bearer|\bpat\b|_pat$)`)

var notID = regexp.MustCompile(`[^a-z0-9]+`)

// serverID turns a Claude Code server name into an ID.
func serverID(name string) string {
	id := strings.Trim(notID.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if id == "" {
		id = "server"
	}
	return id
}

func (e *Engine) readHomeJSON(rel string, into any) (string, bool, error) {
	env, err := readOnlyEnv(e.home)
	if err != nil {
		return "", false, err
	}
	path := filepath.Join(e.home, rel)
	b, err := env.FS().ReadFile(rel)
	if err != nil {
		return path, false, nil
	}
	if err := json.Unmarshal(b, into); err != nil {
		return path, true, fmt.Errorf("%s: %w", path, err)
	}
	return path, true, nil
}

// PreviewClaudeImport reads Claude Code's MCP servers and permission rules
// and says what importing each would do.
func (e *Engine) PreviewClaudeImport(ctx context.Context) (ClaudeImport, error) {
	out := ClaudeImport{Items: []ImportItem{}, Sources: []string{}, Missing: []string{}}

	var cj struct {
		MCPServers map[string]claudeServer `json:"mcpServers"`
		Projects   map[string]struct {
			MCPServers map[string]claudeServer `json:"mcpServers"`
		} `json:"projects"`
	}
	path, ok, err := e.readHomeJSON(".claude.json", &cj)
	if err != nil {
		return out, err
	}
	if ok {
		out.Sources = append(out.Sources, path)
		for _, name := range slices.Sorted(maps.Keys(cj.MCPServers)) {
			out.Items = append(out.Items, e.mcpItem(name, cj.MCPServers[name], path))
		}
		for _, proj := range slices.Sorted(maps.Keys(cj.Projects)) {
			servers := cj.Projects[proj].MCPServers
			for _, name := range slices.Sorted(maps.Keys(servers)) {
				it := e.mcpItem(name, servers[name], path+" (project "+proj+")")
				it.Note = strings.TrimSpace(it.Note + " Claude Code used it in " + proj + " only; here it is a server like any other, off until a profile or you turn it on.")
				if it.server != nil {
					it.server.Server.Enabled = false
				}
				out.Items = append(out.Items, it)
			}
		}
	} else {
		out.Missing = append(out.Missing, path)
	}

	var settings struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
			Ask   []string `json:"ask"`
		} `json:"permissions"`
	}
	path, ok, err = e.readHomeJSON(filepath.Join(".claude", "settings.json"), &settings)
	if err != nil {
		return out, err
	}
	if ok {
		out.Sources = append(out.Sources, path)
		existing, _ := e.rules.Rules("")
		for _, list := range []struct {
			effect permission.Effect
			rules  []string
		}{{permission.Allow, settings.Permissions.Allow}, {permission.Deny, settings.Permissions.Deny}} {
			for _, r := range list.rules {
				out.Items = append(out.Items, ruleItem(list.effect, r, path, existing, e.home))
			}
		}
		for _, r := range settings.Permissions.Ask {
			out.Items = append(out.Items, ImportItem{Key: "ask:" + r, Kind: "rule", Label: "ask " + r, Detail: r, Destination: "none", Status: "unsupported",
				Note: "Freeloop asks for anything no rule allows, so ask rules need no import.", Source: path, Secrets: []string{}})
		}
	} else {
		out.Missing = append(out.Missing, path)
	}
	return out, nil
}

// mcpItem converts one Claude Code server, moving secrets to the Keychain.
func (e *Engine) mcpItem(name string, cs claudeServer, source string) ImportItem {
	id := serverID(name)
	it := ImportItem{Key: "mcp:" + source + ":" + name, Kind: "mcp", Label: name, Destination: "mcp.yaml as " + id, Status: "new", Source: source, Secrets: []string{}}
	s := config.MCPServer{ID: id, Name: name, Enabled: true, Source: source, Env: map[string]string{}, Headers: map[string]string{}}
	req := &SaveMCPServerRequest{Secrets: map[string]string{}}
	switch strings.ToLower(cs.Type) {
	case "", "stdio":
		s.Transport, s.Command, s.Args = "stdio", cs.Command, cs.Args
		it.Detail = strings.TrimSpace(cs.Command + " " + strings.Join(cs.Args, " "))
		for k, v := range cs.Env {
			if secretName.MatchString(k) && v != "" {
				req.Secrets[k] = v
				it.Secrets = append(it.Secrets, k)
			} else {
				s.Env[k] = v
			}
		}
	case "http", "streamable-http":
		s.Transport, s.URL = "http", cs.URL
		it.Detail = cs.URL
		for k, v := range cs.Headers {
			if secretName.MatchString(k) && v != "" {
				req.Secrets[k] = v
				it.Secrets = append(it.Secrets, k)
			} else {
				s.Headers[k] = v
			}
		}
	default:
		it.Status, it.Detail, it.Destination = "unsupported", cs.URL, "none"
		it.Note = fmt.Sprintf("the %s transport is not supported; Freeloop speaks stdio and Streamable HTTP.", cs.Type)
		return it
	}
	if strings.Contains(it.Detail, "${") || slices.ContainsFunc(slices.Collect(maps.Values(cs.Env)), func(v string) bool { return strings.Contains(v, "${") }) {
		it.Note = "It uses ${VAR} expansion, which Freeloop does not do; check the values after importing."
	}
	slices.Sort(it.Secrets)
	if err := s.Validate(); err != nil {
		it.Status, it.Note, it.Destination = "unsupported", err.Error(), "none"
		return it
	}
	if cur, err := e.mcpServer(id); err == nil {
		same := cur.Transport == s.Transport && cur.Command == s.Command && slices.Equal(cur.Args, s.Args) && cur.URL == s.URL &&
			maps.Equal(cur.Env, s.Env) && maps.Equal(cur.Headers, s.Headers) && slices.Equal(slices.Sorted(maps.Keys(cur.Secrets)), it.Secrets)
		if same {
			it.Status = "same"
		} else {
			it.Status = "changed"
			it.Note = strings.TrimSpace(it.Note + " Replaces the server " + id + "; it needs approval again.")
		}
	}
	req.Server = s
	it.server = req
	return it
}

// ruleItem converts one Claude Code permission rule.
func ruleItem(effect permission.Effect, raw, source string, existing []permission.Rule, home string) ImportItem {
	it := ImportItem{Key: string(effect) + ":" + raw, Kind: "rule", Label: string(effect) + " " + raw, Detail: raw, Destination: "rules.yaml", Status: "new", Source: source, Secrets: []string{}}
	rules, note := convertRule(effect, raw, home)
	if rules == nil {
		it.Status, it.Note, it.Destination = "unsupported", note, "none"
		return it
	}
	it.rules, it.Note = rules, note
	var shown []string
	allThere := true
	for _, r := range rules {
		shown = append(shown, r.String())
		if !slices.ContainsFunc(existing, func(x permission.Rule) bool { return sameRule(x, r) }) {
			allThere = false
		}
	}
	it.Destination = "rules.yaml: " + strings.Join(shown, ", ")
	if allThere {
		it.Status = "same"
	}
	return it
}

var toolPattern = regexp.MustCompile(`^([A-Za-z]+)(?:\((.*)\))?$`)

// convertRule maps Claude Code's rule syntax onto Freeloop's (OQ-010), or
// returns nil and why it cannot.
func convertRule(effect permission.Effect, raw, home string) ([]permission.Rule, string) {
	if strings.HasPrefix(raw, "mcp__") {
		return []permission.Rule{{Effect: effect, Tool: raw}}, ""
	}
	m := toolPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return nil, "not a rule Freeloop understands"
	}
	tool, arg := m[1], m[2]
	mk := func(tools []string, path string) []permission.Rule {
		var out []permission.Rule
		for _, t := range tools {
			out = append(out, permission.Rule{Effect: effect, Tool: t, Path: path})
		}
		return out
	}
	path := func(p string) (string, string) {
		switch {
		case strings.HasPrefix(p, "//"):
			return p[1:], ""
		case strings.HasPrefix(p, "~/"):
			return filepath.Join(home, p[2:]), ""
		case strings.HasPrefix(p, "/"):
			return strings.TrimPrefix(p, "/"), "Claude Code reads a leading / as the settings file's folder; Freeloop applies it to each project root."
		}
		return p, ""
	}
	switch tool {
	case "Bash":
		if arg == "" {
			return []permission.Rule{{Effect: effect, Tool: "shell"}}, ""
		}
		cmd := arg
		if p, ok := strings.CutSuffix(arg, ":*"); ok {
			cmd = p + " *"
		}
		return []permission.Rule{{Effect: effect, Tool: "shell", Command: cmd}}, ""
	case "Read":
		p, note := path(arg)
		return mk([]string{"read_file", "list_dir", "glob", "grep"}, p), note
	case "Edit", "MultiEdit":
		p, note := path(arg)
		return mk([]string{"edit_file", "write_file"}, p), note
	case "Write":
		p, note := path(arg)
		return mk([]string{"write_file"}, p), note
	case "Grep", "Glob", "LS":
		p, note := path(arg)
		return mk([]string{map[string]string{"Grep": "grep", "Glob": "glob", "LS": "list_dir"}[tool]}, p), note
	}
	return nil, tool + " has no counterpart in the built-in harness."
}

// ApplyClaudeImport imports the items with the given keys from a fresh
// preview and returns how many it wrote.
func (e *Engine) ApplyClaudeImport(ctx context.Context, keys []string) (int, error) {
	p, err := e.PreviewClaudeImport(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, it := range p.Items {
		if !slices.Contains(keys, it.Key) || it.Status == "same" || it.Status == "unsupported" {
			continue
		}
		switch {
		case it.server != nil:
			if _, err := e.SaveMCPServer(*it.server); err != nil {
				return n, fmt.Errorf("%s: %w", it.Label, err)
			}
		case it.rules != nil:
			for _, r := range it.rules {
				r.Scope = permission.ScopeGlobal
				if err := e.rules.AddRule("", r); err != nil {
					return n, fmt.Errorf("%s: %w", it.Label, err)
				}
			}
		}
		n++
	}
	return n, nil
}
