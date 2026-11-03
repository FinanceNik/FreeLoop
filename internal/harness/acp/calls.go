package acp

import (
	"context"
	"encoding/json"
	"strings"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Token use per model call, for an agent built on the Claude Agent SDK (PRV-008,
// GUI-023). What a prompt's answer says about usage is not enough there: it comes
// only when the turn ends, which can be half an hour on, and it leaves out what the
// agent does after a background task finishes, 97% of the output of one 37-minute
// turn. So the adapter is asked for the SDK's own messages, and only their token
// counts are read.
//
// A stream event holds a call's real count: its message_delta matches what the SDK
// totals. A subagent's calls come only as assistant messages, whose input and cache
// counts are right and whose output stopped early, and not every one comes at all.
// A result's running total per model holds the rest, which is added as an adjustment:
// those subagent calls, and calls of the agent's own that no message shows. With it
// the tokens and the prices give the cost the agent reports, to the digit. The total
// is of the agent's process, also after session/load (checked with adapter 0.99.1;
// look again when the preset's version changes, since history in it would count twice).

const (
	sdkMessageMethod = "_claude/sdkMessage"
	// syntheticModel marks a banner the agent's program wrote itself.
	syntheticModel = "<synthetic>"
	// maxSeenCalls bounds what a long session remembers; an assistant message
	// follows its call's stream events closely.
	maxSeenCalls = 4096
)

// sdkMessageFilter is the messages asked for. A filter goes by type only, so text
// deltas come along, and are dropped unread: a handler that took long would overflow
// the connection's queue.
func sdkMessageFilter() []map[string]any {
	return []map[string]any{{"type": "stream_event"}, {"type": "assistant"}, {"type": "result"}, {"type": "rate_limit_event"}}
}

type sdkUsage struct {
	Input      *int64 `json:"input_tokens"`
	Output     *int64 `json:"output_tokens"`
	CacheRead  *int64 `json:"cache_read_input_tokens"`
	CacheWrite *int64 `json:"cache_creation_input_tokens"`
	// CacheCreation splits CacheWrite by how long the writes last.
	CacheCreation *struct {
		Hour *int64 `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// applyTo takes the counts this reading has: they are running totals of the call.
func (u sdkUsage) applyTo(d *event.UsageData) {
	set := func(to *int64, from *int64) {
		if from != nil {
			*to = *from
		}
	}
	set(&d.InputTokens, u.Input)
	set(&d.OutputTokens, u.Output)
	set(&d.CacheReadTokens, u.CacheRead)
	set(&d.CacheWriteTokens, u.CacheWrite)
	if u.CacheCreation != nil {
		set(&d.CacheWrite1hTokens, u.CacheCreation.Hour)
	}
}

type sdkModelMessage struct {
	ID    string   `json:"id"`
	Model string   `json:"model"`
	Usage sdkUsage `json:"usage"`
}

type sdkMessage struct {
	Type    string          `json:"type"`
	Parent  *string         `json:"parent_tool_use_id"`
	Message sdkModelMessage `json:"message"`
	Event   struct {
		Type    string          `json:"type"`
		Message sdkModelMessage `json:"message"`
		Usage   sdkUsage        `json:"usage"`
	} `json:"event"`
	// RateLimit is a rate_limit_event's report (limits.go).
	RateLimit *sdkRateLimit `json:"rate_limit_info"`
	// ModelUsage is a result's running total for the agent's process, per model.
	ModelUsage map[string]struct {
		Input      int64 `json:"inputTokens"`
		Output     int64 `json:"outputTokens"`
		CacheRead  int64 `json:"cacheReadInputTokens"`
		CacheWrite int64 `json:"cacheCreationInputTokens"`
		Write1h    int64 `json:"cacheCreationEphemeral1hInputTokens"`
	} `json:"modelUsage"`
}

// callTokens is a sum of a model's tokens.
type callTokens struct{ input, output, read, write, write1h int64 }

func (a callTokens) plus(b callTokens) callTokens {
	return callTokens{a.input + b.input, a.output + b.output, a.read + b.read, a.write + b.write, a.write1h + b.write1h}
}

// above is what a has more of than b, kind by kind.
func (a callTokens) above(b callTokens) callTokens {
	return callTokens{max(0, a.input-b.input), max(0, a.output-b.output), max(0, a.read-b.read), max(0, a.write-b.write), max(0, a.write1h-b.write1h)}
}

// callLedger is what a session has learned from the agent's messages.
type callLedger struct {
	// open is the call being streamed, seen those already counted.
	open *event.UsageData
	seen map[string]bool
	// reported says the session's usage comes from its calls.
	reported bool
	// recorded is what was recorded per model.
	recorded map[string]callTokens
}

// HandleExtensionMethod implements sdk.ExtensionMethodHandler.
func (s *session) HandleExtensionMethod(_ context.Context, method string, params json.RawMessage) (any, error) {
	if method != sdkMessageMethod {
		return nil, sdk.NewMethodNotFound(method)
	}
	s.mu.Lock()
	loading := s.loading
	s.mu.Unlock()
	if loading || s.browsing {
		return nil, nil
	}
	var p struct {
		Message sdkMessage `json:"message"`
	}
	// One that cannot be read is skipped: usage is not worth ending a session for.
	if json.Unmarshal(params, &p) == nil {
		s.countCall(p.Message)
		s.reportLimits(p.Message.RateLimit)
	}
	return nil, nil
}

// countCall follows the model calls in the agent's messages and records each once.
func (s *session) countCall(m sdkMessage) {
	if m.Type == "result" {
		s.reconcile(m)
		return
	}
	var done *event.UsageData
	s.mu.Lock()
	c := &s.calls
	switch {
	case m.Type == "stream_event" && m.Parent == nil:
		switch m.Event.Type {
		case "message_start":
			c.remember(m.Event.Message.ID)
			done, c.open = c.open, startCall(m.Event.Message)
		case "message_delta":
			if c.open != nil {
				m.Event.Usage.applyTo(c.open)
			}
		case "message_stop":
			done, c.open = c.open, nil
		}
	case m.Type == "assistant" && m.Message.Model != syntheticModel && c.remember(m.Message.ID):
		done = startCall(m.Message)
	}
	s.mu.Unlock()
	s.reportCall(done)
}

// reconcile adds what a result counts and the calls did not show.
func (s *session) reconcile(m sdkMessage) {
	var adjust []event.UsageData
	s.mu.Lock()
	c := &s.calls
	for key, u := range m.ModelUsage {
		model, _, _ := strings.Cut(key, "[") // claude-sonnet-5[1m]
		missing := callTokens{u.Input, u.Output, u.CacheRead, u.CacheWrite, u.Write1h}.above(c.recorded[model])
		if missing != (callTokens{}) {
			c.add(model, missing)
			adjust = append(adjust, event.UsageData{InputTokens: missing.input, OutputTokens: missing.output, CacheReadTokens: missing.read,
				CacheWriteTokens: missing.write, CacheWrite1hTokens: missing.write1h, Model: model, Adjustment: true})
		}
	}
	s.mu.Unlock()
	for _, d := range adjust {
		s.p.Emit(event.UsageUpdate, d)
	}
}

func (c *callLedger) add(model string, t callTokens) {
	if c.recorded == nil {
		c.recorded = map[string]callTokens{}
	}
	c.recorded[model] = c.recorded[model].plus(t)
}

// remember reports whether a call is new.
func (c *callLedger) remember(id string) bool {
	if c.seen[id] {
		return false
	}
	if c.seen == nil || len(c.seen) >= maxSeenCalls {
		c.seen = map[string]bool{}
	}
	c.seen[id] = true
	return true
}

func startCall(m sdkModelMessage) *event.UsageData {
	d := &event.UsageData{Model: m.Model}
	m.Usage.applyTo(d)
	return d
}

// reportCall records a finished call. A call with no tokens never reached the model.
func (s *session) reportCall(d *event.UsageData) {
	if d == nil || d.InputTokens+d.OutputTokens+d.CacheReadTokens+d.CacheWriteTokens == 0 {
		return
	}
	s.mu.Lock()
	s.calls.reported = true
	s.calls.add(d.Model, callTokens{d.InputTokens, d.OutputTokens, d.CacheReadTokens, d.CacheWriteTokens, d.CacheWrite1hTokens})
	s.mu.Unlock()
	s.p.Emit(event.UsageUpdate, *d)
}

// endCalls records a call a stopped turn cut off, whose input was spent, and reports
// whether the session's usage comes from its calls: then the turn's own would count
// it all twice.
func (s *session) endCalls() bool {
	s.mu.Lock()
	open := s.calls.open
	s.calls.open = nil
	s.mu.Unlock()
	s.reportCall(open)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls.reported
}
