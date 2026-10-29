package engine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// SET-001: hand edits to the config files reach the engine within 2 s, and
// a broken file is reported, not fatal; fixing it clears the report.
func TestHotReload(t *testing.T) {
	dir := t.TempDir()
	changes := make(chan ConfigChange, 16)
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: 20 * time.Millisecond,
		OnConfigChange: func(c ConfigChange) { changes <- c }})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatalf("%s: not reloaded within 2 s", what)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	issueIn := func(file string) bool { return hasIssue(e, file) }

	write("config.yaml", "version: 1\nui:\n  theme: light\n  fontSize: 16\n")
	wait("config.yaml", func() bool { return e.Config().UI.Theme == "light" })
	if c := <-changes; !slices.Contains(c.Files, "config.yaml") {
		t.Errorf("change = %+v", c)
	}

	write("config.yaml", "version: 1\nui: [not, a, map\n")
	wait("broken config.yaml", func() bool { return issueIn("config.yaml") })
	write("config.yaml", "version: 1\n")
	wait("fixed config.yaml", func() bool { return !issueIn("config.yaml") })

	write("providers.yaml", "version: 1\nproviders:\n  - id: local\n    name: Local\n    type: ollama\n    baseUrl: http://127.0.0.1:1\n")
	wait("providers.yaml", func() bool {
		return slices.ContainsFunc(e.ListProviders(), func(p ProviderInfo) bool { return p.ID == "local" })
	})

	write("rules.yaml", "version: 1\nrules:\n  - effect: maybe\n    tool: shell\n")
	wait("broken rules.yaml", func() bool { return issueIn("rules.yaml") })
	write("rules.yaml", "version: 1\nrules:\n  - effect: deny\n    tool: shell\n    command: git push *\n")
	wait("fixed rules.yaml", func() bool { return !issueIn("rules.yaml") && len(e.ListRules("")) == 1 })

	write("environments/ci.yaml", "name: CI\nkind: local\nenv:\n  CI: \"1\"\n")
	wait("environments", func() bool {
		return slices.ContainsFunc(e.ListEnvironments(), func(x EnvironmentInfo) bool { return x.ID == "ci" })
	})
}

func hasIssue(e *Engine, file string) bool {
	for _, i := range e.ConfigIssues() {
		if strings.HasSuffix(i.File, file) {
			return true
		}
	}
	return false
}
