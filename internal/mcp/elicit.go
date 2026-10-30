package mcp

import (
	"context"
	"encoding/json"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Input requests (MCP-008): a server that needs more from the user asks for
// it with an input-required result (the 2026-07-28 Multi Round-Trip Request
// pattern), which the SDK's client fulfils through the elicitation handler
// and retries, or, on older servers, with elicitation/create. Either way
// the request goes to the session whose tool call caused it, which shows
// it to the user; without one it is declined.

// Elicitation is what a server asks for: fields described by a JSON
// schema, or a URL to open.
type Elicitation struct {
	Message string
	Schema  json.RawMessage
	URL     string
}

// Answer is the user's reply: accept with values, decline or cancel.
type Answer struct {
	Action  string
	Content map[string]any
}

// Asker puts a server's request to the user.
type Asker func(ctx context.Context, server string, e Elicitation) (Answer, error)

type askerKey struct{}

// WithAsker makes requests for input during calls made with ctx go to a.
func WithAsker(ctx context.Context, a Asker) context.Context {
	return context.WithValue(ctx, askerKey{}, a)
}

func askerFrom(ctx context.Context) Asker {
	a, _ := ctx.Value(askerKey{}).(Asker)
	return a
}

// elicit is a server's elicitation handler: the asker of the call that is
// waiting (the context of a multi-round-trip retry), else of the call in
// flight on that server (an older server's own request).
func (m *Manager) elicit(id string) func(context.Context, *sdk.ElicitRequest) (*sdk.ElicitResult, error) {
	return func(ctx context.Context, req *sdk.ElicitRequest) (*sdk.ElicitResult, error) {
		a := askerFrom(ctx)
		if a == nil {
			m.mu.Lock()
			if s, ok := m.servers[id]; ok && s.asker != nil {
				a = *s.asker
			}
			m.mu.Unlock()
		}
		if a == nil || req.Params == nil {
			return &sdk.ElicitResult{Action: "decline"}, nil
		}
		e := Elicitation{Message: req.Params.Message, URL: req.Params.URL}
		if req.Params.RequestedSchema != nil {
			e.Schema, _ = json.Marshal(req.Params.RequestedSchema)
		}
		ans, err := a(ctx, id, e)
		if err != nil {
			return &sdk.ElicitResult{Action: "cancel"}, nil
		}
		return &sdk.ElicitResult{Action: ans.Action, Content: ans.Content}, nil
	}
}
