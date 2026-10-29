package skill

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

type allow struct{}

func (allow) CheckRead(string) error  { return nil }
func (allow) CheckWrite(string) error { return nil }

func tree(t *testing.T, files map[string]string) (FS, string) {
	t.Helper()
	root := t.TempDir()
	for p, body := range files {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: allow{}})
	if err != nil {
		t.Fatal(err)
	}
	return env.FS(), env.Root()
}

// SKL-001: a skill written for Claude Code, with fields beyond the spec and
// a comma-separated allowed-tools, loads unchanged.
func TestDiscoverAndParse(t *testing.T) {
	fs, root := tree(t, map[string]string{
		"skills/pdf-tools/SKILL.md":           "---\nname: pdf-tools\ndescription: Fill and merge PDF forms.\nallowed-tools: Read, Bash(python:*)\nargument-hint: <file>\nmetadata:\n  version: 2\n---\n\n# PDF tools\n\nRun scripts/fill.py.\n",
		"skills/pdf-tools/scripts/fill.py":    "print('x')\n",
		"skills/pdf-tools/references/spec.md": "spec",
		"skills/no-name/SKILL.md":             "---\ndescription: Uses the folder name.\n---\nBody\n",
		"skills/wrong/SKILL.md":               "---\nname: other\ndescription: x\n---\n",
		"skills/nodesc/SKILL.md":              "---\nname: nodesc\n---\n",
		"skills/Bad_Name/SKILL.md":            "---\ndescription: x\n---\n",
		"skills/plain-folder/readme.md":       "not a skill",
	})
	list, issues := Discover(fs, "skills", SourceApp, filepath.Join(root, "skills"))
	var names []string
	for _, s := range list {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"no-name", "pdf-tools"}) {
		t.Fatalf("skills = %v, issues = %v", names, issues)
	}
	if len(issues) != 3 {
		t.Errorf("issues = %v", issues)
	}
	pdf := list[1]
	if pdf.Description != "Fill and merge PDF forms." || !slices.Equal(pdf.AllowedTools, []string{"Read", "Bash(python:*)"}) || pdf.Metadata["version"] != "2" {
		t.Errorf("pdf-tools = %+v", pdf)
	}
	body, err := Body(fs, pdf)
	if err != nil || !strings.HasPrefix(body, "# PDF tools") || strings.Contains(body, "description:") {
		t.Errorf("body = %q, %v", body, err)
	}
	if files := Files(fs, pdf, 10); !slices.Equal(files, []string{"references/spec.md", "scripts/fill.py"}) {
		t.Errorf("files = %v", files)
	}
}

func TestMergePrecedence(t *testing.T) {
	a := []Skill{{Name: "x", Source: SourceFreeloop}, {Name: "y", Source: SourceFreeloop}}
	b := []Skill{{Name: "x", Source: SourceApp}, {Name: "z", Source: SourceApp}}
	got := Merge(a, b)
	if len(got) != 3 || got[0].Source != SourceFreeloop || got[2].Name != "z" {
		t.Errorf("merge = %+v", got)
	}
}

func TestSplitNeedsFrontmatter(t *testing.T) {
	for _, in := range []string{"# no frontmatter", "---\nname: x\n"} {
		if _, _, err := Split([]byte(in)); err == nil {
			t.Errorf("Split(%q) accepted", in)
		}
	}
	if f, b, err := Split([]byte("\ufeff---\r\nname: x\r\n---\r\nbody\r\n")); err != nil || string(f) != "name: x" || string(b) != "body\n" {
		t.Errorf("CRLF and BOM: %q %q %v", f, b, err)
	}
}
