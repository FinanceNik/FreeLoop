// Package scripted is a provider that plays a fixed script. Tests use it to
// drive the built-in harness without a network; development builds offer a
// demo script so the whole loop can be tried without an API key.
package scripted

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Call is one scripted tool call.
type Call struct {
	Name  string
	Input any
}

// Step is one scripted model response.
type Step struct {
	Text  string
	Calls []Call
	// Stop overrides the stop reason; default tool_use with calls, else end_turn.
	Stop provider.StopReason
}

// Provider plays Steps. Which step answers a request follows from the
// request itself: the number of tool-result messages since the last user
// prompt. So one script serves any number of sessions and survives restarts.
type Provider struct {
	id    string
	steps []Step

	mu       sync.Mutex
	requests []provider.ChatRequest
}

// New returns a provider that answers with steps in order.
func New(id string, steps ...Step) *Provider {
	return &Provider{id: id, steps: steps}
}

func (p *Provider) ID() string          { return p.id }
func (p *Provider) Type() provider.Type { return provider.TypeScripted }

func (p *Provider) ListModels(context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{ID: "scripted", DisplayName: "Scripted demo"}}, nil
}

func (p *Provider) Capabilities(string) provider.ModelCaps {
	return provider.ModelCaps{Source: provider.CapsStatic, Tools: true, ParallelTools: true, StableToolCallIDs: true, ContextWindow: 100_000, MaxOutput: 8_000}
}

// Requests returns every request received so far.
func (p *Provider) Requests() []provider.ChatRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.ChatRequest{}, p.requests...)
}

// position counts tool-result messages after the last prompt.
func position(msgs []provider.Message) int {
	n := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != provider.RoleUser {
			continue
		}
		if len(m.Parts) > 0 && m.Parts[0].Type == provider.PartToolResult {
			n++
			continue
		}
		break
	}
	return n
}

func (p *Provider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()

	i := position(req.Messages)
	step := Step{Text: "(The script has no more steps.)"}
	if i < len(p.steps) {
		step = p.steps[i]
	}
	turn := len(p.Requests())

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
		msg := provider.Message{Role: provider.RoleAssistant}
		if step.Text != "" {
			for _, w := range strings.SplitAfter(step.Text, " ") {
				if !send(provider.Event{Type: provider.EventTextDelta, Text: w}) {
					return
				}
			}
			msg.Parts = append(msg.Parts, provider.Part{Type: provider.PartText, Text: step.Text})
		}
		for n, c := range step.Calls {
			id := fmt.Sprintf("call_%d_%d", turn, n)
			input, err := json.Marshal(c.Input)
			if err != nil {
				send(provider.Event{Type: provider.EventError, Err: err})
				return
			}
			if !send(provider.Event{Type: provider.EventToolCallStart, ToolCallID: id, ToolName: c.Name}) {
				return
			}
			msg.Parts = append(msg.Parts, provider.Part{Type: provider.PartToolUse, ToolCallID: id, ToolName: c.Name, Input: input})
		}
		stop := step.Stop
		if stop == "" {
			stop = provider.StopEndTurn
			if len(step.Calls) > 0 {
				stop = provider.StopToolUse
			}
		}
		if ctx.Err() != nil {
			send(provider.Event{Type: provider.EventError, Err: ctx.Err()})
			return
		}
		send(provider.Event{Type: provider.EventDone, Message: &msg, StopReason: stop,
			Usage: provider.Usage{InputTokens: 100, OutputTokens: int64(len(step.Text)/4 + 10)}})
	}()
	return ch, nil
}

// Demo is the development demo: look around, write a file (which asks for
// approval in ask mode), run a command, and summarize.
func Demo(id string) *Provider {
	return New(id,
		Step{Text: "I'll start by looking at the project.", Calls: []Call{{Name: "list_dir", Input: map[string]any{"path": "."}}}},
		Step{Text: "Now I'll write a small file.", Calls: []Call{{Name: "write_file", Input: map[string]any{
			"path": "freeloop-demo.txt", "content": "Written by the Freeloop scripted demo.\n",
		}}}},
		Step{Text: "And run a command to check it.", Calls: []Call{{Name: "shell", Input: map[string]any{"command": "ls -la freeloop-demo.txt && cat freeloop-demo.txt"}}}},
		Step{Text: "Done. This was the scripted demo provider: it listed the project, wrote `freeloop-demo.txt` and ran `ls`. Add an Anthropic provider to work with a real model."},
	)
}

// Steps returns the script.
func (p *Provider) Steps() []Step { return append([]Step{}, p.steps...) }
