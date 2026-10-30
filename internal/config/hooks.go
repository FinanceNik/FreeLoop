package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Hooks (EXT-005) are shell commands the app runs at points of a session's
// life, with the event as JSON on stdin. They come from config.yaml (every
// session), a profile (its sessions) and a trusted project's
// .freeloop/hooks.yaml (its sessions), in that order.
//
// The exit code decides: 0 continues; 2 blocks, with standard error as the
// reason (a pre_tool_use or permission_request hook denies the call, a
// session_start hook stops the session from starting); any other code, or
// a timeout, is reported in the transcript and the session continues. A
// hook can block but never approve: approval stays with the rules, the mode
// and the user (SEC-003).
type Hook struct {
	Event string `yaml:"event" json:"event"`
	// Match is a glob on the tool name (pre_tool_use, post_tool_use,
	// permission_request); empty matches every tool.
	Match   string `yaml:"match,omitempty" json:"match"`
	Command string `yaml:"command" json:"command"`
	// Timeout in seconds; DefaultHookTimeout when 0.
	Timeout int `yaml:"timeout,omitempty" json:"timeout"`
	// Source is where the hook came from, for the transcript.
	Source string `yaml:"-" json:"source"`
}

// Hook events.
const (
	HookSessionStart      = "session_start"
	HookPreToolUse        = "pre_tool_use"
	HookPostToolUse       = "post_tool_use"
	HookPermissionRequest = "permission_request"
)

// HookEvents lists the events in the order a tool call meets them.
var HookEvents = []string{HookSessionStart, HookPreToolUse, HookPermissionRequest, HookPostToolUse}

// DefaultHookTimeout and MaxHookTimeout bound a hook, in seconds.
const (
	DefaultHookTimeout = 30
	MaxHookTimeout     = 600
)

// HooksFileName is a project's hooks file in .freeloop/.
const HooksFileName = "hooks.yaml"

// Validate reports the first setting that cannot work.
func (h Hook) Validate() error {
	switch {
	case !slices.Contains(HookEvents, h.Event):
		return fmt.Errorf("event %q must be one of %s", h.Event, strings.Join(HookEvents, ", "))
	case strings.TrimSpace(h.Command) == "":
		return errors.New("a hook needs a command")
	case h.Timeout < 0 || h.Timeout > MaxHookTimeout:
		return fmt.Errorf("timeout %d must be between 1 and %d seconds", h.Timeout, MaxHookTimeout)
	case h.Match != "" && h.Event == HookSessionStart:
		return errors.New("a session_start hook has no tool to match")
	}
	if _, err := path.Match(h.Match, ""); err != nil {
		return fmt.Errorf("match %q is not a valid pattern", h.Match)
	}
	return nil
}

// Matches reports whether the hook runs for event and tool.
func (h Hook) Matches(event, tool string) bool {
	if h.Event != event {
		return false
	}
	if h.Match == "" {
		return true
	}
	ok, _ := path.Match(h.Match, tool)
	return ok
}

// validateHooks drops hooks that cannot work and reports them.
func validateHooks(hooks []Hook, field string) ([]Hook, []Issue) {
	var issues []Issue
	out := hooks[:0:0]
	for i, h := range hooks {
		if err := h.Validate(); err != nil {
			issues = append(issues, Issue{Field: fmt.Sprintf("%s[%d]", field, i), Message: err.Error()})
			continue
		}
		out = append(out, h)
	}
	return out, issues
}

type hooksFile struct {
	Version int    `yaml:"version"`
	Hooks   []Hook `yaml:"hooks"`
}

// ParseHooks reads a project's .freeloop/hooks.yaml; hooks with errors are
// left out and reported.
func ParseHooks(b []byte, source string) ([]Hook, []Issue) {
	var f hooksFile
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, []Issue{{File: source, Line: yamlErrorLine(err), Message: err.Error()}}
	}
	hooks, issues := validateHooks(f.Hooks, "hooks")
	for i := range issues {
		issues[i].File = source
		issues[i].Line = fieldLine(b, issues[i].Field)
	}
	for i := range hooks {
		hooks[i].Source = source
	}
	return hooks, issues
}
