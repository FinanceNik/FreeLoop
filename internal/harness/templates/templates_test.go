package templates

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func render(t *testing.T, lang string) string {
	t.Helper()
	files, err := Render(lang, "Test agent", "example.com/test-agent")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for rel, b := range files {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, rel), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// EXT-003: each template's own test, which runs the agent over the
// protocol the way Freeloop does, passes as rendered.
func TestPythonTemplate(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	dir := render(t, "python")
	out, err := exec.Command(py, "-I", filepath.Join(dir, "test_agent.py")).CombinedOutput()
	if err != nil || string(out) != "ok\n" {
		t.Fatalf("test_agent.py: %v\n%s", err, out)
	}
}

func TestGoTemplate(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go")
	}
	dir := render(t, "go")
	cmd := exec.Command(gobin, "test", "./...")
	cmd.Dir = dir
	// Offline: the SDK is in the module cache, as this module needs it.
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOFLAGS=-mod=mod", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
}

func TestRender(t *testing.T) {
	files, err := Render("go", "My agent", "example.com/my-agent")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["go.mod"]; !ok || string(files["go.mod"][:27]) != "module example.com/my-agent" {
		t.Errorf("files = %v", files["go.mod"])
	}
	if _, err := Render("rust", "x", "y"); err == nil {
		t.Error("a missing template rendered")
	}
}
