package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SET-005: a backup holds the config files and no others; restoring it
// writes them back, keeping the replaced files; a backup that names
// anything else, or holds broken YAML, changes nothing.
func TestBundleRoundTrip(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, ProfilesDir), 0o700)
	os.MkdirAll(filepath.Join(src, "sessions", "x"), 0o700)
	os.WriteFile(filepath.Join(src, FileName), []byte("version: 1\n"), 0o600)
	os.WriteFile(filepath.Join(src, ProvidersFileName), []byte("providers:\n  - id: work\n    type: anthropic\n    apiKey: keychain://freeloop/provider/work\n"), 0o600)
	os.WriteFile(filepath.Join(src, ProfilesDir, "review.yaml"), []byte("name: Review\n"), 0o600)
	os.WriteFile(filepath.Join(src, "sessions", "x", "events.jsonl"), []byte("{}\n"), 0o600)
	b, err := ExportBundle(src)
	if err != nil {
		t.Fatal(err)
	}
	var bundle Bundle
	json.Unmarshal(b, &bundle)
	if len(bundle.Files) != 3 || bundle.Files["profiles/review.yaml"] != "name: Review\n" {
		t.Fatalf("files = %v", bundle.Files)
	}

	dst := t.TempDir()
	os.WriteFile(filepath.Join(dst, FileName), []byte("version: 1\nui:\n  theme: dark\n"), 0o600)
	written, err := ImportBundle(dst, b)
	if err != nil || len(written) != 3 {
		t.Fatalf("import = %v, %v", written, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, ProfilesDir, "review.yaml")); string(got) != "name: Review\n" {
		t.Errorf("restored profile = %q", got)
	}
	kept, _ := filepath.Glob(filepath.Join(dst, "backups", "*", FileName))
	if len(kept) != 1 {
		t.Fatalf("the replaced config.yaml was not kept: %v", kept)
	}
	if old, _ := os.ReadFile(kept[0]); !strings.Contains(string(old), "dark") {
		t.Errorf("kept = %q", old)
	}

	for name, files := range map[string]map[string]string{
		"a path outside": {"../evil.yaml": "x: 1\n"},
		"a session file": {"sessions/x/events.jsonl": "{}"},
		"broken YAML":    {FileName: "version: [\n"},
	} {
		bad, _ := json.Marshal(Bundle{Format: BundleFormat, Version: 1, Files: files})
		before, _ := os.ReadFile(filepath.Join(dst, FileName))
		if _, err := ImportBundle(dst, bad); err == nil {
			t.Errorf("%s was imported", name)
		}
		if after, _ := os.ReadFile(filepath.Join(dst, FileName)); string(after) != string(before) {
			t.Errorf("%s changed config.yaml", name)
		}
	}
}
