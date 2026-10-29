// Package harness defines what every harness offers the engine (HRN-001):
// the built-in loop and, from M2, external agents over ACP. The GUI never
// special-cases one; everything it shows comes from the events a session
// emits.
package harness

import (
	"context"
	"encoding/json"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Info describes a harness.
type Info struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"` // builtin | acp
}

// Emitter records an event in the session's transcript; the store persists
// it before the UI sees it.
type Emitter func(typ event.Type, data any)

// Params is what a session runs with.
type Params struct {
	SessionID string
	Env       environment.Environment
	Broker    *permission.Broker
	Provider  provider.Provider
	Model     string
	Effort    string
	Mode      permission.Mode
	// History restores a built-in session's context after a restart.
	History []provider.Message
	// DisabledTools are not offered to the model (the environment's tool
	// list, ENV-002).
	DisabledTools []string
	// Profile settings (HRN-003): text added to the system prompt, the only
	// tools offered when set, a cap on model calls per prompt (the
	// harness's own when 0), and whether project instruction files are
	// left out.
	SystemPrompt   string
	Tools          []string
	MaxTurns       int
	NoInstructions bool
	// Trusted, when set, is asked before every model call; project
	// instruction files are read only while it returns true (SEC-006).
	Trusted func() bool
	// Commands are the profile's slash commands; a prompt that starts with
	// /name runs the command's prompt (GUI-004).
	Commands []Command
	// Skills are the session's enabled skills, asked at every model call
	// so toggles apply at once (SKL-002, SKL-004); nil means none.
	Skills SkillSource
	// MCP are the session's MCP tools (MCP-003), asked at every model call
	// so servers that start or stop are picked up; nil means none.
	MCP MCPSource
	// MCPServers are passed to an external agent on session creation, so
	// it uses the same servers (MCP-004). Secrets are resolved.
	MCPServers []MCPServerSpec
	Emit       Emitter
}

// MCPServerSpec is an MCP server as an external agent receives it.
type MCPServerSpec struct {
	Name      string
	Transport string // stdio | http
	Command   string
	Args      []string
	Env       map[string]string
	URL       string
	Headers   map[string]string
}

// MCPSource lists a session's MCP tools and calls them.
type MCPSource interface {
	Tools() []MCPTool
	// Call runs the tool named as Tools listed it.
	Call(ctx context.Context, name string, input json.RawMessage) (text string, isError bool, err error)
}

// MCPTool is one MCP tool as the model sees it: Name is
// mcp__<server>__<tool>, made safe for every provider.
type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Server      string
	Tool        string
}

// SkillSource lists a session's enabled skills and reads them.
type SkillSource interface {
	Enabled() []SkillEntry
	// Load returns a skill's SKILL.md body, or with file set one of its
	// files, relative to the skill's folder.
	Load(name, file string) (SkillContent, error)
}

// SkillEntry is what the system prompt lists for a skill.
type SkillEntry struct {
	Name        string
	Description string
}

// SkillContent is a loaded skill or skill file.
type SkillContent struct {
	Text string
	// Dir is the skill's absolute folder, for running its scripts; Files
	// lists its other files when the body was loaded.
	Dir   string
	Files []string
}

// Command is a slash command that sends a prepared prompt; text after the
// name is appended to it.
type Command struct {
	Name        string
	Description string
	Prompt      string
}

// Harness creates sessions.
type Harness interface {
	Info() Info
	NewSession(ctx context.Context, p Params) (Session, error)
}

// StopReason says why a prompt ended.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopMaxTokens StopReason = "max_tokens"
	StopMaxTurns  StopReason = "max_turns"
	StopRefusal   StopReason = "refusal"
	StopCancelled StopReason = "cancelled"
	StopError     StopReason = "error"
)

// Prompt is one user message: its text and the files it mentions, which the
// engine reads and embeds the way an ACP client embeds resources (GUI-004).
type Prompt struct {
	Text      string
	Resources []Resource
}

// Resource is a file embedded in a prompt.
type Resource struct {
	// Path is relative to the project root; URI is its file:// form.
	Path string
	URI  string
	Text string
}

// Session is one running conversation.
type Session interface {
	// Prompt runs one user turn to completion. Cancelling ctx stops it.
	Prompt(ctx context.Context, p Prompt) (StopReason, error)
	Mode() permission.Mode
	// SetMode takes effect on the next tool call (GUI-007).
	SetMode(m permission.Mode)
	// Close ends the session and any process it runs.
	Close() error
}
