// Package openairesponses adapts OpenAI's Responses API to
// provider.Provider (PRV-012): reasoning summaries stream as thinking, and
// reasoning items travel back encrypted, so the model keeps its reasoning
// across tool calls without the conversation being stored at OpenAI.
package openairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Type is the provider type in providers.yaml.
const Type provider.Type = "openai_responses"

// DefaultBaseURL is the OpenAI API.
const DefaultBaseURL = "https://api.openai.com/v1"

// DefaultModel is offered when the user has not picked one.
const DefaultModel = "gpt-5.5"

// Options configures New.
type Options struct {
	ID      string
	BaseURL string
	APIKey  string
	Headers map[string]string
	// HTTPClient is for tests.
	HTTPClient *http.Client
}

// Provider talks to one Responses endpoint.
type Provider struct {
	id     string
	client sdk.Client
}

// New returns a provider for opts.
func New(opts Options) *Provider {
	// The harness retries with its own backoff (PRV-010).
	ro := []option.RequestOption{option.WithAPIKey(opts.APIKey), option.WithMaxRetries(0)}
	base := opts.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	ro = append(ro, option.WithBaseURL(strings.TrimSuffix(base, "/")+"/"))
	for k, v := range opts.Headers {
		ro = append(ro, option.WithHeader(k, v))
	}
	if opts.HTTPClient != nil {
		ro = append(ro, option.WithHTTPClient(opts.HTTPClient))
	}
	return &Provider{id: opts.ID, client: sdk.NewClient(ro...)}
}

func (p *Provider) ID() string          { return p.id }
func (p *Provider) Type() provider.Type { return Type }

// chatModels are the models the Responses API serves for conversation;
// the list endpoint also returns embedding, audio and image models.
var chatModels = regexp.MustCompile(`^(gpt-|o[1-9]|codex-|chatgpt-)`)

// notChat are kinds of models under those prefixes that do not converse.
var notChat = regexp.MustCompile(`(audio|realtime|transcribe|tts|image|search|embedding)`)

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	var out []provider.ModelInfo
	iter := p.client.Models.ListAutoPaging(ctx)
	for iter.Next() {
		m := iter.Current()
		if chatModels.MatchString(m.ID) && !notChat.MatchString(m.ID) {
			out = append(out, provider.ModelInfo{ID: m.ID, DisplayName: m.ID})
		}
	}
	if err := iter.Err(); err != nil {
		return nil, describe(err)
	}
	return out, nil
}

// reasoningModel matches the models that reason and take an effort.
var reasoningModel = regexp.MustCompile(`^(o[1-9]|gpt-5|codex-)`)

func (p *Provider) Capabilities(model string) provider.ModelCaps {
	c := provider.ModelCaps{
		Source: provider.CapsStatic,
		Tools:  true, ParallelTools: true, StreamedToolArgs: true, StableToolCallIDs: true,
		PromptCaching: true,
	}
	if reasoningModel.MatchString(model) {
		c.Reasoning, c.Effort = true, true
	}
	return c
}

// effort maps the app's efforts onto the API's; max is the API's xhigh
// where the model has no max.
func effort(e string) shared.ReasoningEffort {
	switch e {
	case "low", "medium", "high", "xhigh":
		return shared.ReasoningEffort(e)
	case "max":
		return shared.ReasoningEffortXhigh
	}
	return ""
}

func (p *Provider) params(req provider.ChatRequest) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model: req.Model,
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: toInput(req.Messages)},
		// Nothing is kept at OpenAI; reasoning comes back encrypted instead.
		Store:             param.NewOpt(false),
		ParallelToolCalls: param.NewOpt(true),
	}
	if req.MaxTokens > 0 {
		params.MaxOutputTokens = param.NewOpt(int64(req.MaxTokens))
	}
	if req.System != "" {
		params.Instructions = param.NewOpt(req.System)
	}
	for _, t := range req.Tools {
		params.Tools = append(params.Tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{
			Name:        t.Name,
			Description: param.NewOpt(t.Description),
			Parameters:  t.InputSchema,
			// Strict mode needs every property required; the harness
			// validates arguments itself.
			Strict: param.NewOpt(false),
		}})
	}
	if reasoningModel.MatchString(req.Model) {
		params.Reasoning = shared.ReasoningParam{Summary: shared.ReasoningSummaryAuto}
		if e := effort(req.Effort); e != "" {
			params.Reasoning.Effort = e
		}
		params.Include = []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent}
	}
	return params
}

// toInput converts neutral history to input items. Reasoning items from
// this provider go back as they came, with their encrypted content; other
// providers' thinking is dropped.
func toInput(msgs []provider.Message) responses.ResponseInputParam {
	var out responses.ResponseInputParam
	for _, m := range msgs {
		var text strings.Builder
		flush := func() {
			if text.Len() == 0 {
				return
			}
			role := responses.EasyInputMessageRoleUser
			if m.Role == provider.RoleAssistant {
				role = responses.EasyInputMessageRoleAssistant
			}
			out = append(out, responses.ResponseInputItemParamOfMessage(text.String(), role))
			text.Reset()
		}
		for _, part := range m.Parts {
			switch part.Type {
			case provider.PartText:
				if part.Text != "" {
					if text.Len() > 0 {
						text.WriteString("\n\n")
					}
					text.WriteString(part.Text)
				}
			case provider.PartThinking:
				if part.Provider != Type || len(part.Opaque) == 0 {
					continue
				}
				flush()
				out = append(out, param.Override[responses.ResponseInputItemUnionParam](part.Opaque))
			case provider.PartToolUse:
				flush()
				args := string(part.Input)
				if !json.Valid(part.Input) {
					args = "{}"
				}
				out = append(out, responses.ResponseInputItemParamOfFunctionCall(args, part.ToolCallID, part.ToolName))
			case provider.PartToolResult:
				flush()
				item := responses.ResponseInputItemParamOfFunctionCallOutput(part.Text)
				item.OfFunctionCallOutput.CallID = param.NewOpt(part.ToolCallID)
				out = append(out, item)
			}
		}
		flush()
	}
	return out
}

func (p *Provider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	stream := p.client.Responses.NewStreaming(ctx, p.params(req))
	ch := make(chan provider.Event, 64)
	go func() {
		defer close(ch)
		defer stream.Close()
		send := func(ev provider.Event) bool {
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		var final *responses.Response
		for stream.Next() {
			ev := stream.Current()
			var out provider.Event
			switch ev.Type {
			case "response.output_text.delta":
				out = provider.Event{Type: provider.EventTextDelta, Text: ev.Delta}
			case "response.reasoning_summary_text.delta":
				out = provider.Event{Type: provider.EventThinkingDelta, Text: ev.Delta}
			case "response.output_item.added":
				if ev.Item.Type == "function_call" {
					out = provider.Event{Type: provider.EventToolCallStart, ToolCallID: ev.Item.CallID, ToolName: ev.Item.Name}
				}
			case "response.function_call_arguments.delta":
				out = provider.Event{Type: provider.EventToolArgsDelta, Text: ev.Delta}
			case "response.completed", "response.incomplete":
				r := ev.Response
				final = &r
			case "response.failed":
				msg := ev.Response.Error.Message
				if msg == "" {
					msg = "the response failed"
				}
				send(provider.Event{Type: provider.EventError, Err: fmt.Errorf("OpenAI: %s", msg)})
				return
			case "error":
				send(provider.Event{Type: provider.EventError, Err: fmt.Errorf("OpenAI: %s", cmpOr(ev.Message, ev.Code, "stream error"))})
				return
			}
			if out.Type != "" && !send(out) {
				return
			}
		}
		if err := stream.Err(); err != nil {
			send(provider.Event{Type: provider.EventError, Err: describe(err)})
			return
		}
		if final == nil {
			send(provider.Event{Type: provider.EventError, Err: errors.New("OpenAI: the stream ended without a response")})
			return
		}
		msg, stop := fromResponse(*final)
		send(provider.Event{
			Type: provider.EventDone, Message: &msg, StopReason: stop,
			Usage: provider.Usage{
				InputTokens:     final.Usage.InputTokens - final.Usage.InputTokensDetails.CachedTokens,
				OutputTokens:    final.Usage.OutputTokens,
				CacheReadTokens: final.Usage.InputTokensDetails.CachedTokens,
			},
		})
	}()
	return ch, nil
}

// fromResponse converts a finished response to a neutral message and why
// it stopped.
func fromResponse(r responses.Response) (provider.Message, provider.StopReason) {
	out := provider.Message{Role: provider.RoleAssistant}
	stop := provider.StopEndTurn
	for _, item := range r.Output {
		switch item.Type {
		case "message":
			for _, c := range item.AsMessage().Content {
				switch c.Type {
				case "output_text":
					out.Parts = append(out.Parts, provider.Part{Type: provider.PartText, Text: c.Text})
				case "refusal":
					out.Parts = append(out.Parts, provider.Part{Type: provider.PartText, Text: c.Refusal})
					stop = provider.StopRefusal
				}
			}
		case "reasoning":
			ri := item.AsReasoning()
			var summary []string
			for _, s := range ri.Summary {
				summary = append(summary, s.Text)
			}
			out.Parts = append(out.Parts, provider.Part{
				Type: provider.PartThinking, Text: strings.Join(summary, "\n\n"),
				Opaque: reasoningItem(ri), Provider: Type,
			})
		case "function_call":
			fc := item.AsFunctionCall()
			input := json.RawMessage(fc.Arguments)
			if !json.Valid(input) {
				input = json.RawMessage(`{}`)
			}
			out.Parts = append(out.Parts, provider.Part{Type: provider.PartToolUse, ToolCallID: fc.CallID, ToolName: fc.Name, Input: input})
		}
	}
	if len(out.ToolCalls()) > 0 {
		stop = provider.StopToolUse
	} else if r.IncompleteDetails.Reason == "max_output_tokens" {
		stop = provider.StopMaxTokens
	}
	return out, stop
}

// reasoningItem is what a reasoning item sends back on the next request:
// its ID, summary and encrypted content, without output-only fields.
func reasoningItem(ri responses.ResponseReasoningItem) json.RawMessage {
	type summary struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	v := struct {
		Type             string    `json:"type"`
		ID               string    `json:"id"`
		Summary          []summary `json:"summary"`
		EncryptedContent string    `json:"encrypted_content,omitempty"`
	}{Type: "reasoning", ID: ri.ID, Summary: []summary{}, EncryptedContent: ri.EncryptedContent}
	for _, s := range ri.Summary {
		v.Summary = append(v.Summary, summary{Type: "summary_text", Text: s.Text})
	}
	b, _ := json.Marshal(v)
	return b
}

func cmpOr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// describe turns an API error into a readable one naming the cause (PRV-002).
func describe(err error) error {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		msg := apiErr.Error()
		switch {
		case apiErr.StatusCode == http.StatusUnauthorized:
			msg = "OpenAI rejected the API key (401)"
		case apiErr.StatusCode == http.StatusForbidden:
			msg = fmt.Sprintf("OpenAI refused access (403): %s", apiErr.Message)
		case apiErr.StatusCode == http.StatusNotFound:
			msg = "not found (404); check the base URL and model ID"
		case apiErr.StatusCode == http.StatusTooManyRequests:
			msg = "rate limited by OpenAI (429)"
		case apiErr.StatusCode >= 500:
			msg = fmt.Sprintf("OpenAI server error (%d)", apiErr.StatusCode)
		}
		if apiErr.Response != nil {
			return provider.NewHTTPError(apiErr.Response, msg)
		}
		return &provider.HTTPError{Status: apiErr.StatusCode, Message: msg}
	}
	return err
}
