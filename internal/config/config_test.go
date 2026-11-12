package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// OQ-053: the time an idle session keeps its agent is a setting a file can set.
func TestIdleStopSetting(t *testing.T) {
	for _, c := range []struct {
		file  string
		after time.Duration
		on    bool
	}{
		{"version: 1\n", 30 * time.Minute, true},
		{"version: 1\nsessions:\n  idleStop: 2h\n", 2 * time.Hour, true},
		{"version: 1\nsessions:\n  idleStop: never\n", 0, false},
	} {
		cfg, issues, err := Load(write(t, c.file))
		if err != nil || len(issues) != 0 {
			t.Fatalf("%q: issues = %v, err = %v", c.file, issues, err)
		}
		if d, on := cfg.Sessions.IdleStopAfter(); d != c.after || on != c.on {
			t.Errorf("%q: IdleStopAfter = %v, %v", c.file, d, on)
		}
	}
	cfg, issues, _ := Load(write(t, "version: 1\nsessions:\n  idleStop: soon\n"))
	if len(issues) != 1 || issues[0].Field != "sessions.idleStop" {
		t.Fatalf("issues = %v", issues)
	}
	if d, on := cfg.Sessions.IdleStopAfter(); d != DefaultIdleStop || !on {
		t.Errorf("an invalid time gave %v, %v", d, on)
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestMissingFileGivesDefaults(t *testing.T) {
	cfg, issues, err := Load(t.TempDir())
	if err != nil || len(issues) != 0 || !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("Load = %+v, %v, %v", cfg, issues, err)
	}
}

func TestValidFile(t *testing.T) {
	cfg, issues, err := Load(write(t, "version: 1\nui:\n  theme: dark\n"))
	if err != nil || len(issues) != 0 {
		t.Fatalf("issues = %v, err = %v", issues, err)
	}
	if cfg.UI.Theme != "dark" {
		t.Errorf("theme = %q", cfg.UI.Theme)
	}
}

func TestEmptyFileGivesDefaults(t *testing.T) {
	cfg, issues, err := Load(write(t, ""))
	if err != nil || len(issues) != 0 || !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("Load = %+v, %v, %v", cfg, issues, err)
	}
}

func TestSyntaxErrorIsAnIssueNotACrash(t *testing.T) {
	cfg, issues, err := Load(write(t, "ui: [unclosed\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("cfg = %+v, issues = %v", cfg, issues)
	}
	if issues[0].File == "" {
		t.Error("issue does not name the file")
	}
}

func TestUnknownFieldIsAnIssue(t *testing.T) {
	_, issues, _ := Load(write(t, "version: 1\nthme: dark\n"))
	if len(issues) != 1 {
		t.Fatalf("issues = %v, want one for the unknown field", issues)
	}
}

func TestInvalidValueFallsBackPerField(t *testing.T) {
	cfg, issues, _ := Load(write(t, "version: 1\nui:\n  theme: purple\n"))
	if len(issues) != 1 || issues[0].Field != "ui.theme" {
		t.Fatalf("issues = %v", issues)
	}
	if cfg.UI.Theme != "system" {
		t.Errorf("theme = %q, want fallback to system", cfg.UI.Theme)
	}
}

// GUI-048: the interface's language, validated as the theme is.
func TestLanguage(t *testing.T) {
	for _, lang := range []string{"system", "en", "de", "nl"} {
		cfg, issues, _ := Load(write(t, "version: 1\nui:\n  language: "+lang+"\n"))
		if len(issues) != 0 || cfg.UI.Language != lang {
			t.Errorf("language %s: got %q, issues %v", lang, cfg.UI.Language, issues)
		}
	}
	cfg, issues, _ := Load(write(t, "version: 1\nui:\n  language: fr\n"))
	if len(issues) != 1 || issues[0].Field != "ui.language" || cfg.UI.Language != "system" {
		t.Errorf("language fr: got %q, issues %v", cfg.UI.Language, issues)
	}
	if cfg, issues, _ := Load(write(t, "version: 1\nui:\n  theme: dark\n")); len(issues) != 0 || cfg.UI.Language != "system" {
		t.Errorf("no language: got %q, issues %v", cfg.UI.Language, issues)
	}
	c := Default()
	c.UI.Language = "klingon"
	if err := Save(t.TempDir(), c); err == nil {
		t.Error("an unknown language saved")
	}
}

func TestDataDirHonoursHome(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "home")
	got, err := DataDir(dir)
	if err != nil || got != dir {
		t.Fatalf("DataDir = %q, %v", got, err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("DataDir did not create %s", dir)
	}
}

func TestSaveAndFontSize(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.UI.FontSize, c.Defaults.ProviderID, c.Permissions.AllowBypass = 16, "anthropic", true
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	got, issues, err := Load(dir)
	if err != nil || len(issues) != 0 || !reflect.DeepEqual(got, c) {
		t.Fatalf("Load = %+v, %v, %v", got, issues, err)
	}
	c.UI.FontSize = 40
	if err := Save(dir, c); err == nil {
		t.Error("font size 40 saved")
	}
	if _, issues, _ := Load(write(t, "version: 1\nui:\n  theme: dark\n  fontSize: 3\n")); len(issues) != 1 {
		t.Errorf("issues = %v", issues)
	}
}

// Review finding: an unknown key made the whole file default, turning off
// local-only mode and hooks.
func TestUnknownKeyKeepsTheRest(t *testing.T) {
	cfg, issues := parse([]byte("network:\n  localOnly: true\nui:\n  thme: dark\nhooks:\n  - event: pre_tool_use\n    command: ./check\n"))
	if !cfg.Network.LocalOnly || len(cfg.Hooks) != 1 {
		t.Errorf("cfg = %+v", cfg)
	}
	if len(issues) == 0 || Unreadable(issues) {
		t.Errorf("issues = %+v", issues)
	}
	if _, issues := parse([]byte("network: [unclosed\n")); !Unreadable(issues) {
		t.Errorf("a broken file is not unreadable: %+v", issues)
	}
}

// Review finding: a config.yaml that could not be read at startup started
// the app with defaults, local-only mode and hooks off. It now starts with
// the last version that could be read.
func TestUnreadableAtStartupUsesTheLastGoodCopy(t *testing.T) {
	dir := t.TempDir()
	good := Default()
	good.Network.LocalOnly = true
	if err := Save(dir, good); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, FileName), []byte("network: [unclosed\n"), 0o600)
	cfg, issues, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Network.LocalOnly || !Unreadable(issues) {
		t.Errorf("localOnly %v, issues %+v", cfg.Network.LocalOnly, issues)
	}
	if !strings.Contains(issues[0].Message, "last time it could be read") {
		t.Errorf("issue = %q", issues[0].Message)
	}
}
