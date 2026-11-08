package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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
	// Windows has no such modes; the user's own folder keeps others out there.
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
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

// One bad key must not cost the rest of the file at the next save.
func TestSavingDoesNotOverwriteAnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	good := []Provider{{ID: "a", Name: "A", Type: "anthropic"}, {ID: "b", Name: "B", Type: "anthropic"}}
	if err := SaveProviders(dir, good); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ProvidersFileName)
	body, _ := os.ReadFile(path)
	if err := SaveProviders(dir, good[:1]); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path + ".bak"); err != nil || string(b) != string(body) {
		t.Errorf("the previous file was not kept: %v", err)
	}
	for name, broken := range map[string]string{
		"a bad key":     string(body) + "unknownKey: 1\n",
		"a newer file":  strings.Replace(string(body), "version: 1", "version: 2", 1),
		"not even YAML": "providers: [\n",
	} {
		os.WriteFile(path, []byte(broken), 0o600)
		if err := SaveProviders(dir, []Provider{{ID: "c", Name: "C", Type: "anthropic"}}); err == nil {
			t.Errorf("%s was overwritten", name)
		}
		if b, _ := os.ReadFile(path); string(b) != broken {
			t.Errorf("%s changed", name)
		}
	}
	mcpPath := filepath.Join(dir, MCPFileName)
	os.WriteFile(mcpPath, []byte("version: 1\nservers: []\nunknownKey: 1\n"), 0o600)
	if err := SaveMCP(dir, nil); err == nil {
		t.Error("mcp.yaml was overwritten")
	}
}

func TestANewerConfigIsNotOverwritten(t *testing.T) {
	_, issues := parse([]byte("version: 2\n"))
	if !Unreadable(issues) {
		t.Errorf("issues: %+v", issues)
	}
}
