// Package gemini adapts the Gemini API to provider.Provider through
// Google's Gen AI SDK (PRV-012): thoughts stream as thinking, and thought
// signatures travel back with the parts they came on, as the API requires
// for tool calls.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"

	"google.golang.org/genai"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Type is the provider type in providers.yaml.
const Type provider.Type = "gemini"

// DefaultBaseURL is the Gemini API.
const DefaultBaseURL = "https://generativelanguage.googleapis.com"

// DefaultModel is offered when the user has not picked one.
const DefaultModel = "gemini-3-pro"

// Options configures New.
type Options struct {
	ID      string
	BaseURL string
	APIKey  string
	Headers map[string]string
	// HTTPClient is for tests.
	HTTPClient *http.Client
}

// Provider talks to one Gemini endpoint.
type Provider struct {
	id     string
	client *genai.Client
	err    error
	// turns numbers responses, so IDs made up for calls are unique in a
	// session's history.
	turns atomic.Int64
}

// New returns a provider for opts. Without an API key every call fails: the
// SDK would otherwise take one from the app's own environment.
func New(opts Options) *Provider {
	p := &Provider{id: opts.ID}
	if strings.TrimSpace(opts.APIKey) == "" {
		p.err = errors.New("the Gemini provider needs an API key")
		return p
	}
	hdr := http.Header{}
	for k, v := range opts.Headers {
		hdr.Set(k, v)
	}
	cc := &genai.ClientConfig{
		APIKey:      opts.APIKey,
		Backend:     genai.BackendGeminiAPI,
		HTTPClient:  opts.HTTPClient,
		HTTPOptions: genai.HTTPOptions{BaseURL: opts.BaseURL, Headers: hdr},
	}
	if cc.HTTPClient == nil {
		// http.DefaultTransport, so local-only mode covers it (NFR-OFF-003).
		cc.HTTPClient = &http.Client{}
	}
	p.client, p.err = genai.NewClient(context.Background(), cc)
	return p
}

func (p *Provider) ID() string          { return p.id }
func (p *Provider) Type() provider.Type { return Type }

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if p.err != nil {
		return nil, p.err
	}
	var out []provider.ModelInfo
	for m, err := range p.client.Models.All(ctx) {
		if err != nil {
			return nil, describe(err)
		}
		if !slices.Contains(m.SupportedActions, "generateContent") {
			continue
		}
		id := strings.TrimPrefix(m.Name, "models/")
		if !strings.HasPrefix(id, "gemini-") || strings.Contains(id, "embedding") || strings.Contains(id, "image") || strings.Contains(id, "tts") {
			continue
		}
		out = append(out, provider.ModelInfo{ID: id, DisplayName: cmpOr(m.DisplayName, id)})
	}
	return out, nil
}

// thinks reports whether a model thinks and takes a level or budget: 2.5
// and later.
func thinks(model string) bool {
	v, ok := strings.CutPrefix(model, "gemini-")
	return ok && (strings.HasPrefix(v, "2.5") || (len(v) > 0 && v[0] >= '3' && v[0] <= '9'))
}

func (p *Provider) Capabilities(model string) provider.ModelCaps {
	c := provider.ModelCaps{
		Source: provider.CapsStatic,
		Tools:  true, ParallelTools: true,
		// Calls arrive whole, with IDs only on some models; the adapter
		// makes IDs up where they are missing.
		StreamedToolArgs: false, StableToolCallIDs: true,
		ContextWindow: 1_048_576, MaxOutput: 65_536,
	}
	if thinks(model) {
		c.Reasoning, c.Effort = true, true
	}
	return c
}

// thinking sets how much a model thinks for an effort: a level from Gemini
// 3 on, a token budget on 2.5.
func thinking(model, effort string) *genai.ThinkingConfig {
	tc := &genai.ThinkingConfig{IncludeThoughts: true}
	if effort == "" {
		return tc
	}
	if strings.HasPrefix(model, "gemini-2.5") {
		// Flash models take at most 24576 tokens, Pro 32768.
		top := int32(24576)
		if strings.Contains(model, "-pro") {
			top = 32768
		}
		budget := map[string]int32{"low": 1024, "medium": 8192, "high": min(24576, top), "xhigh": top, "max": top}[effort]
		if budget != 0 {
			tc.ThinkingBudget = &budget
		}
		return tc
	}
	switch effort {
	case "low":
		tc.ThinkingLevel = genai.ThinkingLevelLow
	case "medium":
		tc.ThinkingLevel = genai.ThinkingLevelMedium
	default:
		tc.ThinkingLevel = genai.ThinkingLevelHigh
	}
	return tc
}

// signed is what a Gemini part carries back: its thought signature.
type signed struct {
	Signature []byte `json:"signature,omitempty"`
}

func opaque(sig []byte) json.RawMessage {
	if len(sig) == 0 {
		return nil
	}
	b, _ := json.Marshal(signed{Signature: sig})
	return b
}

func signature(part provider.Part) []byte {
	if part.Provider != Type || len(part.Opaque) == 0 {
		return nil
	}
	var s signed
	json.Unmarshal(part.Opaque, &s)
	return s.Signature
}

func (p *Provider) config(req provider.ChatRequest) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{}
	if req.MaxTokens > 0 {
		cfg.MaxOutputTokens = int32(req.MaxTokens)
	}
	if req.System != "" {
		cfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: req.System}}}
	}
	if len(req.Tools) > 0 {
		var decls []*genai.FunctionDeclaration
		for _, t := range req.Tools {
			decls = append(decls, &genai.FunctionDeclaration{Name: t.Name, Description: t.Description, ParametersJsonSchema: t.InputSchema})
		}
		cfg.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}
	if thinks(req.Model) {
		cfg.ThinkingConfig = thinking(req.Model, req.Effort)
	}
	return cfg
}

// toContents converts neutral history. Thought text is not sent back (the
// signatures carry the model's reasoning); another provider's thinking is
// dropped. A function response names its function, which the neutral
// tool_result does not, so names come from the calls before it.
func toContents(msgs []provider.Message) []*genai.Content {
	names := map[string]string{}
	var out []*genai.Content
	for _, m := range msgs {
		c := &genai.Content{Role: genai.RoleUser}
		if m.Role == provider.RoleAssistant {
			c.Role = genai.RoleModel
		}
		for _, part := range m.Parts {
			switch part.Type {
			case provider.PartText:
				if part.Text != "" || len(signature(part)) > 0 {
					c.Parts = append(c.Parts, &genai.Part{Text: part.Text, ThoughtSignature: signature(part)})
				}
			case provider.PartThinking:
				if sig := signature(part); len(sig) > 0 {
					c.Parts = append(c.Parts, &genai.Part{Thought: true, ThoughtSignature: sig})
				}
			case provider.PartToolUse:
				var args map[string]any
				json.Unmarshal(part.Input, &args)
				names[part.ToolCallID] = part.ToolName
				c.Parts = append(c.Parts, &genai.Part{
					FunctionCall:     &genai.FunctionCall{ID: wireID(part.ToolCallID), Name: part.ToolName, Args: args},
					ThoughtSignature: signature(part),
				})
			case provider.PartToolResult:
				key := "output"
				if part.IsError {
					key = "error"
				}
				c.Parts = append(c.Parts, &genai.Part{FunctionResponse: &genai.FunctionResponse{
					ID: wireID(part.ToolCallID), Name: names[part.ToolCallID], Response: map[string]any{key: part.Text},
				}})
			}
		}
		if len(c.Parts) > 0 {
			out = append(out, c)
		}
	}
	return out
}

// madeUp marks IDs the adapter made up for calls that came without one;
// they are not sent back.
const madeUp = "gemini-call-"

func wireID(id string) string {
	if strings.HasPrefix(id, madeUp) {
		return ""
	}
	return id
}

func (p *Provider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	if p.err != nil {
		return nil, p.err
	}
	seq := p.client.Models.GenerateContentStream(ctx, req.Model, toContents(req.Messages), p.config(req))
	ch := make(chan provider.Event, 64)
	go func() {
		defer close(ch)
		send := func(ev provider.Event) bool {
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		done, err := p.run(seq, send)
		if err != nil {
			send(provider.Event{Type: provider.EventError, Err: err})
			return
		}
		send(done)
	}()
	return ch, nil
}

// run streams the chunks of seq as deltas and returns the whole message.
func (p *Provider) run(seq iter.Seq2[*genai.GenerateContentResponse, error], send func(provider.Event) bool) (provider.Event, error) {
	msg := provider.Message{Role: provider.RoleAssistant}
	var usage provider.Usage
	var finish genai.FinishReason
	calls := 0
	turn := p.turns.Add(1)
	// add appends text to the last part when it is of the same kind, so a
	// streamed reply becomes one part; a signature stays with its part.
	add := func(typ provider.PartType, text string, sig []byte) {
		if n := len(msg.Parts); n > 0 && msg.Parts[n-1].Type == typ && len(msg.Parts[n-1].Opaque) == 0 {
			msg.Parts[n-1].Text += text
			if len(sig) > 0 {
				msg.Parts[n-1].Opaque, msg.Parts[n-1].Provider = opaque(sig), Type
			}
			return
		}
		part := provider.Part{Type: typ, Text: text}
		if len(sig) > 0 {
			part.Opaque, part.Provider = opaque(sig), Type
		}
		msg.Parts = append(msg.Parts, part)
	}
	for resp, err := range seq {
		if err != nil {
			return provider.Event{}, describe(err)
		}
		if u := resp.UsageMetadata; u != nil {
			usage = provider.Usage{
				InputTokens:     int64(u.PromptTokenCount - u.CachedContentTokenCount),
				OutputTokens:    int64(u.CandidatesTokenCount + u.ThoughtsTokenCount),
				CacheReadTokens: int64(u.CachedContentTokenCount),
			}
		}
		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != "" {
				finish = resp.Candidates[0].FinishReason
			}
			continue
		}
		cand := resp.Candidates[0]
		if cand.FinishReason != "" {
			finish = cand.FinishReason
		}
		for _, part := range cand.Content.Parts {
			switch {
			case part.FunctionCall != nil:
				calls++
				fc := part.FunctionCall
				id := fc.ID
				if id == "" {
					id = fmt.Sprintf("%s%d-%d", madeUp, turn, calls)
				}
				args, _ := json.Marshal(fc.Args)
				if fc.Args == nil {
					args = []byte("{}")
				}
				if !send(provider.Event{Type: provider.EventToolCallStart, ToolCallID: id, ToolName: fc.Name}) ||
					!send(provider.Event{Type: provider.EventToolArgsDelta, Text: string(args)}) {
					return provider.Event{}, errors.New("cancelled")
				}
				tp := provider.Part{Type: provider.PartToolUse, ToolCallID: id, ToolName: fc.Name, Input: args}
				if len(part.ThoughtSignature) > 0 {
					tp.Opaque, tp.Provider = opaque(part.ThoughtSignature), Type
				}
				msg.Parts = append(msg.Parts, tp)
			case part.Thought:
				if part.Text != "" && !send(provider.Event{Type: provider.EventThinkingDelta, Text: part.Text}) {
					return provider.Event{}, errors.New("cancelled")
				}
				add(provider.PartThinking, part.Text, part.ThoughtSignature)
			case part.Text != "" || len(part.ThoughtSignature) > 0:
				if part.Text != "" && !send(provider.Event{Type: provider.EventTextDelta, Text: part.Text}) {
					return provider.Event{}, errors.New("cancelled")
				}
				add(provider.PartText, part.Text, part.ThoughtSignature)
			}
		}
	}
	stop := provider.StopEndTurn
	switch {
	case calls > 0:
		stop = provider.StopToolUse
	case finish == genai.FinishReasonMaxTokens:
		stop = provider.StopMaxTokens
	case finish == genai.FinishReasonSafety || finish == genai.FinishReasonRecitation || finish == genai.FinishReasonBlocklist ||
		finish == genai.FinishReasonProhibitedContent || finish == genai.FinishReasonSPII:
		stop = provider.StopRefusal
	}
	return provider.Event{Type: provider.EventDone, Message: &msg, StopReason: stop, Usage: usage}, nil
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
	var apiErr genai.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	msg := apiErr.Message
	switch {
	case apiErr.Code == http.StatusBadRequest && strings.Contains(apiErr.Message, "API key"):
		msg = "Google rejected the API key (400)"
	case apiErr.Code == http.StatusUnauthorized || apiErr.Code == http.StatusForbidden:
		msg = fmt.Sprintf("Google refused access (%d): %s", apiErr.Code, apiErr.Message)
	case apiErr.Code == http.StatusNotFound:
		msg = "not found (404); check the base URL and model ID"
	case apiErr.Code == http.StatusTooManyRequests:
		msg = "rate limited by Google (429)"
	case apiErr.Code >= 500:
		msg = fmt.Sprintf("Google server error (%d)", apiErr.Code)
	}
	return &provider.HTTPError{Status: apiErr.Code, Message: msg}
}
