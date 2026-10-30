package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/FinanceNik/Freeloop/internal/permission"
)

// ProfilesDir holds one YAML file per harness profile (HRN-003).
const ProfilesDir = "profiles"

// DefaultProfileID names the profile sessions use when none is chosen. A
// profiles/default.yaml replaces the built-in one.
const DefaultProfileID = "default"

// Profile is a declarative harness profile: which harness and model a
// session runs with, what the model is told and may use, and the session's
// permission rules and slash commands. Empty fields fall back to the
// new-session form, the environment and the harness's defaults.
type Profile struct {
	ID          string `yaml:"-" json:"id"` // the file name without .yaml
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description"`
	// Harness is builtin; external harnesses arrive with HRN-005.
	Harness  string `yaml:"harness,omitempty" json:"harness"`
	Provider string `yaml:"provider,omitempty" json:"provider"`
	Model    string `yaml:"model,omitempty" json:"model"`
	// Effort is low, medium, high, xhigh or max; high when empty.
	Effort string `yaml:"effort,omitempty" json:"effort"`
	// Mode is the permission mode new sessions start in.
	Mode string `yaml:"mode,omitempty" json:"mode"`
	// SystemPrompt is added to the harness's own system prompt.
	SystemPrompt string `yaml:"systemPrompt,omitempty" json:"systemPrompt"`
	// Tools, when set, are the only built-in tools offered to the model.
	Tools []string `yaml:"tools,omitempty" json:"tools"`
	// Rules apply to every session of the profile, at profile scope.
	Rules []permission.Rule `yaml:"rules,omitempty" json:"rules"`
	// MaxTurns caps model calls per prompt; the harness's limit when 0.
	MaxTurns int `yaml:"maxTurns,omitempty" json:"maxTurns"`
	// Instructions is project (AGENTS.md, then CLAUDE.md; the default) or
	// none.
	Instructions string `yaml:"instructions,omitempty" json:"instructions"`
	// Skills, when set, are the only skills the profile's sessions see
	// (SKL-002).
	Skills []string `yaml:"skills,omitempty" json:"skills"`
	// MCPServers are servers from mcp.yaml the profile's sessions use even
	// when they are not enabled for everyone (MCP-001).
	MCPServers []string `yaml:"mcpServers,omitempty" json:"mcpServers"`
	// Commands are the profile's slash commands (GUI-004).
	Commands []Command `yaml:"commands,omitempty" json:"commands"`
	// Hooks run for the profile's sessions (EXT-005).
	Hooks []Hook `yaml:"hooks,omitempty" json:"hooks"`
	// Fallback takes a model call the profile's model keeps failing with
	// rate limits or server errors (PRV-010).
	Fallback *ModelRef `yaml:"fallback,omitempty" json:"fallback"`
	// Source is the file the profile came from; empty for the built-in
	// default.
	Source string `yaml:"-" json:"source"`
}

// ModelRef names a provider's model.
type ModelRef struct {
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model" json:"model"`
}

// Command is a slash command that sends a prepared prompt.
type Command struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description"`
	Prompt      string `yaml:"prompt" json:"prompt"`
}

// BuiltinDefaultProfile is the profile used when profiles/default.yaml does
// not exist.
func BuiltinDefaultProfile() Profile {
	return Profile{ID: DefaultProfileID, Name: "Default", Description: "The built-in agent with every tool.", Harness: "builtin"}
}

// ReservedCommands are the app's own slash commands; profiles cannot
// redefine them.
var ReservedCommands = []string{"ask", "accept-edits", "plan", "bypass", "mode", "help", "compact"}

var (
	commandName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	efforts     = []string{"low", "medium", "high", "xhigh", "max"}
)

// fieldError is a validation problem in one field.
type fieldError struct {
	field string
	msg   string
}

func (e fieldError) Error() string { return e.field + ": " + e.msg }

func fieldErr(field, format string, args ...any) error {
	return fieldError{field: field, msg: fmt.Sprintf(format, args...)}
}

// Validate reports the first setting that cannot work.
func (p Profile) Validate() error {
	switch {
	case !idPattern.MatchString(p.ID):
		return fieldErr("id", "%q must be lowercase letters, digits and dashes", p.ID)
	case strings.TrimSpace(p.Name) == "":
		return fieldErr("name", "a name is required")
	case p.Harness != "" && !idPattern.MatchString(p.Harness):
		return fieldErr("harness", "%q must be builtin or the ID of a harness preset", p.Harness)
	case p.Mode != "" && (!permission.Mode(p.Mode).Valid() || p.Mode == string(permission.ModeBypass)):
		return fieldErr("mode", "%q must be ask, accept_edits or plan", p.Mode)
	case p.Effort != "" && !slices.Contains(efforts, p.Effort):
		return fieldErr("effort", "%q must be one of %s", p.Effort, strings.Join(efforts, ", "))
	case p.MaxTurns < 0 || p.MaxTurns > 1000:
		return fieldErr("maxTurns", "%d must be between 1 and 1000", p.MaxTurns)
	case p.Instructions != "" && p.Instructions != "project" && p.Instructions != "none":
		return fieldErr("instructions", "%q must be project or none", p.Instructions)
	}
	for i, r := range p.Rules {
		if err := r.Validate(); err != nil {
			return fieldErr(fmt.Sprintf("rules[%d]", i), "%v", err)
		}
	}
	if f := p.Fallback; f != nil && (strings.TrimSpace(f.Provider) == "" || strings.TrimSpace(f.Model) == "") {
		return fieldErr("fallback", "a fallback needs a provider and a model")
	}
	for i, h := range p.Hooks {
		if err := h.Validate(); err != nil {
			return fieldErr(fmt.Sprintf("hooks[%d]", i), "%v", err)
		}
	}
	seen := map[string]bool{}
	for i, c := range p.Commands {
		field := fmt.Sprintf("commands[%d]", i)
		switch {
		case !commandName.MatchString(c.Name):
			return fieldErr(field, "name %q must start with a letter and use lowercase letters, digits and dashes", c.Name)
		case slices.Contains(ReservedCommands, c.Name):
			return fieldErr(field, "/%s is one of the app's own commands", c.Name)
		case seen[c.Name]:
			return fieldErr(field, "/%s is defined twice", c.Name)
		case strings.TrimSpace(c.Prompt) == "":
			return fieldErr(field, "/%s needs a prompt", c.Name)
		}
		seen[c.Name] = true
	}
	return nil
}

// ValidateProject reports what a project's own profile may not do: project
// configuration can tighten permissions but never loosen them (SET-002), so
// it cannot start sessions in a mode that asks less than ask, or carry allow
// rules.
func (p Profile) ValidateProject() error {
	if p.Mode != "" && p.Mode != string(permission.ModeAsk) && p.Mode != string(permission.ModePlan) {
		return fieldErr("mode", "a project profile can start in ask or plan only, not %s", p.Mode)
	}
	for i, r := range p.Rules {
		if r.Effect != permission.Deny {
			return fieldErr(fmt.Sprintf("rules[%d]", i), "a project profile can only add deny rules")
		}
	}
	return nil
}

// ParseProjectProfile is ParseProfile for a project's .freeloop/profiles/
// file, which also must pass ValidateProject. Its ID gets the project
// prefix.
func ParseProjectProfile(id string, b []byte, tools []string, source string) (Profile, *Issue) {
	p, issue := ParseProfile(id, b, tools)
	if issue == nil {
		if err := p.ValidateProject(); err != nil {
			var fe fieldError
			errors.As(err, &fe)
			issue = &Issue{Field: fe.field, Line: fieldLine(b, fe.field), Message: fe.msg}
		}
	}
	if issue != nil {
		issue.File = source
		return p, issue
	}
	p.ID = ProjectProfilePrefix + id
	p.Source = source
	for i := range p.Rules {
		p.Rules[i].Scope = permission.ScopeProject
		p.Rules[i].Source = source
	}
	return p, nil
}

// ProjectProfilePrefix marks the ID of a profile from a project's
// .freeloop/profiles/.
const ProjectProfilePrefix = "project:"

func profilesDir(dir string) string { return filepath.Join(dir, ProfilesDir) }

// LoadProfiles reads every profiles/*.yaml in dir, sorted by ID. Files with
// problems are skipped and reported with the line of the field at fault.
// tools are the tool names a profile may list.
func LoadProfiles(dir string, tools []string) ([]Profile, []Issue, error) {
	entries, err := os.ReadDir(profilesDir(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	var out []Profile
	var issues []Issue
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name(), ".yaml")
		if !ok || de.IsDir() {
			continue
		}
		path := filepath.Join(profilesDir(dir), de.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		p, issue := ParseProfile(id, b, tools)
		if issue != nil {
			issue.File = path
			issues = append(issues, *issue)
			continue
		}
		p.Source = path
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Profile) int { return strings.Compare(a.ID, b.ID) })
	return out, issues, nil
}

// ParseProfile decodes and validates one profile file. Unknown fields and
// tool names are errors, so a misspelling does not silently do nothing.
func ParseProfile(id string, b []byte, tools []string) (Profile, *Issue) {
	var p Profile
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil && !errors.Is(err, io.EOF) {
		return p, &Issue{Line: yamlErrorLine(err), Message: yamlMessage(err)}
	}
	p.ID = id
	if p.Harness == "" {
		p.Harness = "builtin"
	}
	for i := range p.Rules {
		p.Rules[i].Scope = permission.ScopeProfile
		p.Rules[i].Source = filepath.Join(ProfilesDir, id+".yaml")
	}
	err := p.Validate()
	for i, t := range p.Tools {
		if err == nil && !slices.Contains(tools, t) {
			err = fieldErr(fmt.Sprintf("tools[%d]", i), "unknown tool %q; the built-in tools are %s", t, strings.Join(tools, ", "))
		}
	}
	if err != nil {
		is := &Issue{Message: err.Error()}
		var fe fieldError
		if errors.As(err, &fe) {
			is.Field, is.Message = fe.field, fe.msg
			is.Line = fieldLine(b, fe.field)
		}
		return p, is
	}
	return p, nil
}

var lineInError = regexp.MustCompile(`line (\d+)`)

var unknownField = regexp.MustCompile(`field (\S+) not found in type \S+`)

// yamlMessage turns a decoder error into a sentence for the user: the
// first problem, without the decoder's wrapping and Go type names.
func yamlMessage(err error) string {
	msg := strings.TrimPrefix(err.Error(), "yaml: ")
	msg = strings.TrimPrefix(msg, "unmarshal errors:\n")
	msg = strings.TrimSpace(strings.Split(msg, "\n")[0])
	msg = lineInError.ReplaceAllString(msg, "")
	msg = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(msg), ":"))
	return unknownField.ReplaceAllString(msg, `unknown field "$1"`)
}

// yamlErrorLine finds the first line number in a YAML error.
func yamlErrorLine(err error) int {
	m := lineInError.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	n := 0
	fmt.Sscan(m[1], &n)
	return n
}

// fieldLine returns the line of a field such as "mode" or "commands[1]".
func fieldLine(b []byte, field string) int {
	var doc yaml.Node
	if yaml.Unmarshal(b, &doc) != nil || len(doc.Content) == 0 {
		return 0
	}
	root := doc.Content[0]
	key, index, hasIndex := strings.Cut(field, "[")
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != key {
			continue
		}
		val := root.Content[i+1]
		if hasIndex {
			n := 0
			fmt.Sscan(strings.TrimSuffix(index, "]"), &n)
			if val.Kind == yaml.SequenceNode && n < len(val.Content) {
				return val.Content[n].Line
			}
		}
		return root.Content[i].Line
	}
	return 0
}

// SaveProfile writes profiles/<id>.yaml.
func SaveProfile(dir string, p Profile) error {
	if p.Harness == "" {
		p.Harness = "builtin"
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(profilesDir(dir), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(profilesDir(dir), p.ID+".yaml"), b)
}

// DeleteProfile removes profiles/<id>.yaml.
func DeleteProfile(dir, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("bad profile id %q", id)
	}
	err := os.Remove(filepath.Join(profilesDir(dir), id+".yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ReadProfileFile returns profiles/<id>.yaml as written, comments included;
// "" when the profile has no file.
func ReadProfileFile(dir, id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("bad profile id %q", id)
	}
	b, err := os.ReadFile(filepath.Join(profilesDir(dir), id+".yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

// WriteProfileFile replaces profiles/<id>.yaml with text as written, once
// ParseProfile accepts it, so a hand-written file keeps its comments.
func WriteProfileFile(dir, id, text string, tools []string) error {
	if _, issue := ParseProfile(id, []byte(text), tools); issue != nil {
		if issue.Line > 0 {
			return fmt.Errorf("line %d: %s", issue.Line, issue.Message)
		}
		return errors.New(issue.Message)
	}
	if err := os.MkdirAll(profilesDir(dir), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return writeAtomic(filepath.Join(profilesDir(dir), id+".yaml"), []byte(text))
}
