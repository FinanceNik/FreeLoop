package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

func TestStylesFollowTheFolder(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if s := e.Styles(); len(s) != 0 {
		t.Fatalf("a new data folder has styles: %+v", s)
	}

	// Saved as new: the ID comes from the name and does not take another's.
	paper, err := e.SaveStyle(config.Style{Name: "Paper & Ink", TextFont: "Inter", Light: map[string]string{"accent": "#7c3aed"}})
	if err != nil {
		t.Fatal(err)
	}
	if paper.ID != "paper-ink" || paper.Name != "Paper & Ink" || paper.Light["accent"] != "#7c3aed" {
		t.Fatalf("saved = %+v", paper)
	}
	again, err := e.SaveStyle(config.Style{Name: "paper ink"})
	if err != nil || again.ID != "paper-ink-2" {
		t.Fatalf("second = %+v, %v", again, err)
	}
	odd, err := e.SaveStyle(config.Style{Name: "★★★"})
	if err != nil || odd.ID != "style" {
		t.Fatalf("a name without letters = %+v, %v", odd, err)
	}
	long, err := e.SaveStyle(config.Style{Name: strings.Repeat("ab ", 21)})
	if err != nil || len(long.ID) > 63 || strings.HasSuffix(long.ID, "-") {
		t.Fatalf("a long name = %+v, %v", long, err)
	}
	if _, err := e.SaveStyle(config.Style{Name: "Bad", Dark: map[string]string{"bg": "url(x)"}}); err == nil {
		t.Fatal("a value that is not a color was saved")
	}

	// Updated in place.
	paper.Dark = map[string]string{"bg": "#101010"}
	if _, err := e.SaveStyle(paper); err != nil {
		t.Fatal(err)
	}
	if s := e.style("paper-ink"); s.Dark["bg"] != "#101010" || s.Name != "Paper & Ink" {
		t.Fatalf("updated = %+v", s)
	}

	// A file that cannot be parsed is reported, left out and not overwritten,
	// and a new style does not take its name.
	broken := filepath.Join(dir, config.StylesDir, "night.json")
	if err := os.WriteFile(broken, []byte(`{"name":`), 0o600); err != nil {
		t.Fatal(err)
	}
	e.reload([]string{config.StyleFile("night")})
	if !hasIssue(e, "night.json") || e.style("night").ID != "" {
		t.Fatalf("broken file: %+v, %+v", e.Styles(), e.ConfigIssues())
	}
	if _, err := e.SaveStyle(config.Style{ID: "night", Name: "Night"}); err == nil {
		t.Fatal("an unreadable style was overwritten")
	}
	if s, err := e.SaveStyle(config.Style{Name: "Night"}); err != nil || s.ID != "night-2" {
		t.Fatalf("new beside a broken file = %+v, %v", s, err)
	}

	// By hand: a fixed file is picked up and its bad entry reported.
	if err := os.WriteFile(broken, []byte(`{"name":"Night","dark":{"bg":"#000","text":"red"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	e.reload([]string{config.StyleFile("night")})
	if s := e.style("night"); s.Name != "Night" || s.Dark["bg"] != "#000" || s.Dark["text"] != "" {
		t.Fatalf("hand-edited = %+v", s)
	}
	if !hasIssue(e, "night.json") {
		t.Fatal("the bad entry is not reported")
	}

	if err := e.DeleteStyle("paper-ink"); err != nil {
		t.Fatal(err)
	}
	if e.style("paper-ink").ID != "" {
		t.Fatal("a deleted style is still listed")
	}
	folder, err := e.StylesFolder()
	if err != nil || folder != filepath.Join(dir, config.StylesDir) {
		t.Fatalf("folder = %q, %v", folder, err)
	}
}

func TestStylesFolderIsMadeWhenMissing(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	folder, err := e.StylesFolder()
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(folder); err != nil || !fi.IsDir() {
		t.Fatalf("folder %q: %v", folder, err)
	}
}
