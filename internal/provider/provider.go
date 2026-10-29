// Package provider defines the model endpoint interface and the
// provider-neutral conversation types the built-in harness keeps its history
// in. Adapters live in subpackages; normalization (tool-call IDs, buffered
// tool arguments, reasoning fields) happens inside each adapter so the
// harness only ever sees these types (PRV-004).
package provider

import (
	"context"
	"encoding/json"
)

// Type is a provider implementation.
type Type string

const (
	TypeAnthropic Type = "anthropic"
	// TypeScripted plays a fixed demo script; development builds only.
	TypeScripted Type = "scripted"
)

// Provider is one configured model endpoint.
type Provider interface {
	ID() string
	Type() Type
	ListModels(ctx context.Context) ([]ModelInfo, error)
	Capabilities(model string) ModelCaps
	// Stream sends a request and returns its events. The channel ends with
	// exactly one event of type EventDone or EventError.
	Stream(ctx context.Context, req ChatRequest) (<-chan Event, error)
}

// ModelInfo describes a model a provider offers.
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// CapsSource says where a model's capabilities came from.
type CapsSource string

const (
	// CapsStatic: the adapter knows the model family.
	CapsStatic CapsSource = "static"
	// CapsReported: the endpoint described the model (Ollama /api/show).
	CapsReported CapsSource = "reported"
	// CapsProbe: a tool-calling probe ran against the model.
	CapsProbe CapsSource = "probe"
	// CapsOverride: the user set them.
	CapsOverride CapsSource = "override"
	// CapsUnknown: nothing is known yet; Tools is false until a probe runs.
	CapsUnknown CapsSource = "unknown"
)

// ModelCaps is what the built-in harness may rely on for a model (PRV-005).
type ModelCaps struct {
	Source            CapsSource `json:"source"`
	Tools             bool       `json:"tools"`
	ParallelTools     bool       `json:"parallelTools"`
	StreamedToolArgs  bool       `json:"streamedToolArgs"`
	StableToolCallIDs bool       `json:"stableToolCallIds"`
	Reasoning         bool       `json:"reasoning"`
	// Effort reports support for the effort levels low through max.
	Effort        bool `json:"effort"`
	PromptCaching bool `json:"promptCaching"`
	ContextWindow int  `json:"contextWindow"`
	MaxOutput     int  `json:"maxOutput"`
}

// Role of a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// PartType names a message part.
type PartType string

const (
	PartText       PartType = "text"
	PartThinking   PartType = "thinking"
	PartToolUse    PartType = "tool_use"
	PartToolResult PartType = "tool_result"
)

// Part is one block of a message. Fields are used per type.
type Part struct {
	Type PartType `json:"type"`
	// Text for text, thinking and tool_result parts.
	Text string `json:"text,omitempty"`
	// Opaque carries provider data that must be sent back unchanged with the
	// same provider, such as a thinking block's signature or redacted
	// thinking. Other providers drop parts they cannot read.
	Opaque   json.RawMessage `json:"opaque,omitempty"`
	Provider Type            `json:"provider,omitempty"`
	// ToolCallID links tool_use and tool_result.
	ToolCallID string          `json:"toolCallId,omitempty"`
	ToolName   string          `json:"toolName,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	IsError    bool            `json:"isError,omitempty"`
}

// Message is one turn of the conversation.
type Message struct {
	Role  Role   `json:"role"`
	Parts []Part `json:"parts"`
}

// ToolCalls returns the tool_use parts of m.
func (m Message) ToolCalls() []Part {
	var out []Part
	for _, p := range m.Parts {
		if p.Type == PartToolUse {
			out = append(out, p)
		}
	}
	return out
}

// ToolSpec is a tool offered to the model.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ChatRequest is one model call.
type ChatRequest struct {
	Model     string
	System    string
	Messages  []Message
	Tools     []ToolSpec
	MaxTokens int
	// Effort is low, medium, high, xhigh or max; ignored when unsupported.
	Effort string
}

// StopReason says why a response ended.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
	StopRefusal   StopReason = "refusal"
)

// Usage is token accounting for one call (PRV-007).
type Usage struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
}

// EventType names a stream event.
type EventType string

const (
	EventTextDelta     EventType = "text_delta"
	EventThinkingDelta EventType = "thinking_delta"
	// EventToolCallStart announces a tool call; its arguments follow as
	// EventToolArgsDelta and are complete only in EventDone's Message.
	EventToolCallStart EventType = "tool_call_start"
	EventToolArgsDelta EventType = "tool_args_delta"
	EventDone          EventType = "done"
	EventError         EventType = "error"
)

// Event is one streamed item.
type Event struct {
	Type       EventType
	Text       string
	ToolCallID string
	ToolName   string
	// Set on EventDone.
	Message    *Message
	StopReason StopReason
	Usage      Usage
	// Set on EventError.
	Err error
}
