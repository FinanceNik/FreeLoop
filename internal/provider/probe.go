package provider

import (
	"context"
	"errors"
	"time"
)

// Describer is implemented by providers whose endpoint can report a
// model's capabilities, such as Ollama's /api/show.
type Describer interface {
	Describe(ctx context.Context, model string) (ModelCaps, error)
}

// ProbeTimeout bounds a tool-calling probe; local runtimes may load the
// model first.
const ProbeTimeout = 2 * time.Minute

// ProbeTools asks model to call a trivial tool and reports whether it came
// back as a structured tool call (PRV-005). A model that answers in text, or
// writes the call into its text, fails. Errors mean the probe could not run.
func ProbeTools(ctx context.Context, p Provider, model string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	ch, err := p.Stream(ctx, ChatRequest{
		Model:     model,
		System:    "You are being tested for tool use. Use the provided tool when asked.",
		MaxTokens: 256,
		Messages: []Message{{Role: RoleUser, Parts: []Part{{Type: PartText,
			Text: `Call the tool "ping" with value "ok". Do not reply with text.`}}}},
		Tools: []ToolSpec{{
			Name:        "ping",
			Description: "Answers a ping.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"value": map[string]any{"type": "string"}},
				"required":   []string{"value"},
			},
		}},
	})
	if err != nil {
		return false, err
	}
	for ev := range ch {
		switch ev.Type {
		case EventDone:
			for _, c := range ev.Message.ToolCalls() {
				if c.ToolName == "ping" {
					return true, nil
				}
			}
			return false, nil
		case EventError:
			return false, ev.Err
		}
	}
	if ctx.Err() != nil {
		return false, errors.New("the probe timed out")
	}
	return false, errors.New("the provider stream ended without a response")
}
