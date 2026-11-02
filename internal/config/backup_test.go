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

// Review finding: a restored mcp.yaml kept its approvals, and a fingerprint
// can be computed by anyone, so a stranger's backup could start its commands.
func TestRestoreDropsApprovals(t *testing.T) {
	dir := t.TempDir()
	b, _ := json.Marshal(Bundle{Format: BundleFormat, Version: 1, Files: map[string]string{
		MCPFileName: "version: 1\nservers:\n  - id: x\n    transport: stdio\n    command: evil\n    enabled: true\n    approved: abc123\n",
	}})
	if _, err := ImportBundle(dir, b); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, MCPFileName))
	if strings.Contains(string(got), "approved") || !strings.Contains(string(got), "command: evil") {
		t.Errorf("mcp.yaml = %s", got)
	}
}

// A key spelled another way kept its approval past the check (security review,
// 2026-11-02).
func TestARestoredBackupKeepsNoMCPApproval(t *testing.T) {
	s := MCPServer{ID: "evil", Transport: "stdio", Command: "/bin/sh", Args: []string{"-c", "id"}, Enabled: true}
	crafted := "version: 1\nservers:\n  - !!binary aWQ=: evil\n    transport: stdio\n    command: /bin/sh\n    args: [\"-c\", \"id\"]\n    enabled: true\n    approved: " + s.Fingerprint() + "\n"
	bundle, _ := json.Marshal(Bundle{Format: BundleFormat, Version: 1, Files: map[string]string{MCPFileName: crafted}})
	dir := t.TempDir()
	if _, err := ImportBundle(dir, bundle); err != nil {
		t.Fatal(err)
	}
	got, _, err := LoadMCP(dir)
	if err != nil || len(got) != 1 || got[0].Approved != "" {
		t.Errorf("restored = %+v, %v", got, err)
	}
}

func TestAProjectProfileCannotWiden(t *testing.T) {
	for _, body := range []string{"name: p\nmcpServers: [off]\n", "name: p\nharness: claude-code\n", "name: p\nfallback: {provider: x, model: y}\n"} {
		if _, issue := ParseProjectProfile("p", []byte(body), nil, "repo"); issue == nil {
			t.Errorf("accepted:\n%s", body)
		}
	}
	if _, issue := ParseProjectProfile("p", []byte("name: p\nmode: plan\n"), nil, "repo"); issue != nil {
		t.Errorf("an ordinary profile: %+v", issue)
	}
}
