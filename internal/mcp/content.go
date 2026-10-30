package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Resources and prompts (MCP-007). Lists are read from the server when
// asked, so they are never stale; a server that does not offer resources or
// prompts is skipped.

// Resource is a server's resource as the composer lists it.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// Prompt is a server's prompt template.
type Prompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments"`
}

// PromptArgument is one of a prompt's arguments.
type PromptArgument struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

// PromptText is a prompt's messages as one user message, plus the
// resources they embed.
type PromptText struct {
	Text      string
	Resources []ResourceText
}

// ResourceText is a resource's text content.
type ResourceText struct {
	URI  string
	Text string
}

// errUnsupported marks a server without the capability asked for.
var errUnsupported = errors.New("not offered by the server")

// running returns a running server's session and capabilities.
func (m *Manager) running(id string) (*sdk.ClientSession, *sdk.ServerCapabilities, error) {
	m.mu.Lock()
	s, ok := m.servers[id]
	var cs *sdk.ClientSession
	if ok && s.status.State == Running {
		cs = s.session
	}
	m.mu.Unlock()
	if cs == nil {
		return nil, nil, fmt.Errorf("the MCP server %s is not running", id)
	}
	caps := &sdk.ServerCapabilities{}
	if ir := cs.InitializeResult(); ir != nil && ir.Capabilities != nil {
		caps = ir.Capabilities
	}
	return cs, caps, nil
}

// Resources lists a running server's resources; none when it offers none.
func (m *Manager) Resources(ctx context.Context, id string) ([]Resource, error) {
	cs, caps, err := m.running(id)
	if err != nil {
		return nil, err
	}
	out := []Resource{}
	if caps.Resources == nil {
		return out, nil
	}
	params := &sdk.ListResourcesParams{}
	for range 20 {
		res, err := cs.ListResources(ctx, params)
		if err != nil {
			return nil, err
		}
		for _, r := range res.Resources {
			out = append(out, Resource{URI: r.URI, Name: r.Name, Title: r.Title, Description: r.Description, MIMEType: r.MIMEType})
		}
		if res.NextCursor == "" {
			break
		}
		params = &sdk.ListResourcesParams{Cursor: res.NextCursor}
	}
	return out, nil
}

// Prompts lists a running server's prompts; none when it offers none.
func (m *Manager) Prompts(ctx context.Context, id string) ([]Prompt, error) {
	cs, caps, err := m.running(id)
	if err != nil {
		return nil, err
	}
	out := []Prompt{}
	if caps.Prompts == nil {
		return out, nil
	}
	params := &sdk.ListPromptsParams{}
	for range 20 {
		res, err := cs.ListPrompts(ctx, params)
		if err != nil {
			return nil, err
		}
		for _, p := range res.Prompts {
			pr := Prompt{Name: p.Name, Title: p.Title, Description: p.Description, Arguments: []PromptArgument{}}
			for _, a := range p.Arguments {
				pr.Arguments = append(pr.Arguments, PromptArgument{Name: a.Name, Title: a.Title, Description: a.Description, Required: a.Required})
			}
			out = append(out, pr)
		}
		if res.NextCursor == "" {
			break
		}
		params = &sdk.ListPromptsParams{Cursor: res.NextCursor}
	}
	return out, nil
}

// ReadResource returns a resource's text. Binary contents are left out
// with a note; a resource with no text at all is an error.
func (m *Manager) ReadResource(ctx context.Context, id, uri string) (string, error) {
	cs, caps, err := m.running(id)
	if err != nil {
		return "", err
	}
	if caps.Resources == nil {
		return "", fmt.Errorf("resources are %w %s", errUnsupported, id)
	}
	res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: uri})
	if err != nil {
		return "", err
	}
	if len(res.InputRequests) > 0 {
		return "", errors.New("the server asks for more input, which Freeloop does not answer yet")
	}
	var parts []string
	text := false
	for _, c := range res.Contents {
		switch {
		case c.Text != "" || len(c.Blob) == 0:
			parts = append(parts, c.Text)
			text = true
		case utf8.Valid(c.Blob) && !strings.ContainsRune(string(c.Blob), 0):
			parts = append(parts, string(c.Blob))
			text = true
		default:
			parts = append(parts, fmt.Sprintf("[binary content of %s (%s, %d bytes) left out]", c.URI, c.MIMEType, len(c.Blob)))
		}
	}
	if !text {
		return "", errors.New("the resource has no text content")
	}
	return strings.Join(parts, "\n"), nil
}

// GetPrompt renders a prompt with its arguments.
func (m *Manager) GetPrompt(ctx context.Context, id, name string, args map[string]string) (PromptText, error) {
	cs, caps, err := m.running(id)
	if err != nil {
		return PromptText{}, err
	}
	if caps.Prompts == nil {
		return PromptText{}, fmt.Errorf("prompts are %w %s", errUnsupported, id)
	}
	res, err := cs.GetPrompt(ctx, &sdk.GetPromptParams{Name: name, Arguments: args})
	if err != nil {
		return PromptText{}, err
	}
	if len(res.InputRequests) > 0 {
		return PromptText{}, errors.New("the server asks for more input, which Freeloop does not answer yet")
	}
	var out PromptText
	var texts []string
	for _, msg := range res.Messages {
		prefix := ""
		if msg.Role == "assistant" {
			prefix = "Assistant: "
		}
		switch c := msg.Content.(type) {
		case *sdk.TextContent:
			texts = append(texts, prefix+c.Text)
		case *sdk.EmbeddedResource:
			if c.Resource != nil && c.Resource.Text != "" {
				out.Resources = append(out.Resources, ResourceText{URI: c.Resource.URI, Text: c.Resource.Text})
			}
		case *sdk.ResourceLink:
			texts = append(texts, prefix+"See "+c.URI)
		default:
			texts = append(texts, prefix+"[content the app cannot show left out]")
		}
	}
	out.Text = strings.TrimSpace(strings.Join(texts, "\n\n"))
	if out.Text == "" && len(out.Resources) == 0 {
		return PromptText{}, errors.New("the prompt is empty")
	}
	return out, nil
}
