// Package openaicompat adapts any OpenAI-compatible Chat Completions
// endpoint to provider.Provider: OpenAI, OpenRouter, LM Studio, llama.cpp
// server, vLLM and Ollama's /v1. It is hand-written so it can absorb the
// known deviations: missing tool-call IDs, tool calls that reuse index 0,
// reasoning under different field names, and servers without usage in the
// stream.
package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Type is this adapter's provider type.
const Type provider.Type = "openai_compat"

// Preset prefills a provider form and tells the adapter about the endpoint.
type Preset struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	BaseURL  string            `json:"baseUrl"`
	NeedsKey bool              `json:"needsKey"`
	Local    bool              `json:"local"`
	Headers  map[string]string `json:"headers,omitempty"`
	Note     string            `json:"note,omitempty"`
	// StreamUsage sends stream_options.include_usage.
	StreamUsage bool `json:"-"`
	// Cloud endpoints whose models all call tools reliably.
	ToolsKnown bool `json:"-"`
	// Effort says how the endpoint takes a reasoning effort (PRV-011):
	// "openai" as reasoning_effort, for its reasoning models only;
	// "openrouter" as reasoning.effort, which models without reasoning
	// ignore; empty for not at all.
	Effort string `json:"-"`
}

// Presets are the endpoints offered in the provider form (PRV-001).
var Presets = []Preset{
	{ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", NeedsKey: true, StreamUsage: true, ToolsKnown: true, Effort: "openai"},
	{ID: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", NeedsKey: true, StreamUsage: true, ToolsKnown: true, Effort: "openrouter",
		Headers: map[string]string{"HTTP-Referer": "https://github.com/FinanceNik/Freeloop", "X-Title": "Freeloop"},
		Note:    "Not every OpenRouter model supports tools; pick one that does."},
	{ID: "lmstudio", Name: "LM Studio", BaseURL: "http://localhost:1234/v1", Local: true, StreamUsage: true},
	{ID: "llamacpp", Name: "llama.cpp server", BaseURL: "http://localhost:8080/v1", Local: true, StreamUsage: true,
		Note: "Start llama-server with --jinja, or tool calls will not work."},
	{ID: "vllm", Name: "vLLM", BaseURL: "http://localhost:8000/v1", Local: true, StreamUsage: true,
		Note: "Start vLLM with --enable-auto-tool-choice and a --tool-call-parser."},
	{ID: "ollama-v1", Name: "Ollama (OpenAI-compatible)", BaseURL: "http://localhost:11434/v1", Local: true,
		Note: "The native Ollama provider is usually the better choice."},
	{ID: "custom", Name: "Custom OpenAI-compatible", Local: true},
}

// PresetByID returns the preset with id, or the custom preset.
func PresetByID(id string) Preset {
	if i := slices.IndexFunc(Presets, func(p Preset) bool { return p.ID == id }); i >= 0 {
		return Presets[i]
	}
	return Presets[len(Presets)-1]
}

// Options configures New.
type Options struct {
	ID      string
	Preset  string
	BaseURL string
	APIKey  string
	Headers map[string]string
	// HTTPClient is for tests.
	HTTPClient *http.Client
}

// Provider talks to one endpoint.
type Provider struct {
	id      string
	preset  Preset
	base    string
	key     string
	headers map[string]string
	client  *http.Client
	calls   atomic.Int64 // for synthesized tool-call IDs
}

// New returns a provider for opts. The preset's base URL and headers apply
// unless opts overrides them.
func New(opts Options) *Provider {
	pr := PresetByID(opts.Preset)
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = pr.BaseURL
	}
	headers := map[string]string{}
	for k, v := range pr.Headers {
		headers[k] = v
	}
	for k, v := range opts.Headers {
		headers[k] = v
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	return &Provider{id: opts.ID, preset: pr, base: base, key: opts.APIKey, headers: headers, client: client}
}

func (p *Provider) ID() string          { return p.id }
func (p *Provider) Type() provider.Type { return Type }

func (p *Provider) Capabilities(model string) provider.ModelCaps {
	if p.preset.ToolsKnown {
		c := provider.ModelCaps{Source: provider.CapsStatic, Tools: true, ParallelTools: true, StreamedToolArgs: true, StableToolCallIDs: true}
		c.Effort = p.takesEffort(model)
		c.Reasoning = c.Effort
		return c
	}
	return provider.ModelCaps{Source: provider.CapsUnknown}
}

// openAIReasoning matches OpenAI's reasoning models, which take
// reasoning_effort; its other models reject the parameter.
var openAIReasoning = regexp.MustCompile(`^(o[1-9]|gpt-5)`)

// takesEffort reports whether a request for model carries the effort.
func (p *Provider) takesEffort(model string) bool {
	switch p.preset.Effort {
	case "openai":
		return openAIReasoning.MatchString(model)
	case "openrouter":
		return true
	}
	return false
}

// wireEffort maps the app's efforts (low to max) onto the low, medium and
// high these endpoints take.
func wireEffort(e string) string {
	switch e {
	case "low", "medium", "high":
		return e
	case "xhigh", "max":
		return "high"
	}
	return ""
}

func (p *Provider) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.key != "" {
		req.Header.Set("Authorization", "Bearer "+p.key)
	}
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			return nil, fmt.Errorf("could not connect to %s; is the server running?", p.base)
		}
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		return nil, statusError(resp)
	}
	return resp, nil
}

func statusError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var e struct {
		Error json.RawMessage `json:"error"`
	}
	msg := strings.TrimSpace(string(b))
	if json.Unmarshal(b, &e) == nil && len(e.Error) > 0 {
		var obj struct{ Message string }
		var s string
		if json.Unmarshal(e.Error, &obj) == nil && obj.Message != "" {
			msg = obj.Message
		} else if json.Unmarshal(e.Error, &s) == nil {
			msg = s
		}
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		msg = fmt.Sprintf("the endpoint rejected the API key (401): %s", msg)
	case http.StatusNotFound:
		msg = fmt.Sprintf("not found (404); check the base URL and model ID: %s", msg)
	case http.StatusTooManyRequests:
		msg = fmt.Sprintf("rate limited (429): %s", msg)
	default:
		msg = fmt.Sprintf("the endpoint returned %d: %s", resp.StatusCode, msg)
	}
	return provider.NewHTTPError(resp, msg)
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	resp, err := p.do(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("unexpected /models response: %w", err)
	}
	out := make([]provider.ModelInfo, 0, len(body.Data))
	for _, m := range body.Data {
		out = append(out, provider.ModelInfo{ID: m.ID, DisplayName: m.Name})
	}
	slices.SortFunc(out, func(a, b provider.ModelInfo) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// Wire types for Chat Completions.

type wireMessage struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content"`
	ToolCalls  []wireCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type wireCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

func strPtr(s string) *string { return &s }

// toWire converts neutral history. Tool results become role "tool"
// messages; reasoning is not sent back.
func toWire(system string, msgs []provider.Message) []wireMessage {
	out := []wireMessage{}
	if system != "" {
		out = append(out, wireMessage{Role: "system", Content: strPtr(system)})
	}
	for _, m := range msgs {
		if m.Role == provider.RoleAssistant {
			w := wireMessage{Role: "assistant"}
			var text strings.Builder
			for _, part := range m.Parts {
				switch part.Type {
				case provider.PartText:
					text.WriteString(part.Text)
				case provider.PartToolUse:
					c := wireCall{ID: part.ToolCallID, Type: "function"}
					c.Function.Name = part.ToolName
					c.Function.Arguments = string(part.Input)
					if c.Function.Arguments == "" {
						c.Function.Arguments = "{}"
					}
					w.ToolCalls = append(w.ToolCalls, c)
				}
			}
			if text.Len() > 0 || len(w.ToolCalls) == 0 {
				w.Content = strPtr(text.String())
			}
			out = append(out, w)
			continue
		}
		var text strings.Builder
		for _, part := range m.Parts {
			switch part.Type {
			case provider.PartToolResult:
				content := part.Text
				if part.IsError {
					content = "Error: " + content
				}
				out = append(out, wireMessage{Role: "tool", ToolCallID: part.ToolCallID, Content: strPtr(content)})
			case provider.PartText:
				if text.Len() > 0 {
					text.WriteString("\n\n")
				}
				text.WriteString(part.Text)
			}
		}
		if text.Len() > 0 {
			out = append(out, wireMessage{Role: "user", Content: strPtr(text.String())})
		}
	}
	return out
}

type chunk struct {
	Choices []struct {
		Delta struct {
			Content          *string `json:"content"`
			ReasoningContent *string `json:"reasoning_content"`
			Reasoning        *string `json:"reasoning"`
			ToolCalls        []struct {
				Index    *int   `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int64 `json:"prompt_tokens"`
		CompletionTokens    int64 `json:"completion_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type callAcc struct {
	id   string
	name string
	args strings.Builder
}

func (p *Provider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	body := map[string]any{
		"model":    req.Model,
		"messages": toWire(req.System, req.Messages),
		"stream":   true,
	}
	if p.preset.StreamUsage {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if e := wireEffort(req.Effort); e != "" && p.takesEffort(req.Model) {
		if p.preset.Effort == "openrouter" {
			body["reasoning"] = map[string]any{"effort": e}
		} else {
			body["reasoning_effort"] = e
		}
	}
	if len(req.Tools) > 0 {
		tools := make([]wireTool, 0, len(req.Tools))
		for _, t := range req.Tools {
			w := wireTool{Type: "function"}
			w.Function.Name, w.Function.Description, w.Function.Parameters = t.Name, t.Description, t.InputSchema
			tools = append(tools, w)
		}
		body["tools"] = tools
	}
	resp, err := p.do(ctx, http.MethodPost, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	turn := p.calls.Add(1)

	ch := make(chan provider.Event, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		send := func(ev provider.Event) bool {
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		var text, reasoning strings.Builder
		var calls []*callAcc
		var usage provider.Usage
		finish := ""

		newCall := func(id string) *callAcc {
			if id == "" {
				id = fmt.Sprintf("call_%d_%d", turn, len(calls))
			}
			c := &callAcc{id: id}
			calls = append(calls, c)
			return c
		}

		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			data, ok := strings.CutPrefix(line, "data:")
			if !ok {
				continue
			}
			data = strings.TrimSpace(data)
			if data == "[DONE]" {
				break
			}
			var c chunk
			if err := json.Unmarshal([]byte(data), &c); err != nil {
				continue // keep-alives and comments from some servers
			}
			if c.Error != nil {
				send(provider.Event{Type: provider.EventError, Err: errors.New(c.Error.Message)})
				return
			}
			if c.Usage != nil {
				usage.InputTokens = c.Usage.PromptTokens
				usage.OutputTokens = c.Usage.CompletionTokens
				if c.Usage.PromptTokensDetails != nil {
					usage.CacheReadTokens = c.Usage.PromptTokensDetails.CachedTokens
					usage.InputTokens -= usage.CacheReadTokens
				}
			}
			if len(c.Choices) == 0 {
				continue
			}
			ch0 := c.Choices[0]
			d := ch0.Delta
			if d.Content != nil && *d.Content != "" {
				text.WriteString(*d.Content)
				if !send(provider.Event{Type: provider.EventTextDelta, Text: *d.Content}) {
					return
				}
			}
			for _, r := range []*string{d.ReasoningContent, d.Reasoning} {
				if r != nil && *r != "" {
					reasoning.WriteString(*r)
					if !send(provider.Event{Type: provider.EventThinkingDelta, Text: *r}) {
						return
					}
					break
				}
			}
			for _, tc := range d.ToolCalls {
				var acc *callAcc
				switch {
				case tc.Index != nil && *tc.Index < len(calls):
					acc = calls[*tc.Index]
					// Some servers send every call as index 0, each complete.
					if tc.ID != "" && acc.id != tc.ID && json.Valid([]byte(acc.args.String())) {
						acc = newCall(tc.ID)
					}
				case tc.Index == nil && len(calls) > 0 && (tc.ID == "" || tc.ID == calls[len(calls)-1].id):
					acc = calls[len(calls)-1]
				default:
					acc = newCall(tc.ID)
				}
				if acc.name == "" && tc.Function.Name != "" {
					acc.name = tc.Function.Name
					if !send(provider.Event{Type: provider.EventToolCallStart, ToolCallID: acc.id, ToolName: acc.name}) {
						return
					}
				}
				if tc.Function.Arguments != "" {
					acc.args.WriteString(tc.Function.Arguments)
					if !send(provider.Event{Type: provider.EventToolArgsDelta, ToolCallID: acc.id, Text: tc.Function.Arguments}) {
						return
					}
				}
			}
			if ch0.FinishReason != nil && *ch0.FinishReason != "" {
				finish = *ch0.FinishReason
			}
		}
		if err := sc.Err(); err != nil {
			send(provider.Event{Type: provider.EventError, Err: err})
			return
		}
		if ctx.Err() != nil {
			send(provider.Event{Type: provider.EventError, Err: ctx.Err()})
			return
		}

		msg := provider.Message{Role: provider.RoleAssistant}
		if reasoning.Len() > 0 {
			msg.Parts = append(msg.Parts, provider.Part{Type: provider.PartThinking, Text: reasoning.String(), Provider: Type})
		}
		if text.Len() > 0 {
			msg.Parts = append(msg.Parts, provider.Part{Type: provider.PartText, Text: text.String()})
		}
		for _, c := range calls {
			args := c.args.String()
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			msg.Parts = append(msg.Parts, provider.Part{Type: provider.PartToolUse, ToolCallID: c.id, ToolName: c.name, Input: json.RawMessage(args)})
		}
		stop := provider.StopEndTurn
		switch {
		case len(calls) > 0:
			stop = provider.StopToolUse
		case finish == "length":
			stop = provider.StopMaxTokens
		case finish == "content_filter":
			stop = provider.StopRefusal
		}
		send(provider.Event{Type: provider.EventDone, Message: &msg, StopReason: stop, Usage: usage})
	}()
	return ch, nil
}
