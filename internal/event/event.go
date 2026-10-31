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
	// Checkpoint records a snapshot of the session's files at the start of
	// a turn; Rewind restores one (SES-014).
	Checkpoint Type = "checkpoint"
	Rewind     Type = "rewind"
	// Handoff starts a session that continues another with a different
	// profile or harness (HRN-012).
	Handoff Type = "handoff"
	// PullRequest records a pull request opened from the session (SES-011).
	PullRequest Type = "pull_request"
	// TagsChanged sets a session's tags (SES-013).
	TagsChanged Type = "tags_changed"
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
	GitState, ModelChanged, EnvHealth, HookRan, Error, HistoryMessage, Checkpoint, Rewind, Handoff,
	PullRequest, TagsChanged,
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
	// prompt. An image block (SES-015) names the session's stored picture
	// as attachment:<name>, in place of ACP's inline data.
	URI      string `json:"uri,omitempty"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// AttachmentURI is the uri of an image block for a stored attachment.
func AttachmentURI(name string) string { return "attachment:" + name }

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
	// ContextTokens is the size of the context this call sent, and
	// ContextWindow the model's, for the context meter (HRN-013).
	ContextTokens int64 `json:"contextTokens,omitempty"`
	ContextWindow int   `json:"contextWindow,omitempty"`
	// ProviderID and Model made the call, for cost estimates (PRV-008);
	// empty when the harness does not say (an external agent).
	ProviderID string `json:"providerId,omitempty"`
	Model      string `json:"model,omitempty"`
}

// CompactionData is the payload of compaction_update (HRN-013): status
// started or completed, why (auto or manual), the context size before and
// an estimate after, and the summary the session continues from.
type CompactionData struct {
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	ContextTokens int64  `json:"contextTokens"`
	AfterTokens   int64  `json:"afterTokens,omitempty"`
	Summary       string `json:"summary,omitempty"`
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

// CheckpointData is the payload of checkpoint: the snapshot of the
// session's files before a turn and, for the built-in harness, how many
// context messages the session had then.
type CheckpointData struct {
	Commit     string `json:"commit"`
	HistoryLen int    `json:"historyLen"`
}

// RewindData is the payload of rewind: the checkpoint event restored, and
// the snapshot taken of the files just before, which a later rewind can
// return to.
type RewindData struct {
	CheckpointSeq int64  `json:"checkpointSeq"`
	Commit        string `json:"commit"`
	Saved         string `json:"saved"`
	// Context is false when the harness cannot rewind its context, as for
	// an external agent.
	Context bool `json:"context"`
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

// PullRequestData is the payload of pull_request: where the pull request
// is and what it proposes.
type PullRequestData struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Base  string `json:"base"`
	Head  string `json:"head"`
	Draft bool   `json:"draft,omitempty"`
}

// TagsData is the payload of tags_changed: the session's tags from now on.
type TagsData struct {
	Tags []string `json:"tags"`
}

// HandoffData is the payload of handoff: the session continued from and
// what the new one is told about it, a model's summary (Generated) or the
// end of its transcript.
type HandoffData struct {
	FromSessionID string `json:"fromSessionId"`
	FromTitle     string `json:"fromTitle"`
	Summary       string `json:"summary"`
	Generated     bool   `json:"generated"`
}
