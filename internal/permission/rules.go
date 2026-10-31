package permission

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
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
	// ScopeEnvironment rules come from an environment file (SEC-002).
	ScopeEnvironment Scope = "environment"
	// ScopeProfile rules come from a harness profile (HRN-003).
	ScopeProfile Scope = "profile"
	// ScopeProject rules come from a trusted project's .freeloop/ and can
	// only deny (SET-002).
	ScopeProject Scope = "project"
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
//   - literal: the command or path is matched as written, never as a
//     pattern. "Allow always" sets it, so approving `rm -rf *` or a file
//     named `**` approves exactly that and nothing more.
//
// A rule with neither command nor path matches every call of its tools.
//
// Shell deny rules see through common wrappers (variable assignments, env,
// command, exec, a leading backslash, subshells, sh -c '...'), but a shell
// can always spell a command another way; until shell commands run in a
// sandbox, deny rules for shell are a guard against mistakes, not against a
// determined agent.
type Rule struct {
	Effect  Effect `yaml:"effect" json:"effect"`
	Tool    string `yaml:"tool" json:"tool"`
	Command string `yaml:"command,omitempty" json:"command,omitempty"`
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
	Literal bool   `yaml:"literal,omitempty" json:"literal,omitempty"`
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
	q := ""
	if r.Literal {
		q = "="
	}
	switch {
	case r.Command != "":
		s += fmt.Sprintf("(%s%s)", q, r.Command)
	case r.Path != "":
		s += fmt.Sprintf("(%s%s)", q, r.Path)
	}
	return s
}

// shellOperators split a command line into the commands it runs.
var shellOperators = []string{"&&", "||", ";", "|", "&", "\n"}

// expanding are the characters outside single quotes that let a shell run
// or write more than the words of a line: substitutions and expansions
// ($, backticks, zsh's =(...) and ${(e)...}), redirections, groups and
// zsh glob qualifiers such as *(e:...:), and escapes.
const expanding = "$`<>(){}\\"

// plain reports whether a line is only words a prefix allow rule can
// approve: no expanding character outside single quotes, and none of $,
// backticks or backslashes inside double quotes, where the shell still
// expands them. zsh, the default shell, and sh agree on both. Anything
// else asks.
func plain(cmd string) bool {
	var quote rune
	for _, c := range cmd {
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case quote == '"':
			if c == '"' {
				quote = 0
			} else if strings.ContainsRune("$`\\", c) {
				return false
			}
		case c == '\'' || c == '"':
			quote = c
		case strings.ContainsRune(expanding, c):
			return false
		}
	}
	return quote == 0
}

func normalize(cmd string) string { return strings.Join(strings.Fields(cmd), " ") }

var blanks = regexp.MustCompile(`[ \t]+`)

// exactForm collapses runs of spaces and tabs but keeps newlines, which the
// shell treats as command separators.
func exactForm(cmd string) string { return strings.TrimSpace(blanks.ReplaceAllString(cmd, " ")) }

// wrappers run the command that follows them.
var wrappers = []string{"env", "command", "exec", "nohup", "time", "builtin", "nice", "sudo", "doas"}

var shellC = regexp.MustCompile(`^(?:\S*/)?(?:sh|bash|zsh|dash|ksh)\s+-[a-z]*c[a-z]*\s+(.*)$`)

// simpleCommands returns the commands a line runs, unwrapped for matching
// deny rules: each segment between operators, without enclosing
// parentheses, braces or quotes, leading assignments, wrapper commands, a
// leading backslash or a path on the program; sh -c strings are unwrapped
// recursively.
func simpleCommands(cmd string, depth int) []string {
	var out []string
	for _, seg := range segments(cmd) {
		seg = strings.Trim(seg, "(){} \t'\"")
		fields := strings.Fields(seg)
		for len(fields) > 0 {
			f := fields[0]
			switch {
			case strings.Contains(f, "=") && !strings.HasPrefix(f, "="):
				fields = fields[1:] // VAR=value
			case slices.Contains(wrappers, strings.TrimPrefix(f, "\\")):
				fields = fields[1:]
				for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
					fields = fields[1:] // wrapper options such as env -i
				}
			default:
				goto done
			}
		}
	done:
		if len(fields) == 0 {
			continue
		}
		fields[0] = filepath.Base(strings.TrimPrefix(fields[0], "\\"))
		simple := strings.Join(fields, " ")
		out = append(out, simple)
		if g := withoutGitOptions(fields); g != "" {
			out = append(out, g)
		}
		if m := shellC.FindStringSubmatch(simple); m != nil && depth < 3 {
			out = append(out, simpleCommands(strings.Trim(m[1], "'\""), depth+1)...)
		}
	}
	return out
}

// gitValueOptions are git's global options that take a separate value.
var gitValueOptions = []string{"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--config-env", "--super-prefix", "--attr-source", "--exec-path"}

// withoutGitOptions returns a git command without the global options before
// its subcommand, so a deny rule for "git push *" also matches
// "git -C . push" and "git -c a=b --no-pager push"; "" when fields is not a
// git command with such options.
func withoutGitOptions(fields []string) string {
	if fields[0] != "git" || len(fields) < 2 || !strings.HasPrefix(fields[1], "-") {
		return ""
	}
	rest := fields[1:]
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		if slices.Contains(gitValueOptions, rest[0]) && len(rest) > 1 && rest[0] != "--exec-path" {
			rest = rest[2:]
			continue
		}
		rest = rest[1:]
	}
	return strings.Join(append([]string{"git"}, rest...), " ")
}

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

// matchCommand: a deny rule matches if any command the line runs matches.
// An exact or literal allow rule matches exactly the line it names, as
// written, operators included, since that is what the user approved; a
// prefix allow rule matches only a single plain command.
func (r Rule) matchCommand(cmd string) bool {
	if r.Effect == Deny {
		if r.Literal {
			return exactForm(cmd) == exactForm(r.Command)
		}
		return slices.ContainsFunc(simpleCommands(cmd, 0), func(s string) bool { return commandMatches(r.Command, s) })
	}
	pattern := normalize(r.Command)
	if r.Literal || (pattern != "*" && !strings.HasSuffix(pattern, " *")) {
		return exactForm(cmd) == exactForm(r.Command)
	}
	segs := segments(cmd)
	if len(segs) != 1 || !plain(cmd) {
		return false
	}
	return commandMatches(r.Command, segs[0])
}

func (r Rule) matchPath(root, p string) bool {
	pattern := r.Path
	if r.Literal {
		if filepath.IsAbs(pattern) {
			return strings.EqualFold(filepath.Clean(pattern), p)
		}
		for _, rt := range canonical(root) {
			if strings.EqualFold(filepath.Join(rt, filepath.FromSlash(pattern)), p) {
				return true
			}
		}
		return false
	}
	// Globs ignore case, as macOS volumes do, so SECRETS/key is secrets/key.
	if filepath.IsAbs(pattern) {
		name := strings.ToLower(strings.TrimPrefix(filepath.ToSlash(p), "/"))
		for _, pat := range absSpellings(pattern) {
			if environment.MatchGlob(strings.ToLower(strings.TrimPrefix(filepath.ToSlash(pat), "/")), name) {
				return true
			}
		}
		return false
	}
	for _, rt := range canonical(root) {
		if within(p, rt) {
			rel := strings.TrimPrefix(p[len(rt):], string(filepath.Separator))
			return environment.MatchGlob(strings.ToLower(pattern), strings.ToLower(filepath.ToSlash(rel)))
		}
	}
	return false
}

// absSpellings returns an absolute pattern under each spelling of its
// folders without globs, so /tmp/** also matches the resolved
// /private/tmp/... paths reach the broker as.
func absSpellings(pattern string) []string {
	parts := strings.Split(filepath.ToSlash(pattern), "/")
	i := slices.IndexFunc(parts, func(s string) bool { return strings.ContainsAny(s, "*?[") })
	if i < 0 {
		i = len(parts)
	}
	prefix, rest := strings.Join(parts[:i], "/"), strings.Join(parts[i:], "/")
	if prefix == "" {
		return []string{pattern}
	}
	var out []string
	for _, sp := range canonical(filepath.FromSlash(prefix)) {
		out = append(out, path.Join(filepath.ToSlash(sp), rest))
	}
	return append(out, pattern)
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

// RuleFor is the rule "Allow always" creates for req: the literal command,
// or the literal path (relative to root when inside it), or the tool alone
// for a tool that names neither. It reports false for a request one rule
// cannot narrow, several paths or a command it does not show: a rule for
// the whole tool would allow far more than the user saw.
func RuleFor(req Request, root string, scope Scope) (Rule, bool) {
	r := Rule{Effect: Allow, Tool: req.Tool, Scope: scope}
	switch {
	case req.Command != "":
		r.Command, r.Literal = exactForm(req.Command), true
	case len(req.Paths) == 1:
		r.Path, r.Literal = req.Paths[0], true
		for _, rt := range canonical(root) {
			if rel, err := filepath.Rel(rt, req.Paths[0]); err == nil && !strings.HasPrefix(rel, "..") {
				r.Path = filepath.ToSlash(rel)
				break
			}
		}
	case len(req.Paths) > 1, req.Kind == KindExecute:
		return r, false
	}
	return r, true
}

// RuleSource supplies the rules in force for a session, every scope
// together. An error means some rules could not be read; the broker then
// fails closed.
type RuleSource interface {
	Rules(sessionID string) ([]Rule, error)
}

// RuleSink stores a rule created by "Allow always".
type RuleSink interface {
	AddRule(sessionID string, r Rule) error
}
