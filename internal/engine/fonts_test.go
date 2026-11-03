package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

func TestFontsFollowTheFile(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	c := e.Config()
	c.UI.TextFont, c.UI.CodeFont = "Avenir Next", "Menlo"
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	c.UI.CodeFont = "x; } * { background: url(y) }"
	if _, err := e.SaveSettings(c); err == nil {
		t.Error("a bad font was saved")
	}
	if got := e.Config().UI; got.TextFont != "Avenir Next" || got.CodeFont != "Menlo" {
		t.Fatalf("after the refused save: %+v", got)
	}

	file := filepath.Join(dir, config.FileName)
	if err := os.WriteFile(file, []byte("version: 1\nui:\n  theme: system\n  textFont: Georgia\n  codeFont: 'a\"; } * { color: red }'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.reload([]string{config.FileName})
	if got := e.Config().UI; got.TextFont != "Georgia" || got.CodeFont != "" {
		t.Errorf("after the edit: %+v", got)
	}
	if !hasIssue(e, config.FileName) {
		t.Errorf("the bad font is not reported: %+v", e.ConfigIssues())
	}
}
