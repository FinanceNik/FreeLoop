// Package event defines the versioned session event schema (EXT-002).
//
// The schema is a strict superset of ACP's session/update variants (OQ-012):
// an ACP update maps to an Event with the same type name and the update object,
// minus its sessionUpdate discriminator, as Data. Payload field names follow
// ACP's camelCase. The JSON Schema lives in schemas/events.json and a test keeps
// the two in sync.
package event

import (
	"encoding/json"
	"time"
)

// Version is the schema version written into every event's v field.
const Version = 1

// Type names an event variant.
type Type string

// Variants taken one-to-one from ACP's session/update.
const (
	UserMessageChunk        Type = "user_message_chunk"
	AgentMessageChunk       Type = "agent_message_chunk"
	AgentThoughtChunk       Type = "agent_thought_chunk"
	ToolCall                Type = "tool_call"
	ToolCallUpdate          Type = "tool_call_update"
	Plan                    Type = "plan"
	AvailableCommandsUpdate Type = "available_commands_update"
	CurrentModeUpdate       Type = "current_mode_update"
	ConfigOptionUpdate      Type = "config_option_update"
	SessionInfoUpdate       Type = "session_info_update"
	UsageUpdate             Type = "usage_update"
	Notice                  Type = "notice"
	CompactionUpdate        Type = "compaction_update"
	CompactionSummaryChunk  Type = "compaction_summary_chunk"
)

// Variants Freeloop adds on top of ACP.
const (
	TurnStarted         Type = "turn_started"
	TurnEnded           Type = "turn_ended"
	PermissionRequested Type = "permission_requested"
	PermissionDecided   Type = "permission_decided"
	SessionStatus       Type = "session_status"
	GitState            Type = "git_state"
	ModelChanged        Type = "model_changed"
	EnvHealth           Type = "env_health"
	HookRan             Type = "hook_ran"
	Error               Type = "error"
	// HistoryMessage records a built-in harness context message so a
	// session continues with its full context after a restart (SES-003).
	// The UI does not render it.
	HistoryMessage Type = "history_message"
)

// ACPTypes lists the variants that come from ACP, in schema order.
var ACPTypes = []Type{
	UserMessageChunk, AgentMessageChunk, AgentThoughtChunk, ToolCall, ToolCallUpdate,
	Plan, AvailableCommandsUpdate, CurrentModeUpdate, ConfigOptionUpdate, SessionInfoUpdate,
	UsageUpdate, Notice, CompactionUpdate, CompactionSummaryChunk,
}

// FreeloopTypes lists the variants Freeloop adds, in schema order.
var FreeloopTypes = []Type{
	TurnStarted, TurnEnded, PermissionRequested, PermissionDecided, SessionStatus,
	GitState, ModelChanged, EnvHealth, HookRan, Error, HistoryMessage,
}

// AllTypes returns every known variant.
func AllTypes() []Type {
	return append(append([]Type{}, ACPTypes...), FreeloopTypes...)
}

// Valid reports whether t is a known variant.
func (t Type) Valid() bool {
	for _, k := range AllTypes() {
		if k == t {
			return true
		}
	}
	return false
}

// Event is one line of a session transcript.
type Event struct {
	V    int             `json:"v"`
	Seq  int64           `json:"seq"`
	TS   time.Time       `json:"ts"`
	SID  string          `json:"sid"`
	Type Type            `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ContentBlock is ACP's content block. Schema v0 only fixes the text variant.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// URI and Name describe a resource_link, such as a file mentioned in a
	// prompt.
	URI  string `json:"uri,omitempty"`
	Name string `json:"name,omitempty"`
}

// MessageChunk is the payload of the three *_message_chunk / thought variants.
type MessageChunk struct {
	Content ContentBlock `json:"content"`
}

// SessionStatusData is the payload of session_status.
type SessionStatusData struct {
	Status string `json:"status"`
}

// UsageData is the payload of usage_update: one model call's tokens.
type UsageData struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
}

// GitStateData is the payload of git_state (SES-006).
type GitStateData struct {
	Repo   bool   `json:"repo"`
	Branch string `json:"branch"`
	Head   string `json:"head"`
	// Dirty is the number of changed files, -1 when it was not checked.
	Dirty      int    `json:"dirty"`
	BaseCommit string `json:"baseCommit"`
}

// CurrentModeData is the payload of current_mode_update.
type CurrentModeData struct {
	CurrentModeID string `json:"currentModeId"`
}

// TurnEndedData is the payload of turn_ended.
type TurnEndedData struct {
	StopReason string `json:"stopReason"`
}

// NoticeData is the payload of notice: something the user should know
// that is not an error.
type NoticeData struct {
	Message string `json:"message"`
}

// ModelChangedData is the payload of model_changed (PRV-006).
type ModelChangedData struct {
	ProviderID string `json:"providerId"`
	Model      string `json:"model"`
}

// AvailableCommandsData is the payload of available_commands_update: the
// slash commands the session's harness offers, as in ACP.
type AvailableCommandsData struct {
	AvailableCommands []AvailableCommand `json:"availableCommands"`
}

// AvailableCommand is one slash command.
type AvailableCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ErrorData is the payload of error.
type ErrorData struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}
