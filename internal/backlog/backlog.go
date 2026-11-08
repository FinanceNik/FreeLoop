// Package backlog reads a workflow's backlog (WFL-004): PRDs and issues as Markdown
// files with YAML frontmatter, and which issues are done, ready or blocked. It reads
// bytes it is given; the engine reads the files through the environment.
package backlog

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Folders inside a backlog.
const (
	IssuesDir = "issues"
	PRDsDir   = "prds"
)

// Issue statuses and types as files write them.
const (
	StatusOpen = "open"
	StatusDone = "done"
	TypeAFK    = "afk"
	TypeHuman  = "human"
)

// States of an issue in its backlog.
const (
	Done    = "done"
	Ready   = "ready"
	Blocked = "blocked"
)

// Issue is one issue file.
type Issue struct {
	// ID is the file name without .md, which blockedBy names.
	ID string `json:"id"`
	// Path is the file's path inside the project, with slashes.
	Path   string `json:"path"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Type is afk or human; human when the file says neither, so nothing runs
	// unattended that was not marked for it.
	Type      string   `json:"type"`
	BlockedBy []string `json:"blockedBy"`
	PRD       string   `json:"prd"`
	// Problem says what is wrong with the frontmatter, if anything.
	Problem string `json:"problem,omitempty"`
}

// PRD is one PRD file.
type PRD struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// Item is an issue with its place in the backlog.
type Item struct {
	Issue
	State string `json:"state"`
	// WaitsOn are the blockers not done yet, or not found.
	WaitsOn []string `json:"waitsOn"`
	// Blocks are the issues waiting on this one.
	Blocks []string `json:"blocks"`
	// Cycle is set when it waits on itself through others.
	Cycle bool `json:"cycle,omitempty"`
}

type front struct {
	Title     string    `yaml:"title"`
	Status    string    `yaml:"status"`
	Type      string    `yaml:"type"`
	BlockedBy stringish `yaml:"blockedBy"`
	PRD       string    `yaml:"prd"`
}

// stringish takes a list of strings or one string.
type stringish []string

func (s *stringish) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Tag == "!!null" || strings.TrimSpace(n.Value) == "" {
			*s = nil
			return nil
		}
		*s = []string{n.Value}
		return nil
	case yaml.SequenceNode:
		var out []string
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: blockedBy lists file names", c.Line)
			}
			out = append(out, c.Value)
		}
		*s = out
		return nil
	}
	return fmt.Errorf("line %d: blockedBy lists file names", n.Line)
}

// split returns a file's frontmatter and body; no frontmatter is an empty one.
func split(b []byte) (fm, body []byte, ok bool) {
	b = bytes.TrimPrefix(b, []byte("\ufeff"))
	s := bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(s, []byte("---\n")) {
		return nil, s, false
	}
	rest := s[4:]
	if bytes.HasPrefix(rest, []byte("---\n")) || bytes.Equal(rest, []byte("---")) {
		return nil, bytes.TrimPrefix(bytes.TrimPrefix(rest, []byte("---")), []byte("\n")), true
	}
	i := bytes.Index(rest, []byte("\n---\n"))
	if i < 0 {
		if bytes.HasSuffix(rest, []byte("\n---")) {
			return rest[:len(rest)-4], nil, true
		}
		return nil, s, false
	}
	return rest[:i], rest[i+5:], true
}

// Body returns a file's text after its frontmatter.
func Body(b []byte) string {
	_, body, _ := split(b)
	return string(body)
}

// ID is an issue's ID as blockedBy may write it: the file name, with or without
// .md, a folder or a leading #.
func ID(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimPrefix(s, "#")
	return strings.TrimSuffix(s, ".md")
}

// ParseIssue reads one issue file; a problem with it is reported on the issue, which
// then counts as human and open, so nothing runs from a file the app cannot read.
func ParseIssue(path string, b []byte) Issue {
	is := Issue{ID: ID(path), Path: path, Status: StatusOpen, Type: TypeHuman}
	fm, body, ok := split(b)
	var f front
	switch {
	case !ok:
		is.Problem = "it has no frontmatter between --- lines"
	default:
		if err := yaml.Unmarshal(fm, &f); err != nil {
			is.Problem = "its frontmatter does not read: " + err.Error()
		}
	}
	is.Title = strings.TrimSpace(f.Title)
	if is.Title == "" {
		is.Title = heading(body, is.ID)
	}
	switch s := strings.ToLower(strings.TrimSpace(f.Status)); s {
	case StatusDone, "closed", "complete", "completed":
		is.Status = StatusDone
	case "", StatusOpen, "todo", "in-progress", "in progress", "blocked":
		is.Status = StatusOpen
	default:
		is.Status = StatusOpen
		if is.Problem == "" {
			is.Problem = fmt.Sprintf("status %q is not open or done", f.Status)
		}
	}
	switch t := strings.ToLower(strings.TrimSpace(f.Type)); t {
	case TypeAFK:
		is.Type = TypeAFK
	case "", TypeHuman, "hitl":
	default:
		if is.Problem == "" {
			is.Problem = fmt.Sprintf("type %q is not afk or human", f.Type)
		}
	}
	if is.Problem != "" {
		is.Type = TypeHuman
	}
	for _, b := range f.BlockedBy {
		if id := ID(b); id != "" && id != is.ID && !slices.Contains(is.BlockedBy, id) {
			is.BlockedBy = append(is.BlockedBy, id)
		}
	}
	is.PRD = ID(f.PRD)
	return is
}

// ParsePRD reads one PRD file.
func ParsePRD(path string, b []byte) PRD {
	p := PRD{ID: ID(path), Path: path, Status: StatusOpen}
	fm, body, ok := split(b)
	var f front
	if ok {
		yaml.Unmarshal(fm, &f)
	}
	p.Title = strings.TrimSpace(f.Title)
	if p.Title == "" {
		p.Title = heading(body, p.ID)
	}
	if strings.EqualFold(strings.TrimSpace(f.Status), StatusDone) {
		p.Status = StatusDone
	}
	return p
}

// heading is a body's first Markdown heading, or fallback.
func heading(body []byte, fallback string) string {
	for _, line := range strings.Split(string(body), "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(line), "#"); ok {
			if t = strings.TrimSpace(strings.TrimLeft(t, "#")); t != "" {
				return t
			}
		}
	}
	return fallback
}

// Analyze places each issue: done, ready (every blocker done), or blocked (a blocker
// open, missing, or a cycle back to itself). Items come sorted by ID.
func Analyze(issues []Issue) []Item {
	byID := map[string]Issue{}
	for _, is := range issues {
		byID[is.ID] = is
	}
	items := make([]Item, 0, len(issues))
	for _, is := range issues {
		it := Item{Issue: is, WaitsOn: []string{}, Blocks: []string{}}
		if it.BlockedBy == nil {
			it.BlockedBy = []string{}
		}
		for _, b := range is.BlockedBy {
			if dep, ok := byID[b]; !ok || dep.Status != StatusDone {
				it.WaitsOn = append(it.WaitsOn, b)
			}
		}
		for _, other := range issues {
			if slices.Contains(other.BlockedBy, is.ID) {
				it.Blocks = append(it.Blocks, other.ID)
			}
		}
		slices.Sort(it.Blocks)
		switch {
		case is.Status == StatusDone:
			it.State = Done
		case len(it.WaitsOn) > 0:
			it.State = Blocked
		default:
			it.State = Ready
		}
		it.Cycle = is.Status != StatusDone && cyclic(is.ID, byID)
		if it.Cycle {
			it.State = Blocked
		}
		items = append(items, it)
	}
	slices.SortFunc(items, func(a, b Item) int { return strings.Compare(a.ID, b.ID) })
	return items
}

// cyclic reports whether id waits on itself through open issues.
func cyclic(id string, byID map[string]Issue) bool {
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(at string) bool {
		for _, b := range byID[at].BlockedBy {
			dep, ok := byID[b]
			if !ok || dep.Status == StatusDone {
				continue
			}
			if b == id {
				return true
			}
			if !seen[b] {
				seen[b] = true
				if walk(b) {
					return true
				}
			}
		}
		return false
	}
	return walk(id)
}

// NextAFK is the first ready AFK issue that skip does not leave out.
func NextAFK(items []Item, skip func(id string) bool) (Item, bool) {
	for _, it := range items {
		if it.State == Ready && it.Type == TypeAFK && it.Problem == "" && (skip == nil || !skip(it.ID)) {
			return it, true
		}
	}
	return Item{}, false
}

// Summary lists the issues for a prompt, one a line: ID, state, type and title, with
// what each waits on.
func Summary(items []Item) string {
	if len(items) == 0 {
		return "(no issues yet)"
	}
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "- %s [%s, %s] %s", it.Path, it.State, it.Type, it.Title)
		if len(it.WaitsOn) > 0 {
			fmt.Fprintf(&b, " (waits on %s)", strings.Join(it.WaitsOn, ", "))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Finished are the PRDs every issue of which is done, at least one of them.
func Finished(prds []PRD, items []Item) []PRD {
	var out []PRD
	for _, p := range prds {
		n, done := 0, 0
		for _, it := range items {
			if it.PRD == p.ID {
				n++
				if it.State == Done {
					done++
				}
			}
		}
		if (n > 0 && n == done) || (n == 0 && p.Status == StatusDone) {
			out = append(out, p)
		}
	}
	return out
}
