package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProvidersRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := []Provider{{ID: "anthropic", Name: "Anthropic", Type: "anthropic", APIKey: "keychain://freeloop/providers/anthropic", DefaultModel: "claude-opus-5-5"}}
	if err := SaveProviders(dir, in); err != nil {
		t.Fatal(err)
	}
	out, issues, err := LoadProviders(dir)
	if err != nil || len(issues) != 0 || len(out) != 1 || !reflect.DeepEqual(out[0], in[0]) {
		t.Fatalf("LoadProviders = %+v, %v, %v", out, issues, err)
	}
	fi, _ := os.Stat(filepath.Join(dir, ProvidersFileName))
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

func TestBadProvidersAreSkipped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ProvidersFileName), []byte(`version: 1
providers:
  - {id: ok, name: OK, type: anthropic}
  - {id: Bad ID, name: X, type: anthropic}
  - {id: ok, name: Dup, type: anthropic}
  - {id: notype, name: X}
`), 0o600)
	out, issues, _ := LoadProviders(dir)
	if len(out) != 1 || len(issues) != 3 {
		t.Fatalf("out = %+v, issues = %v", out, issues)
	}
	if !strings.Contains(issues[0].Field, "providers[1]") {
		t.Errorf("issue = %+v", issues[0])
	}
}
