package permission

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// A protected location is protected under the data volume's name for it too: on a Mac
// /System/Volumes/Data/Users/me is /Users/me, and the policy used to compare the strings
// (SEC-005). Reproduced on 2026-11-11 with `freeloop check read_file`.
func TestProtectedLocationsHoldThroughTheDataVolume(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the data volume is macOS's")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	home = environment.Canonical(home)
	data := filepath.Join(home, "Library", "Application Support", "freeloop")
	p := NewPolicy(home, data)
	for _, path := range []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(data, "config.yaml"),
	} {
		through := "/System/Volumes/Data" + path
		if p.readRefusal(path) == "" {
			t.Fatalf("%s is readable even directly", path)
		}
		if p.readRefusal(through) == "" {
			t.Errorf("%s is readable through the data volume", through)
		}
		if p.writeRefusal(filepath.Join(home, "repo"), through) == "" {
			t.Errorf("%s is writable through the data volume", through)
		}
	}
}

// A path deny rule holds through the data volume's name, as a literal path and as a glob,
// and a rule written with that name holds for the plain one.
func TestPathRulesHoldThroughTheDataVolume(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the data volume is macOS's")
	}
	root := environment.Canonical(t.TempDir())
	secret := filepath.Join(root, "vault", "key.pem")
	for _, c := range []struct {
		name string
		rule Rule
		path string
	}{
		{"glob, plain path in the rule", Rule{Effect: Deny, Tool: "read_file", Path: filepath.Join(root, "vault", "**")}, "/System/Volumes/Data" + secret},
		{"literal, plain path in the rule", Rule{Effect: Deny, Tool: "read_file", Path: secret, Literal: true}, "/System/Volumes/Data" + secret},
		{"relative glob", Rule{Effect: Deny, Tool: "read_file", Path: "vault/**"}, "/System/Volumes/Data" + secret},
		{"glob written with the volume's name", Rule{Effect: Deny, Tool: "read_file", Path: "/System/Volumes/Data" + filepath.Join(root, "vault", "**")}, secret},
	} {
		if !c.rule.matchPath(root, c.path) {
			t.Errorf("%s: %q does not match %+v", c.name, c.path, c.rule)
		}
	}
}

// What a tool reaches goes through the environment's guard, which sees the path as the
// environment resolved it: the file tools, search and an external agent's reads alike.
func TestTheEnvironmentRefusesAProtectedFileThroughTheDataVolume(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the data volume is macOS's")
	}
	home := environment.Canonical(t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(home, ".ssh", "id_ed25519")
	if err := os.WriteFile(key, []byte("PRIVATE"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(home, "repo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	p := NewPolicy(home, filepath.Join(home, "data"))
	// The project itself is opened through the volume's name.
	env, err := environment.NewLocal(environment.LocalOptions{ID: "t", Root: "/System/Volumes/Data" + project, Guard: p.Guard(project)})
	if err != nil {
		t.Fatal(err)
	}
	if env.Root() != project {
		t.Errorf("root %q, want %q", env.Root(), project)
	}
	if !env.Contains("/System/Volumes/Data" + filepath.Join(project, "a.go")) {
		t.Error("a file of the project, spelled through the data volume, is outside it")
	}
	through := "/System/Volumes/Data" + key
	if got, err := env.FS().ReadFile(through); err == nil {
		t.Errorf("read %q through the data volume: %q", through, got)
	}
	if _, err := env.FS().ReadFile(key); err == nil {
		t.Error("read the key directly")
	}
	if got, err := env.FS().Grep(context.Background(), "/System/Volumes/Data"+filepath.Join(home, ".ssh"), "PRIVATE", "", false, 10); err == nil && len(got) > 0 {
		t.Errorf("grep found the key through the data volume: %+v", got)
	}
	if got, err := env.FS().Grep(context.Background(), filepath.Join(home, ".ssh"), "PRIVATE", "", false, 10); err == nil && len(got) > 0 {
		t.Errorf("grep found the key directly: %+v", got)
	}
}
