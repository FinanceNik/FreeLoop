// Package skill reads Agent Skills (SKL-001): a folder holding SKILL.md
// with YAML frontmatter and optional scripts/, references/ and assets/. It
// reads through a FileSystem, never the OS directly, so project skills are
// read through the project's environment like every project file.
package skill

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// FileName is the file that makes a folder a skill.
const FileName = "SKILL.md"

// Sources, in precedence order: a skill found earlier hides one of the same
// name found later (docs/ARCHITECTURE.md section 7).
const (
	SourceFreeloop = ".freeloop"
	SourceAgents   = ".agents"
	SourceClaude   = ".claude"
	SourceApp      = "app"
)

// FS is the part of an environment's file system discovery needs.
type FS interface {
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]environment.FileInfo, error)
}

// Skill is one skill's frontmatter and where it was found.
type Skill struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	// AllowedTools is experimental in the spec and only shown, never
	// treated as permission.
	AllowedTools []string `json:"allowedTools,omitempty"`
	Source       string   `json:"source"`
	// Dir is the skill's folder relative to the FS it was read from; Path
	// is its absolute path, for display and for running its scripts.
	Dir  string `json:"dir"`
	Path string `json:"path"`
	// Lines counts the body's lines; the spec recommends under 500.
	Lines int `json:"lines"`
}

// Issue is a skill that could not be loaded.
type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type frontmatter struct {
	Name          string         `yaml:"name"`
	Description   string         `yaml:"description"`
	License       string         `yaml:"license"`
	Compatibility string         `yaml:"compatibility"`
	Metadata      map[string]any `yaml:"metadata"`
	AllowedTools  yaml.Node      `yaml:"allowed-tools"`
}

// Names follow the spec: lowercase letters, digits and single hyphens, not
// at either end, at most 64 characters.
var namePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidName reports whether name is a valid skill name.
func ValidName(name string) bool { return len(name) <= 64 && namePattern.MatchString(name) }

// Split separates SKILL.md into its frontmatter and body.
func Split(b []byte) (front, body []byte, err error) {
	b = bytes.TrimPrefix(b, []byte("\ufeff"))
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return nil, nil, errors.New("SKILL.md must start with YAML frontmatter between --- lines")
	}
	rest := s[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, nil, errors.New("the frontmatter is not closed with ---")
	}
	body = []byte(strings.TrimLeft(strings.TrimPrefix(rest[end+4:], "\n"), "\n"))
	return []byte(rest[:end]), body, nil
}

// Parse reads the skill in dir. Fields beyond the spec, which other tools
// add, are ignored so their skills load unchanged; a missing name is the
// folder's.
func Parse(fs FS, dir, source, abs string) (Skill, error) {
	b, err := fs.ReadFile(path.Join(dir, FileName))
	if err != nil {
		return Skill{}, err
	}
	front, body, err := Split(b)
	if err != nil {
		return Skill{}, err
	}
	var fm frontmatter
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return Skill{}, fmt.Errorf("frontmatter: %w", err)
	}
	folder := path.Base(dir)
	s := Skill{
		Name: strings.TrimSpace(fm.Name), Description: strings.TrimSpace(fm.Description),
		License: fm.License, Compatibility: fm.Compatibility, Source: source, Dir: dir, Path: abs,
		Lines: bytes.Count(body, []byte("\n")) + 1,
	}
	if s.Name == "" {
		s.Name = folder
	}
	switch {
	case !ValidName(s.Name):
		return s, fmt.Errorf("name %q must be lowercase letters, digits and single hyphens, at most 64 characters", s.Name)
	case s.Name != folder:
		return s, fmt.Errorf("name %q must equal the folder name %q", s.Name, folder)
	case s.Description == "":
		return s, errors.New("description is required")
	case len(s.Description) > 1024:
		return s, fmt.Errorf("description is %d characters; at most 1024", len(s.Description))
	case len(s.Compatibility) > 500:
		return s, fmt.Errorf("compatibility is %d characters; at most 500", len(s.Compatibility))
	}
	if len(fm.Metadata) > 0 {
		s.Metadata = map[string]string{}
		for k, v := range fm.Metadata {
			s.Metadata[k] = fmt.Sprint(v)
		}
	}
	s.AllowedTools = allowedTools(fm.AllowedTools)
	return s, nil
}

// allowedTools accepts the spec's space-separated string as well as the
// comma-separated strings and lists other tools write.
func allowedTools(n yaml.Node) []string {
	var out []string
	switch n.Kind {
	case yaml.ScalarNode:
		for _, f := range strings.FieldsFunc(n.Value, func(r rune) bool { return r == ',' || r == ' ' }) {
			out = append(out, f)
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			out = append(out, strings.TrimSpace(c.Value))
		}
	}
	return out
}

// Discover lists the skills in the folders below base; root is base's
// absolute path. A missing base has none.
func Discover(fs FS, base, source, root string) ([]Skill, []Issue) {
	entries, err := fs.ReadDir(base)
	if err != nil {
		return nil, nil
	}
	var out []Skill
	var issues []Issue
	for _, e := range entries {
		if !e.IsDir || strings.HasPrefix(e.Name, ".") {
			continue
		}
		dir := path.Join(base, e.Name)
		if _, err := fs.ReadFile(path.Join(dir, FileName)); err != nil {
			continue // not a skill
		}
		s, err := Parse(fs, dir, source, path.Join(root, e.Name))
		if err != nil {
			issues = append(issues, Issue{Path: path.Join(root, e.Name, FileName), Message: err.Error()})
			continue
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return out, issues
}

// Merge combines lists in precedence order: the first skill of a name wins.
func Merge(lists ...[]Skill) []Skill {
	var out []Skill
	seen := map[string]bool{}
	for _, l := range lists {
		for _, s := range l {
			if !seen[s.Name] {
				seen[s.Name] = true
				out = append(out, s)
			}
		}
	}
	slices.SortFunc(out, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Body returns SKILL.md without its frontmatter.
func Body(fs FS, s Skill) (string, error) {
	b, err := fs.ReadFile(path.Join(s.Dir, FileName))
	if err != nil {
		return "", err
	}
	_, body, err := Split(b)
	return string(body), err
}

// Files lists the files in a skill's folder, relative to it, at most limit.
func Files(fs FS, s Skill, limit int) []string {
	var out []string
	var walk func(rel string, depth int)
	walk = func(rel string, depth int) {
		entries, err := fs.ReadDir(path.Join(s.Dir, rel))
		if err != nil || depth > 4 {
			return
		}
		for _, e := range entries {
			if len(out) >= limit || strings.HasPrefix(e.Name, ".") {
				continue
			}
			p := path.Join(rel, e.Name)
			if e.IsDir {
				walk(p, depth+1)
			} else if p != FileName {
				out = append(out, p)
			}
		}
	}
	walk("", 0)
	slices.Sort(out)
	return out
}
