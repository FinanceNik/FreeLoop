// Package ollama adapts Ollama's native API (/api/chat, /api/tags,
// /api/show) to provider.Provider (OQ-011). Ollama reports per-model
// capabilities, so tool support is known without a probe for current
// versions; tool calls arrive whole and without IDs, which are synthesized.
package ollama

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
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Type is this adapter's provider type.
const Type provider.Type = "ollama"

// DefaultBaseURL is a local Ollama.
const DefaultBaseURL = "http://localhost:11434"

// Options configures New.
type Options struct {
	ID         string
	BaseURL    string
	HTTPClient *http.Client
}

// Provider talks to one Ollama server.
type Provider struct {
	id     string
	base   string
	client *http.Client
	calls  atomic.Int64

	mu   sync.Mutex
	caps map[string]provider.ModelCaps
	// thinks records models that report the thinking capability.
	thinks map[string]bool
}

// New returns a provider for opts.
func New(opts Options) *Provider {
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	return &Provider{id: opts.ID, base: base, client: client, caps: map[string]provider.ModelCaps{}, thinks: map[string]bool{}}
}

func (p *Provider) ID() string          { return p.id }
func (p *Provider) Type() provider.Type { return Type }

// Capabilities returns what Describe found, or unknown.
func (p *Provider) Capabilities(model string) provider.ModelCaps {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.caps[model]; ok {
		return c
	}
	return provider.ModelCaps{Source: provider.CapsUnknown}
}

func (p *Provider) post(ctx context.Context, path string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return p.send(req)
}

func (p *Provider) send(req *http.Request) (*http.Response, error) {
	resp, err := p.client.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			return nil, fmt.Errorf("could not connect to Ollama at %s; is it running?", p.base)
		}
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		var e struct{ Error string }
		msg := strings.TrimSpace(string(b))
		if json.Unmarshal(b, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return nil, provider.NewHTTPError(resp, fmt.Sprintf("Ollama returned %d: %s", resp.StatusCode, msg))
	}
	return resp, nil
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.send(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("unexpected /api/tags response: %w", err)
	}
	out := make([]provider.ModelInfo, 0, len(body.Models))
	for _, m := range body.Models {
		id := m.Model
		if id == "" {
			id = m.Name
		}
		out = append(out, provider.ModelInfo{ID: id, DisplayName: m.Name})
	}
	slices.SortFunc(out, func(a, b provider.ModelInfo) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// Describe asks Ollama what a model can do (/api/show). Versions that do
// not report capabilities leave tool support unknown, for a probe to settle.
func (p *Provider) Describe(ctx context.Context, model string) (provider.ModelCaps, error) {
	resp, err := p.post(ctx, "/api/show", map[string]any{"model": model})
	if err != nil {
		return provider.ModelCaps{}, err
	}
	defer resp.Body.Close()
	var body struct {
		Capabilities []string       `json:"capabilities"`
		ModelInfo    map[string]any `json:"model_info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return provider.ModelCaps{}, fmt.Errorf("unexpected /api/show response: %w", err)
	}
	c := provider.ModelCaps{Source: provider.CapsUnknown}
	if body.Capabilities != nil {
		c.Source = provider.CapsReported
		c.Tools = slices.Contains(body.Capabilities, "tools")
		c.Reasoning = slices.Contains(body.Capabilities, "thinking")
		// Calls arrive whole, possibly several per response.
		c.ParallelTools = c.Tools
	}
	for k, v := range body.ModelInfo {
		if strings.HasSuffix(k, ".context_length") {
			if n, ok := v.(float64); ok {
				c.ContextWindow = int(n)
			}
		}
	}
	p.mu.Lock()
	p.caps[model] = c
	p.thinks[model] = c.Reasoning
	p.mu.Unlock()
	return c, nil
}

type wireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Images are base64 pictures for a user message (SES-015).
	Images    []string   `json:"images,omitempty"`
	ToolCalls []wireCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
}

type wireCall struct {
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

// toWire converts neutral history. Tool results carry the tool's name,
// looked up from the call they answer.
func toWire(system string, msgs []provider.Message) []wireMessage {
	out := []wireMessage{}
	if system != "" {
		out = append(out, wireMessage{Role: "system", Content: system})
	}
	names := map[string]string{}
	for _, m := range msgs {
		if m.Role == provider.RoleAssistant {
			w := wireMessage{Role: "assistant"}
			for _, part := range m.Parts {
				switch part.Type {
				case provider.PartText:
					w.Content += part.Text
				case provider.PartToolUse:
					names[part.ToolCallID] = part.ToolName
					var c wireCall
					c.Function.Name = part.ToolName
					c.Function.Arguments = part.Input
					if !json.Valid(c.Function.Arguments) {
						c.Function.Arguments = json.RawMessage(`{}`)
					}
					w.ToolCalls = append(w.ToolCalls, c)
				}
			}
			out = append(out, w)
			continue
		}
		var text []string
		for _, part := range m.Parts {
			switch part.Type {
			case provider.PartToolResult:
				content := part.Text
				if part.IsError {
					content = "Error: " + content
				}
				out = append(out, wireMessage{Role: "tool", Content: content, ToolName: names[part.ToolCallID]})
			case provider.PartText:
				text = append(text, part.Text)
			}
		}
		var images []string
		for _, img := range m.Images() {
			images = append(images, img.Base64())
		}
		if len(text) > 0 || len(images) > 0 {
			out = append(out, wireMessage{Role: "user", Content: strings.Join(text, "\n\n"), Images: images})
		}
	}
	return out
}

type chatChunk struct {
	Message struct {
		Content   string     `json:"content"`
		Thinking  string     `json:"thinking"`
		ToolCalls []wireCall `json:"tool_calls"`
	} `json:"message"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int64  `json:"prompt_eval_count"`
	EvalCount       int64  `json:"eval_count"`
	Error           string `json:"error"`
}

func (p *Provider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	body := map[string]any{
		"model":    req.Model,
		"messages": toWire(req.System, req.Messages),
		"stream":   true,
	}
	p.mu.Lock()
	if p.thinks[req.Model] {
		body["think"] = true
	}
	p.mu.Unlock()
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
				"name": t.Name, "description": t.Description, "parameters": t.InputSchema,
			}})
		}
		body["tools"] = tools
	}
	if req.MaxTokens > 0 {
		body["options"] = map[string]any{"num_predict": req.MaxTokens}
	}
	resp, err := p.post(ctx, "/api/chat", body)
	if err != nil {
		return nil, err
	}
	turn := p.calls.Add(1)

	ch := make(chan provider.Event, 64)
	go func() {
		defer close(ch)
		defer provider.RecoverStream(ch)
		defer resp.Body.Close()
		send := func(ev provider.Event) bool {
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		var text, thinking strings.Builder
		var calls []provider.Part
		var final chatChunk
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			var c chatChunk
			if err := json.Unmarshal(line, &c); err != nil {
				send(provider.Event{Type: provider.EventError, Err: fmt.Errorf("unexpected stream line: %w", err)})
				return
			}
			if c.Error != "" {
				send(provider.Event{Type: provider.EventError, Err: errors.New(c.Error)})
				return
			}
			if c.Message.Thinking != "" {
				thinking.WriteString(c.Message.Thinking)
				if !send(provider.Event{Type: provider.EventThinkingDelta, Text: c.Message.Thinking}) {
					return
				}
			}
			if c.Message.Content != "" {
				text.WriteString(c.Message.Content)
				if !send(provider.Event{Type: provider.EventTextDelta, Text: c.Message.Content}) {
					return
				}
			}
			for _, tc := range c.Message.ToolCalls {
				id := tc.ID
				if id == "" {
					id = fmt.Sprintf("call_%d_%d", turn, len(calls))
				}
				args := tc.Function.Arguments
				if len(args) == 0 || string(args) == "null" {
					args = json.RawMessage(`{}`)
				}
				calls = append(calls, provider.Part{Type: provider.PartToolUse, ToolCallID: id, ToolName: tc.Function.Name, Input: args})
				if !send(provider.Event{Type: provider.EventToolCallStart, ToolCallID: id, ToolName: tc.Function.Name}) {
					return
				}
			}
			if c.Done {
				final = c
				break
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
		if !final.Done {
			send(provider.Event{Type: provider.EventError, Err: errors.New("Ollama's stream ended early")})
			return
		}
		msg := provider.Message{Role: provider.RoleAssistant}
		if thinking.Len() > 0 {
			msg.Parts = append(msg.Parts, provider.Part{Type: provider.PartThinking, Text: thinking.String(), Provider: Type})
		}
		if text.Len() > 0 {
			msg.Parts = append(msg.Parts, provider.Part{Type: provider.PartText, Text: text.String()})
		}
		msg.Parts = append(msg.Parts, calls...)
		stop := provider.StopEndTurn
		switch {
		case len(calls) > 0:
			stop = provider.StopToolUse
		case final.DoneReason == "length":
			stop = provider.StopMaxTokens
		}
		send(provider.Event{Type: provider.EventDone, Message: &msg, StopReason: stop,
			Usage: provider.Usage{InputTokens: final.PromptEvalCount, OutputTokens: final.EvalCount}})
	}()
	return ch, nil
}
