package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// MCP-002 and SET-003: Claude Code's MCP servers and permission rules are
// previewed with their destination, secrets in env and headers move to the
// Keychain, rules convert or say why not, importing twice changes nothing,
// and Claude Code's files are untouched.
func TestClaudeImport(t *testing.T) {
	ctx := context.Background()
	home, dir := t.TempDir(), t.TempDir()
	store := secrets.NewMemory()
	e, err := Open(Options{DataDir: dir, Secrets: store, Home: home, Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	claudeJSON := `{
	  "mcpServers": {
	    "GitHub": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"], "env": {"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_secretvalue123", "LOG_LEVEL": "info"}},
	    "linear": {"type": "http", "url": "https://mcp.linear.app/mcp", "headers": {"Authorization": "Bearer lin_secret_456"}},
	    "old": {"type": "sse", "url": "https://example.com/sse"}
	  },
	  "projects": {"/work/app": {"mcpServers": {"db": {"command": "/usr/local/bin/db-mcp"}}}}
	}`
	settings := `{"permissions": {
	  "allow": ["Bash(npm run test:*)", "Bash(git status)", "Read(~/notes/**)", "Edit(src/**)", "mcp__github__get_issue", "WebFetch(domain:example.com)"],
	  "deny": ["Bash(git push:*)"],
	  "ask": ["Bash(rm:*)"]
	}}`
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(claudeJSON), 0o600)
	os.MkdirAll(filepath.Join(home, ".claude"), 0o700)
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o600)

	p, err := e.PreviewClaudeImport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Sources) != 2 || len(p.Missing) != 0 {
		t.Fatalf("sources = %v, missing = %v", p.Sources, p.Missing)
	}
	by := map[string]ImportItem{}
	for _, it := range p.Items {
		by[it.Label] = it
	}
	gh := by["GitHub"]
	if gh.Status != "new" || gh.Destination != "mcp.yaml as github" || !slices.Equal(gh.Secrets, []string{"GITHUB_PERSONAL_ACCESS_TOKEN"}) {
		t.Errorf("github = %+v", gh)
	}
	if by["linear"].Status != "new" || !slices.Equal(by["linear"].Secrets, []string{"Authorization"}) {
		t.Errorf("linear = %+v", by["linear"])
	}
	if by["old"].Status != "unsupported" || by["db"].Status != "new" {
		t.Errorf("old = %+v, db = %+v", by["old"], by["db"])
	}
	for label, dest := range map[string]string{
		"allow Bash(npm run test:*)": "allow shell(npm run test *)",
		"allow Bash(git status)":     "allow shell(git status)",
		"allow Read(~/notes/**)":     "allow read_file(" + filepath.Join(home, "notes/**") + ")",
		"allow Edit(src/**)":         "allow edit_file(src/**), allow write_file(src/**)",
		"deny Bash(git push:*)":      "deny shell(git push *)",
	} {
		if it := by[label]; it.Status != "new" || !strings.Contains(it.Destination, dest) {
			t.Errorf("%s = %+v, want destination with %q", label, it, dest)
		}
	}
	if by["allow WebFetch(domain:example.com)"].Status != "unsupported" || by["ask Bash(rm:*)"].Status != "unsupported" {
		t.Error("unsupported rules are not reported")
	}

	var keys []string
	for _, it := range p.Items {
		keys = append(keys, it.Key)
	}
	n, err := e.ApplyClaudeImport(ctx, keys)
	if err != nil || n != 9 { // 3 servers, 6 rules
		t.Fatalf("applied %d, %v", n, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, config.MCPFileName))
	if strings.Contains(string(b), "ghp_secretvalue123") || strings.Contains(string(b), "lin_secret_456") {
		t.Fatal("a secret is in mcp.yaml")
	}
	if v, err := store.Get(mcpSecretRef("github", "GITHUB_PERSONAL_ACCESS_TOKEN")); err != nil || v != "ghp_secretvalue123" {
		t.Errorf("keychain = %q, %v", v, err)
	}
	gs, _ := e.mcpServer("github")
	if gs.Env["LOG_LEVEL"] != "info" || gs.Approved != "" || !gs.Enabled || !strings.HasSuffix(gs.Source, ".claude.json") {
		t.Errorf("github server = %+v", gs)
	}
	if db, _ := e.mcpServer("db"); db.Enabled {
		t.Error("a project-only server is on for everyone")
	}
	if !slices.ContainsFunc(e.ListRules(""), func(r permission.Rule) bool { return r.Effect == permission.Deny && r.Command == "git push *" }) {
		t.Error("the deny rule was not imported")
	}

	// Again: nothing new.
	p, _ = e.PreviewClaudeImport(ctx)
	for _, it := range p.Items {
		if it.Status == "new" || it.Status == "changed" {
			t.Errorf("after import %s is %s", it.Label, it.Status)
		}
	}
	if after, _ := os.ReadFile(filepath.Join(home, ".claude.json")); string(after) != claudeJSON {
		t.Error("~/.claude.json changed")
	}
}
