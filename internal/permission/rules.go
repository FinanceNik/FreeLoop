package permission

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// Effect of a rule.
type Effect string

const (
	Allow Effect = "allow"
	Deny  Effect = "deny"
)

// Scope is where a rule lives (SEC-002). M1 has global (rules.yaml in the
// app data directory) and session; environment, profile and project scopes
// arrive with those features.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeSession Scope = "session"
)

// Rule is Freeloop's canonical rule (OQ-010): a tool, and optionally a
// command or a path pattern.
//
//   - tool: a tool name such as shell or write_file, or a pattern such as
//     mcp__github__* or *.
//   - command (shell): an exact command, or a prefix written "git *", which
//     matches "git" and anything starting with "git ".
//   - path (file tools): a glob with ** for any depth, relative to the
//     project root, or absolute.
//
// A rule with neither command nor path matches every call of its tools.
type Rule struct {
	Effect  Effect `yaml:"effect" json:"effect"`
	Tool    string `yaml:"tool" json:"tool"`
	Command string `yaml:"command,omitempty" json:"command,omitempty"`
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
	// Scope and Source are filled in by whoever loads the rule.
	Scope  Scope  `yaml:"-" json:"scope"`
	Source string `yaml:"-" json:"source"`
}

// Validate reports a rule that cannot match anything sensible.
func (r Rule) Validate() error {
	switch {
	case r.Effect != Allow && r.Effect != Deny:
		return fmt.Errorf("effect must be allow or deny, not %q", r.Effect)
	case strings.TrimSpace(r.Tool) == "":
		return errors.New("tool is required")
	case r.Command != "" && r.Path != "":
		return errors.New("a rule has a command or a path, not both")
	}
	if _, err := path.Match(r.Tool, ""); err != nil {
		return fmt.Errorf("bad tool pattern %q: %w", r.Tool, err)
	}
	return nil
}

// String renders a rule the way the UI and decisions show it.
func (r Rule) String() string {
	s := string(r.Effect) + " " + r.Tool
	switch {
	case r.Command != "":
		s += fmt.Sprintf("(%s)", r.Command)
	case r.Path != "":
		s += fmt.Sprintf("(%s)", r.Path)
	}
	return s
}

// shellOperators split a command line into the commands it runs.
var shellOperators = []string{"&&", "||", ";", "|", "&", "\n"}

// risky marks command lines an allow rule must never approve: substitutions
// and redirections can run or write anything regardless of the prefix.
var risky = []string{"$(", "`", ">", "<"}

func normalize(cmd string) string { return strings.Join(strings.Fields(cmd), " ") }

// segments splits a command line at shell operators.
func segments(cmd string) []string {
	parts := []string{cmd}
	for _, op := range shellOperators {
		var next []string
		for _, p := range parts {
			next = append(next, strings.Split(p, op)...)
		}
		parts = next
	}
	var out []string
	for _, p := range parts {
		if n := normalize(p); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func commandMatches(pattern, cmd string) bool {
	pattern = normalize(pattern)
	if prefix, ok := strings.CutSuffix(pattern, " *"); ok {
		return cmd == prefix || strings.HasPrefix(cmd, prefix+" ")
	}
	if pattern == "*" {
		return true
	}
	return cmd == pattern
}

// matchCommand: a deny rule matches if any command in the line matches. An
// exact allow rule matches exactly the line it names, operators included,
// since that is what the user approved; a prefix allow rule matches only a
// single plain command.
func (r Rule) matchCommand(cmd string) bool {
	if r.Effect == Deny {
		return slices.ContainsFunc(segments(cmd), func(s string) bool { return commandMatches(r.Command, s) })
	}
	pattern := normalize(r.Command)
	if pattern != "*" && !strings.HasSuffix(pattern, " *") {
		return normalize(cmd) == pattern
	}
	segs := segments(cmd)
	if len(segs) != 1 || slices.ContainsFunc(risky, func(m string) bool { return strings.Contains(cmd, m) }) {
		return false
	}
	return commandMatches(r.Command, segs[0])
}

func (r Rule) matchPath(root, p string) bool {
	pattern := r.Path
	if filepath.IsAbs(pattern) {
		return environment.MatchGlob(strings.TrimPrefix(filepath.ToSlash(pattern), "/"), strings.TrimPrefix(filepath.ToSlash(p), "/"))
	}
	for _, rt := range canonical(root) {
		if rel, err := filepath.Rel(rt, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return environment.MatchGlob(pattern, filepath.ToSlash(rel))
		}
	}
	return false
}

// Matches reports whether r applies to req in a session rooted at root. An
// allow rule with a path must cover every path the call touches; a deny rule
// with a path matches if any path does.
func (r Rule) Matches(req Request, root string) bool {
	if ok, _ := path.Match(r.Tool, req.Tool); !ok {
		return false
	}
	switch {
	case r.Command != "":
		return req.Command != "" && r.matchCommand(req.Command)
	case r.Path != "":
		if len(req.Paths) == 0 {
			return false
		}
		match := func(p string) bool { return r.matchPath(root, p) }
		if r.Effect == Deny {
			return slices.ContainsFunc(req.Paths, match)
		}
		for _, p := range req.Paths {
			if !match(p) {
				return false
			}
		}
		return true
	}
	return true
}

// RuleFor is the rule "Allow always" creates for req: the exact command, or
// the exact path (relative to root when inside it), or the tool alone.
func RuleFor(req Request, root string, scope Scope) Rule {
	r := Rule{Effect: Allow, Tool: req.Tool, Scope: scope}
	switch {
	case req.Command != "":
		r.Command = normalize(req.Command)
	case len(req.Paths) == 1:
		r.Path = req.Paths[0]
		for _, rt := range canonical(root) {
			if rel, err := filepath.Rel(rt, req.Paths[0]); err == nil && !strings.HasPrefix(rel, "..") {
				r.Path = filepath.ToSlash(rel)
				break
			}
		}
	}
	return r
}

// RuleSource supplies the rules in force for a session, every scope
// together.
type RuleSource interface {
	Rules(sessionID string) []Rule
}

// RuleSink stores a rule created by "Allow always".
type RuleSink interface {
	AddRule(sessionID string, r Rule) error
}
