package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestColorValues(t *testing.T) {
	for _, v := range []string{"#fff", "#FFFF", "#2563eb", "#2563eb80", "rgb(1, 2, 3)", "rgba(37, 99, 235, 0.1)", "rgb(1 2 3 / 50%)"} {
		if !ValidColor(v) {
			t.Errorf("%q should pass", v)
		}
	}
	for _, v := range []string{"", "red", "#12", "#12345", "#gggggg", "url(https://example.com/x)", "#fff;background:url(x)", "rgb(1,2,3);}", "var(--text)", "rgb(url(x))"} {
		if ValidColor(v) {
			t.Errorf("%q should not pass", v)
		}
	}
}

func TestLoadColorsDropsAndReportsBadEntries(t *testing.T) {
	dir := t.TempDir()
	file := `{"version":1,"light":{"accent":"#123456","nope":"#fff"},"dark":{"bg":"url(https://example.com/x)","text":"#eee"}}`
	if err := os.WriteFile(filepath.Join(dir, ColorsFileName), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	c, issues := LoadColors(dir)
	if c.Light["accent"] != "#123456" || c.Dark["text"] != "#eee" || len(c.Light) != 1 || len(c.Dark) != 1 {
		t.Fatalf("colors = %+v", c)
	}
	if len(issues) != 2 || issues[0].Field != "dark.bg" || issues[1].Field != "light.nope" || Unreadable(issues) {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestLoadColorsMissingAndBroken(t *testing.T) {
	dir := t.TempDir()
	c, issues := LoadColors(dir)
	if len(issues) != 0 || c.Light == nil || c.Dark == nil || len(c.Light)+len(c.Dark) != 0 {
		t.Fatalf("missing file: %+v, %+v", c, issues)
	}
	if err := os.WriteFile(filepath.Join(dir, ColorsFileName), []byte(`{"light":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, issues := LoadColors(dir); !Unreadable(issues) {
		t.Fatalf("broken file: %+v", issues)
	}
}

func TestSaveColorsRoundTripAndRefusal(t *testing.T) {
	dir := t.TempDir()
	in := Colors{Light: map[string]string{"accent": "#123456"}, Dark: map[string]string{"bg": "rgb(1, 2, 3)"}}
	if err := SaveColors(dir, in); err != nil {
		t.Fatal(err)
	}
	c, issues := LoadColors(dir)
	if len(issues) != 0 || c.Version != 1 || c.Light["accent"] != "#123456" || c.Dark["bg"] != "rgb(1, 2, 3)" {
		t.Fatalf("round trip: %+v, %+v", c, issues)
	}
	if err := SaveColors(dir, Colors{Light: map[string]string{"accent": "url(x)"}}); err == nil {
		t.Fatal("a bad value was saved")
	}
	if err := SaveColors(dir, Colors{Dark: map[string]string{"nope": "#fff"}}); err == nil {
		t.Fatal("an unknown token was saved")
	}
	if c, _ := LoadColors(dir); c.Light["accent"] != "#123456" {
		t.Fatalf("a refused save changed the file: %+v", c)
	}
}

// The names must stay in step with the tokens the style sheet defines.
func TestColorTokensExistInTheStyleSheet(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range ColorTokens {
		if !strings.Contains(string(css), "--"+name+":") {
			t.Errorf("app.css defines no --%s", name)
		}
	}
}

func TestColorsAreWatchedAndBackedUp(t *testing.T) {
	dir := t.TempDir()
	if err := SaveColors(dir, Colors{Light: map[string]string{"accent": "#123456"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := Fingerprint(dir)[ColorsFileName]; !ok {
		t.Fatal("the colors file is not watched")
	}
	b, err := ExportBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	fresh := t.TempDir()
	if _, err := ImportBundle(fresh, b); err != nil {
		t.Fatal(err)
	}
	if c, _ := LoadColors(fresh); c.Light["accent"] != "#123456" {
		t.Fatalf("restored colors: %+v", c)
	}
}
