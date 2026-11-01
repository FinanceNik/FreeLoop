package archtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// NFR-SEC-004, S-08: what a build takes in is named exactly and by checksum, so
// a release can be built again from its tag.
func TestDependenciesArePinned(t *testing.T) {
	goMod := readRepo(t, "go.mod")
	if !regexp.MustCompile(`(?m)^toolchain go\d+\.\d+\.\d+$`).MatchString(goMod) {
		t.Error("go.mod names no exact toolchain")
	}
	wails := regexp.MustCompile(`github\.com/wailsapp/wails/v3 (v\S+)`).FindStringSubmatch(goMod)
	if wails == nil || !strings.Contains(readRepo(t, "Makefile"), "WAILS_VERSION := "+wails[1]+"\n") {
		t.Errorf("the Makefile's WAILS_VERSION does not match go.mod's %v", wails)
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(readRepo(t, "frontend/package.json")), &pkg); err != nil {
		t.Fatal(err)
	}
	exact := regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		for name, v := range deps {
			if !exact.MatchString(v) {
				t.Errorf("package.json: %s is %q, not an exact version", name, v)
			}
		}
	}
	// S-08's budget: each one is code in the page that can call the engine.
	if len(pkg.Dependencies) > 4 {
		t.Errorf("%d npm packages ship with the app; the budget is 4, so ask before adding one", len(pkg.Dependencies))
	}

	var lock struct {
		Packages map[string]struct {
			Integrity string `json:"integrity"`
			Resolved  string `json:"resolved"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(readRepo(t, "frontend/package-lock.json")), &lock); err != nil {
		t.Fatal(err)
	}
	for key, p := range lock.Packages {
		if key == "" {
			continue
		}
		if !strings.HasPrefix(p.Integrity, "sha512-") {
			t.Errorf("package-lock.json: %s has no sha512 checksum", key)
		}
		if !strings.HasPrefix(p.Resolved, "https://registry.npmjs.org/") {
			t.Errorf("package-lock.json: %s comes from %q", key, p.Resolved)
		}
	}

	uses := regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*(\S+)`)
	sha := regexp.MustCompile(`@[0-9a-f]{40}$`)
	workflows, _ := filepath.Glob(filepath.Join(root, ".github/workflows/*.yml"))
	if len(workflows) == 0 {
		t.Error("no workflows found")
	}
	for _, f := range workflows {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range uses.FindAllStringSubmatch(string(b), -1) {
			if !sha.MatchString(m[1]) {
				t.Errorf("%s: %s is not pinned to a commit", filepath.Base(f), m[1])
			}
		}
	}
}

// A build must not change what it builds from.
func TestBuildLeavesTheModuleAlone(t *testing.T) {
	if b := readRepo(t, "build/Taskfile.yml"); regexp.MustCompile(`(?m)go mod tidy\s*$`).MatchString(b) {
		t.Error("the build runs go mod tidy, which can rewrite go.mod and go.sum")
	}
}
