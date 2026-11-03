// Package anthropic adapts the Anthropic Messages API to provider.Provider.
package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

// DefaultBaseURL is the Anthropic API.
const DefaultBaseURL = "https://api.anthropic.com"

// DefaultModel is offered when the user has not picked one.
const DefaultModel = "claude-opus-5-5"

// Options configures New.
type Options struct {
	ID      string
	BaseURL string
	APIKey  string
	Headers map[string]string
	// HTTPClient is for tests.
	HTTPClient *http.Client
}

// Provider talks to one Anthropic endpoint.
type Provider struct {
	id     string
	client sdk.Client
}

// New returns a provider for opts.
func New(opts Options) *Provider {
	// The harness retries with its own backoff and tells the user
	// (PRV-010); the SDK's retries would stack under it.
	ro := []option.RequestOption{option.WithAPIKey(opts.APIKey), option.WithMaxRetries(0)}
	if opts.BaseURL != "" {
		ro = append(ro, option.WithBaseURL(opts.BaseURL))
	}
	for k, v := range opts.Headers {
		ro = append(ro, option.WithHeader(k, v))
	}
	client := opts.HTTPClient
	if client == nil {
		client = provider.KeyClient()
	}
	ro = append(ro, option.WithHTTPClient(client))
	return &Provider{id: opts.ID, client: sdk.NewClient(ro...)}
}

func (p *Provider) ID() string          { return p.id }
func (p *Provider) Type() provider.Type { return provider.TypeAnthropic }

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	var out []provider.ModelInfo
	iter := p.client.Models.ListAutoPaging(ctx, sdk.ModelListParams{})
	for iter.Next() {
		m := iter.Current()
		out = append(out, provider.ModelInfo{ID: m.ID, DisplayName: m.DisplayName})
	}
	if err := iter.Err(); err != nil {
		return nil, describe(err)
	}
	return out, nil
}

// modern reports whether a model takes adaptive thinking and effort: the
// 4.6 generation and later.
func modern(model string) bool {
	for _, p := range []string{"claude-opus-4-6", "claude-sonnet-4-6", "claude-opus-4-7", "claude-opus-4-8"} {
		if strings.HasPrefix(model, p) {
			return true
		}
	}
	for _, family := range []string{"claude-opus-", "claude-sonnet-", "claude-haiku-", "claude-fable-", "claude-mythos-"} {
		if v, ok := strings.CutPrefix(model, family); ok && len(v) > 0 && v[0] >= '5' && v[0] <= '9' {
			return true
		}
	}
	return false
}

func (p *Provider) Capabilities(model string) provider.ModelCaps {
	c := provider.ModelCaps{
		Source: provider.CapsStatic,
		Tools:  true, ParallelTools: true, StreamedToolArgs: true, StableToolCallIDs: true,
		PromptCaching: true, ContextWindow: 200_000, MaxOutput: 64_000, Images: true,
	}
	if modern(model) {
		c.Reasoning, c.Effort = true, true
		c.ContextWindow, c.MaxOutput = 1_000_000, 128_000
	}
	return c
}

// opaqueThinking is what a thinking part carries back to this provider.
type opaqueThinking struct {
	Signature string `json:"signature,omitempty"`
	Redacted  string `json:"redacted,omitempty"`
}

func (p *Provider) params(req provider.ChatRequest) sdk.MessageNewParams {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 64_000
	}
	params := sdk.MessageNewParams{
		Model:     req.Model,
		MaxTokens: int64(maxTokens),
		Messages:  toParams(req.Messages),
		// Automatic prompt caching on the last cacheable block.
		CacheControl: sdk.NewCacheControlEphemeralParam(),
	}
	if req.System != "" {
		params.System = []sdk.TextBlockParam{{Text: req.System}}
	}
	for _, t := range req.Tools {
		tool := sdk.ToolParam{
			Name:        t.Name,
			Description: sdk.String(t.Description),
			InputSchema: sdk.ToolInputSchemaParam{
				Properties: t.InputSchema["properties"],
			},
			// Arguments stream as generated; the harness validates them
			// before running anything.
			EagerInputStreaming: sdk.Bool(true),
		}
		if req, ok := t.InputSchema["required"].([]string); ok {
			tool.InputSchema.Required = req
		}
		params.Tools = append(params.Tools, sdk.ToolUnionParam{OfTool: &tool})
	}
	if modern(req.Model) {
		adaptive := sdk.ThinkingConfigAdaptiveParam{Display: sdk.ThinkingConfigAdaptiveDisplaySummarized}
		params.Thinking = sdk.ThinkingConfigParamUnion{OfAdaptive: &adaptive}
		if req.Effort != "" {
			params.OutputConfig = sdk.OutputConfigParam{Effort: sdk.OutputConfigEffort(req.Effort)}
		}
	}
	return params
}

// toParams converts neutral history to API messages. Thinking parts from
// another provider are dropped; this provider's are sent back unchanged.
func toParams(msgs []provider.Message) []sdk.MessageParam {
	out := make([]sdk.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		var blocks []sdk.ContentBlockParamUnion
		for _, part := range m.Parts {
			switch part.Type {
			case provider.PartText:
				if part.Text != "" {
					blocks = append(blocks, sdk.NewTextBlock(part.Text))
				}
			case provider.PartThinking:
				if part.Provider != provider.TypeAnthropic {
					continue
				}
				var o opaqueThinking
				json.Unmarshal(part.Opaque, &o)
				if o.Redacted != "" {
					blocks = append(blocks, sdk.NewRedactedThinkingBlock(o.Redacted))
				} else {
					blocks = append(blocks, sdk.NewThinkingBlock(o.Signature, part.Text))
				}
			case provider.PartToolUse:
				input := json.RawMessage(part.Input)
				if !json.Valid(input) {
					input = json.RawMessage(`{}`) // a cut-off call must not break every later request
				}
				blocks = append(blocks, sdk.NewToolUseBlock(part.ToolCallID, input, part.ToolName))
			case provider.PartToolResult:
				blocks = append(blocks, sdk.NewToolResultBlock(part.ToolCallID, part.Text, part.IsError))
			case provider.PartImage:
				if part.Image != nil && len(part.Image.Data) > 0 {
					blocks = append(blocks, sdk.NewImageBlockBase64(part.Image.MediaType, part.Image.Base64()))
				}
			}
		}
		if len(blocks) == 0 {
			continue
		}
		if m.Role == provider.RoleAssistant {
			out = append(out, sdk.NewAssistantMessage(blocks...))
		} else {
			out = append(out, sdk.NewUserMessage(blocks...))
		}
	}
	return out
}

func (p *Provider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	stream := p.client.Messages.NewStreaming(ctx, p.params(req))
	ch := make(chan provider.Event, 64)
	go func() {
		defer close(ch)
		defer provider.RecoverStream(ch)
		defer stream.Close()
		send := func(ev provider.Event) bool {
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		var msg sdk.Message
		for stream.Next() {
			ev := stream.Current()
			if err := msg.Accumulate(ev); err != nil {
				send(provider.Event{Type: provider.EventError, Err: err})
				return
			}
			var out provider.Event
			switch e := ev.AsAny().(type) {
			case sdk.ContentBlockStartEvent:
				if tu, ok := e.ContentBlock.AsAny().(sdk.ToolUseBlock); ok {
					out = provider.Event{Type: provider.EventToolCallStart, ToolCallID: tu.ID, ToolName: tu.Name}
				}
			case sdk.ContentBlockDeltaEvent:
				switch d := e.Delta.AsAny().(type) {
				case sdk.TextDelta:
					out = provider.Event{Type: provider.EventTextDelta, Text: d.Text}
				case sdk.ThinkingDelta:
					out = provider.Event{Type: provider.EventThinkingDelta, Text: d.Thinking}
				case sdk.InputJSONDelta:
					out = provider.Event{Type: provider.EventToolArgsDelta, Text: d.PartialJSON}
				}
			}
			if out.Type != "" && !send(out) {
				return
			}
		}
		if err := stream.Err(); err != nil {
			send(provider.Event{Type: provider.EventError, Err: describe(err)})
			return
		}
		// Only message_delta sets this, so a stream cut cleanly before it has no reply
		// to trust, and possibly a tool call with empty arguments.
		if msg.StopReason == "" {
			send(provider.Event{Type: provider.EventError, Err: provider.ErrStreamEnded})
			return
		}
		final := fromMessage(msg)
		send(provider.Event{
			Type:       provider.EventDone,
			Message:    &final,
			StopReason: provider.StopReason(msg.StopReason),
			Usage: provider.Usage{
				InputTokens:      msg.Usage.InputTokens,
				OutputTokens:     msg.Usage.OutputTokens,
				CacheReadTokens:  msg.Usage.CacheReadInputTokens,
				CacheWriteTokens: msg.Usage.CacheCreationInputTokens,
			},
		})
	}()
	return ch, nil
}

// fromMessage converts an accumulated response to a neutral message.
func fromMessage(msg sdk.Message) provider.Message {
	out := provider.Message{Role: provider.RoleAssistant}
	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case sdk.TextBlock:
			out.Parts = append(out.Parts, provider.Part{Type: provider.PartText, Text: b.Text})
		case sdk.ThinkingBlock:
			o, _ := json.Marshal(opaqueThinking{Signature: b.Signature})
			out.Parts = append(out.Parts, provider.Part{Type: provider.PartThinking, Text: b.Thinking, Opaque: o, Provider: provider.TypeAnthropic})
		case sdk.RedactedThinkingBlock:
			o, _ := json.Marshal(opaqueThinking{Redacted: b.Data})
			out.Parts = append(out.Parts, provider.Part{Type: provider.PartThinking, Opaque: o, Provider: provider.TypeAnthropic})
		case sdk.ToolUseBlock:
			out.Parts = append(out.Parts, provider.Part{
				Type: provider.PartToolUse, ToolCallID: b.ID, ToolName: b.Name,
				Input: json.RawMessage(b.JSON.Input.Raw()),
			})
		}
	}
	return out
}

// describe turns an API error into a readable one naming the cause (PRV-002).
func describe(err error) error {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode < 400 {
		// An error event inside a stream that began fine: the status is the
		// stream's 200, the kind is in the event.
		return streamError("Anthropic", apiErr.Error())
	}
	if errors.As(err, &apiErr) {
		msg := apiErr.Error()
		switch {
		case apiErr.StatusCode == http.StatusUnauthorized:
			msg = "Anthropic rejected the API key (401)"
		case apiErr.StatusCode == http.StatusForbidden:
			msg = fmt.Sprintf("Anthropic refused access (403): %s", apiErr.Error())
		case apiErr.StatusCode == http.StatusNotFound:
			msg = "not found (404); check the base URL and model ID"
		case apiErr.StatusCode == http.StatusTooManyRequests:
			msg = "rate limited by Anthropic (429)"
		case apiErr.StatusCode >= 500:
			msg = fmt.Sprintf("Anthropic server error (%d)", apiErr.StatusCode)
		}
		if apiErr.Response != nil {
			return provider.NewHTTPError(apiErr.Response, msg)
		}
		return &provider.HTTPError{Status: apiErr.StatusCode, Message: msg}
	}
	return err
}

// streamError classifies an error event inside a stream, so the harness
// retries an overload or a rate limit like the same HTTP status.
func streamError(vendor, msg string) error {
	status := 0
	switch {
	case strings.Contains(msg, "overloaded"):
		status = 529
	case strings.Contains(msg, "rate_limit"):
		status = http.StatusTooManyRequests
	case strings.Contains(msg, "api_error"), strings.Contains(msg, "server_error"), strings.Contains(msg, "timeout"):
		status = http.StatusInternalServerError
	}
	if status == 0 {
		return fmt.Errorf("%s: %s", vendor, msg)
	}
	return &provider.HTTPError{Status: status, Message: fmt.Sprintf("%s: %s", vendor, msg)}
}
