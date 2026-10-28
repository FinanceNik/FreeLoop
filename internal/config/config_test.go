package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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

func TestDataDirHonoursEnv(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "home")
	t.Setenv(HomeEnv, dir)
	got, err := DataDir()
	if err != nil || got != dir {
		t.Fatalf("DataDir = %q, %v", got, err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("DataDir did not create %s", dir)
	}
}
