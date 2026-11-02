// Package harness defines what every harness offers the engine (HRN-001):
// the built-in loop and external agents over ACP. The GUI never
// special-cases one; everything it shows comes from the events a session
// emits.
package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/web"
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
	// Fallback takes a model call the provider keeps failing with
	// retryable errors (PRV-010); nil for none.
	Fallback *Fallback
	// ContextWindow is the model's context size in tokens; 0 when unknown,
	// which turns automatic compaction off (HRN-013).
	ContextWindow int
	Effort        string
	Mode          permission.Mode
	// History restores a built-in session's context after a restart.
	History []provider.Message
	// Attachment reads one of the session's stored images, for history
	// that names images restored without their data (SES-015).
	Attachment func(name string) ([]byte, error)
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
	// SkillPlugin is a folder, as the session's environment sees it, laid
	// out as a Claude Code plugin with the session's enabled skills, for an
	// external agent whose preset takes one (SKL-008); "" for none.
	SkillPlugin string
	// MCP are the session's MCP tools (MCP-003), asked at every model call
	// so servers that start or stop are picked up; nil means none.
	MCP MCPSource
	// Subagents runs the task tool's child agents (HRN-014); nil offers no
	// task tool, as for a subagent itself.
	Subagents Subagents
	// Web backs the web_fetch and web_search tools (HRN-017); nil offers
	// neither.
	Web WebSource
	// Optimizer rewrites the shell tool's approved commands so their output
	// costs fewer tokens (HRN-019); nil for none.
	Optimizer CommandOptimizer
	// MCPServers are passed to an external agent on session creation, so
	// it uses the same servers (MCP-004). Secrets are resolved.
	MCPServers []MCPServerSpec
	// ExternalID, when set, is the harness-side session to load instead of
	// starting a new one (SES-003).
	ExternalID string
	Emit       Emitter
}

// CommandOptimizer is asked about every shell command the broker has approved,
// so switching it applies from the next command (HRN-019).
type CommandOptimizer interface {
	Rewrite(ctx context.Context, command string) Rewrite
}

// Rewrite is what to run for an approved command. Via names the optimizer when
// Command differs from what was approved; it is empty when the command stays
// as it is. Env is added to the command either way, while an optimizer is on.
type Rewrite struct {
	Command string
	Via     string
	Env     map[string]string
}

// WebSource reads the web for the built-in harness (HRN-017). The broker
// has approved a call before it gets here.
type WebSource interface {
	// Fetch returns a public page as text.
	Fetch(ctx context.Context, url string) (WebPage, error)
	// CanSearch reports whether a search endpoint is configured.
	CanSearch() bool
	Search(ctx context.Context, query string) ([]web.Result, error)
}

// WebPage is a fetched page.
type WebPage struct {
	URL       string
	Title     string
	Text      string
	Truncated bool
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

// Resumable is implemented by sessions with a harness-side ID the engine
// keeps for loading them after a restart (SES-003).
type Resumable interface {
	ExternalID() string
}

// ModelSetter is implemented by sessions whose model can change between
// turns (PRV-006): the built-in harness's.
type ModelSetter interface {
	SetModel(p provider.Provider, model string, contextWindow int)
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
	// Images are pictures the user attached (SES-015), with their data;
	// the engine has stored each as an attachment of the session.
	Images []provider.Image
}

// Subagents runs a child agent on a task and returns its final reply and
// the session that holds its transcript.
type Subagents interface {
	Run(ctx context.Context, description, prompt string) (SubagentResult, error)
}

// SubagentResult is a finished subagent.
type SubagentResult struct {
	SessionID string
	Text      string
}

// Fallback is the model a call goes to when the session's model keeps
// failing with retryable errors (PRV-010).
type Fallback struct {
	Provider provider.Provider
	Model    string
}

// Resource is a file, or an MCP server's resource, embedded in a prompt.
type Resource struct {
	// Path is relative to the project root, or a label for an MCP
	// resource; URI is the file's file:// form or the resource's URI.
	Path string
	URI  string
	Text string
	// Server is the MCP server an attached resource came from (MCP-007).
	Server string
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

// ResourceText is a resource as text for a model: a file by its path, an
// MCP resource by its server and URI.
func ResourceText(r Resource) string {
	if r.Server != "" {
		return fmt.Sprintf("<resource server=%q uri=%q>\n%s\n</resource>", r.Server, r.URI, r.Text)
	}
	if !strings.HasPrefix(r.URI, "file://") {
		return fmt.Sprintf("<resource name=%q uri=%q>\n%s\n</resource>", r.Path, r.URI, r.Text)
	}
	return fmt.Sprintf("<file path=%q>\n%s\n</file>", r.Path, r.Text)
}

type toolCallKey struct{}

// WithToolCall marks ctx as running the tool call id, so what the call
// asks of the user (an MCP server's input request, MCP-008) shows on it.
func WithToolCall(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolCallKey{}, id)
}

// ToolCall returns the tool call ctx runs, or "".
func ToolCall(ctx context.Context) string {
	id, _ := ctx.Value(toolCallKey{}).(string)
	return id
}

// ErrSignIn marks a prompt an external agent refused because it is not
// signed in to its own account; prompts queued behind it would fail the
// same way.
var ErrSignIn = errors.New("the agent is not signed in")
