package engine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

func TestColorsFollowTheFile(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	file := filepath.Join(dir, config.ColorsFileName)
	if c := e.Colors(); len(c.Light)+len(c.Dark) != 0 {
		t.Fatalf("a new data folder has colors: %+v", c)
	}

	// From the editor.
	if _, err := e.SaveColors(config.Colors{Dark: map[string]string{"accent": "#123456"}}); err != nil {
		t.Fatal(err)
	}
	if e.Colors().Dark["accent"] != "#123456" {
		t.Fatalf("saved colors: %+v", e.Colors())
	}
	if _, err := e.SaveColors(config.Colors{Dark: map[string]string{"accent": "url(x)"}}); err == nil {
		t.Fatal("a value that is not a color was saved")
	}

	// By hand: the good entries apply and the bad one is reported.
	os.WriteFile(file, []byte(`{"dark":{"accent":"#654321","bg":"red"}}`), 0o600)
	e.reload([]string{config.ColorsFileName})
	if e.Colors().Dark["accent"] != "#654321" || e.Colors().Dark["bg"] != "" {
		t.Fatalf("edited colors: %+v", e.Colors())
	}
	if !slices.ContainsFunc(e.ConfigIssues(), func(i config.Issue) bool { return i.Field == "dark.bg" }) {
		t.Fatalf("issues: %+v", e.ConfigIssues())
	}

	// A half-saved edit keeps the colors in force and is not saved over.
	broken := []byte(`{"dark":`)
	os.WriteFile(file, broken, 0o600)
	e.reload([]string{config.ColorsFileName})
	if e.Colors().Dark["accent"] != "#654321" {
		t.Fatalf("colors after a broken edit: %+v", e.Colors())
	}
	if _, err := e.SaveColors(config.Colors{}); err == nil {
		t.Fatal("saved over a file that cannot be read")
	}
	if b, _ := os.ReadFile(file); string(b) != string(broken) {
		t.Fatalf("the file changed: %s", b)
	}

	// Fixing the file clears the problem.
	os.WriteFile(file, []byte(`{"light":{"text":"#111"}}`), 0o600)
	e.reload([]string{config.ColorsFileName})
	if e.Colors().Light["text"] != "#111" || len(e.ConfigIssues()) != 0 {
		t.Fatalf("after the fix: %+v, %+v", e.Colors(), e.ConfigIssues())
	}
}

func TestColorsFileIsCreatedForOpening(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	path, err := e.ColorsFile()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"version\": 1,\n  \"light\": {},\n  \"dark\": {}\n}\n"; string(b) != want {
		t.Fatalf("new file:\n%s", b)
	}
}
