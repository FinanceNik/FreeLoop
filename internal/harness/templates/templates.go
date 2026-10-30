// Package templates holds the starting points for custom harnesses
// (HRN-011, EXT-003): ACP agents in Python and Go that already speak the
// protocol, each with a test that runs it the way Freeloop does. Files end
// in .tmpl so the Go template is not part of this module.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

//go:embed python go
var files embed.FS

// Languages are the templates there are.
var Languages = []string{"python", "go"}

// Render returns a template's files by path, named and filled in for an
// agent called name with Go module path module.
func Render(lang, name, module string) (map[string][]byte, error) {
	sub, err := fs.Sub(files, lang)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		text := strings.NewReplacer("AGENT_NAME", name, "MODULE_NAME", module).Replace(string(b))
		out[strings.TrimSuffix(p, ".tmpl")] = []byte(text)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("there is no %s template", lang)
	}
	return out, nil
}

// Command is how Freeloop starts an agent made from the template in dir.
func Command(lang, dir string) (string, []string) {
	if lang == "go" {
		return "go", []string{"-C", dir, "run", "."}
	}
	return "python3", []string{filepath.Join(dir, "agent.py")}
}

// Entry is the file to edit first.
func Entry(lang string) string {
	if lang == "go" {
		return "main.go"
	}
	return "agent.py"
}
