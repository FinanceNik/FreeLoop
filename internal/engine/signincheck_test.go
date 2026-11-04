package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// OQ-018: Gemini CLI uses the login its own settings choose, whoever starts it, so a
// Google login there is told to the user. Nothing else is, and nothing else is read.
func TestSignInWarning(t *testing.T) {
	home := t.TempDir()
	e, err := Open(Options{DataDir: t.TempDir(), Secrets: secrets.NewMemory(), Home: home, Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	preset := func(id string) config.HarnessPreset {
		p, err := e.preset(id)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	gemini := preset("gemini-cli")
	settings := filepath.Join(home, ".gemini", "settings.json")
	if got := e.signInWarning(gemini); got != "" {
		t.Errorf("with no settings: %q", got)
	}
	os.MkdirAll(filepath.Dir(settings), 0o755)
	for _, c := range []struct {
		file string
		warn bool
	}{
		{`{"security":{"auth":{"selectedType":"oauth-personal"}}}`, true},
		{`{"selectedAuthType":"oauth-personal"}`, true},
		{`{"security":{"auth":{"selectedType":"gemini-api-key"}}}`, false},
		{`{"security":{"auth":"oauth-personal"}}`, false},
		{`{"security":null}`, false},
		{`{}`, false},
		{`not json`, false},
	} {
		if err := os.WriteFile(settings, []byte(c.file), 0o644); err != nil {
			t.Fatal(err)
		}
		got := e.signInWarning(gemini)
		if c.warn != (got != "") {
			t.Errorf("%s: %q", c.file, got)
		}
		if c.warn && !strings.Contains(got, "suspending an account") {
			t.Errorf("%s: says %q", c.file, got)
		}
	}
	os.WriteFile(settings, []byte(`{"selectedAuthType":"oauth-personal"}`), 0o644)
	for _, id := range []string{config.ClaudeCodePresetID, "codex"} {
		if got := e.signInWarning(preset(id)); got != "" {
			t.Errorf("%s warned: %q", id, got)
		}
	}
}
