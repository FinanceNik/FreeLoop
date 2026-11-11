package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStyle(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, StylesDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, StylesDir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadStyles(t *testing.T) {
	dir := t.TempDir()
	if styles, issues := LoadStyles(dir); len(styles) != 0 || len(issues) != 0 {
		t.Fatalf("no folder: %+v, %+v", styles, issues)
	}
	writeStyle(t, dir, "paper.json", `{"version":1,"name":"Paper","textFont":"Inter","codeFont":"Menlo","light":{"accent":"#7c3aed"},"dark":{"bg":"#101010"}}`)
	// Bad entries are left out and reported; the rest of the file holds.
	writeStyle(t, dir, "night.json", `{"name":"Night","textFont":"Inter; x","light":{"accent":"url(https://example.com/x)","nope":"#fff","text":"#222"}}`)
	writeStyle(t, dir, "broken.json", `{"name":`)
	writeStyle(t, dir, "no-name.json", `{"dark":{"text":"#eee"}}`)
	writeStyle(t, dir, "Bad Name.json", `{"name":"Bad"}`)
	writeStyle(t, dir, "notes.txt", `not a style`)

	styles, issues := LoadStyles(dir)
	var ids []string
	for _, s := range styles {
		ids = append(ids, s.ID)
	}
	// Sorted by name; a file without one is named after its ID.
	if strings.Join(ids, ",") != "night,no-name,paper" {
		t.Fatalf("ids = %v", ids)
	}
	night, noName, paper := styles[0], styles[1], styles[2]
	if paper.Name != "Paper" || paper.TextFont != "Inter" || paper.CodeFont != "Menlo" || paper.Light["accent"] != "#7c3aed" || paper.Dark["bg"] != "#101010" || paper.Version != 1 {
		t.Fatalf("paper = %+v", paper)
	}
	if night.TextFont != "" || len(night.Light) != 1 || night.Light["text"] != "#222" || night.Dark == nil {
		t.Fatalf("night = %+v", night)
	}
	if noName.Name != "no-name" || noName.Dark["text"] != "#eee" || noName.Light == nil {
		t.Fatalf("no-name = %+v", noName)
	}

	got := map[string]Issue{}
	for _, is := range issues {
		got[is.File+" "+is.Field] = is
	}
	if len(issues) != 5 {
		t.Fatalf("issues = %+v", issues)
	}
	for _, key := range []string{"styles/night.json light.accent", "styles/night.json light.nope", "styles/night.json textFont", "styles/Bad Name.json "} {
		if _, ok := got[filepath.FromSlash(key)]; !ok {
			t.Errorf("no issue %q in %+v", key, issues)
		}
	}
	if is := got[filepath.FromSlash("styles/broken.json ")]; !is.Unreadable {
		t.Errorf("broken file: %+v", issues)
	}
}

func TestSaveStyleRoundTripAndRefusal(t *testing.T) {
	dir := t.TempDir()
	in := Style{ID: "paper", Name: "Paper", TextFont: "Inter", Light: map[string]string{"accent": "#7c3aed"}, Dark: map[string]string{"bg": "rgb(1, 2, 3)"}}
	if err := SaveStyle(dir, in); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, StylesDir, "paper.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The ID is the file's name and is not written into it.
	if strings.Contains(string(b), `"id"`) || !strings.Contains(string(b), `"version": 1`) {
		t.Fatalf("file = %s", b)
	}
	styles, issues := LoadStyles(dir)
	if len(issues) != 0 || len(styles) != 1 || styles[0].Name != "Paper" || styles[0].TextFont != "Inter" || styles[0].Light["accent"] != "#7c3aed" || styles[0].Dark["bg"] != "rgb(1, 2, 3)" {
		t.Fatalf("round trip: %+v, %+v", styles, issues)
	}

	for name, bad := range map[string]Style{
		"no name":       {ID: "a", Name: "  "},
		"long name":     {ID: "a", Name: strings.Repeat("x", 65)},
		"bad id":        {ID: "../x", Name: "X"},
		"upper id":      {ID: "Paper", Name: "X"},
		"bad color":     {ID: "a", Name: "X", Light: map[string]string{"accent": "url(x)"}},
		"unknown token": {ID: "a", Name: "X", Dark: map[string]string{"nope": "#fff"}},
		"bad font":      {ID: "a", Name: "X", CodeFont: "Menlo}"},
	} {
		if err := SaveStyle(dir, bad); err == nil {
			t.Errorf("%s: saved", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, StylesDir, "a.json")); !os.IsNotExist(err) {
		t.Fatalf("a refused save wrote a file: %v", err)
	}

	if err := DeleteStyle(dir, "paper"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteStyle(dir, "paper"); err != nil {
		t.Fatalf("deleting a missing style: %v", err)
	}
	if err := DeleteStyle(dir, "../config"); err == nil {
		t.Fatal("a path was taken for an ID")
	}
	if styles, _ := LoadStyles(dir); len(styles) != 0 {
		t.Fatalf("after delete: %+v", styles)
	}
}

func TestStylePointerIsAnID(t *testing.T) {
	c := Default()
	c.UI.Style = "paper-2"
	if issues := c.Validate(); len(issues) != 0 || c.UI.Style != "paper-2" {
		t.Fatalf("a good ID: %+v, %q", issues, c.UI.Style)
	}
	c.UI.Style = "../colors"
	issues := c.Validate()
	if len(issues) != 1 || issues[0].Field != "ui.style" || c.UI.Style != "" {
		t.Fatalf("a bad ID: %+v, %q", issues, c.UI.Style)
	}
}

func TestStylesAreWatchedAndBackedUp(t *testing.T) {
	dir := t.TempDir()
	if err := SaveStyle(dir, Style{ID: "paper", Name: "Paper", Light: map[string]string{"accent": "#123456"}}); err != nil {
		t.Fatal(err)
	}
	writeStyle(t, dir, "notes.txt", "x")
	fp := Fingerprint(dir)
	if _, ok := fp[filepath.Join(StylesDir, "paper.json")]; !ok {
		t.Fatalf("the style is not watched: %v", fp)
	}
	if _, ok := fp[filepath.Join(StylesDir, "notes.txt")]; ok {
		t.Fatal("a file that is not a style is watched")
	}
	b, err := ExportBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	fresh := t.TempDir()
	if _, err := ImportBundle(fresh, b); err != nil {
		t.Fatal(err)
	}
	if styles, _ := LoadStyles(fresh); len(styles) != 1 || styles[0].Light["accent"] != "#123456" {
		t.Fatalf("restored styles: %+v", styles)
	}
	if !backedUp("styles/paper.json") || backedUp("styles/paper.yaml") || backedUp("styles/.hidden.json") || backedUp("styles/../config.json") {
		t.Fatal("backedUp takes the wrong files from styles/")
	}
}
