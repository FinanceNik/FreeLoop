package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/mcp"
)

// MCP resources and prompts (MCP-007). The user attaches a resource of one
// of the session's servers to a message, like an @ mention of a file, and
// runs a server's prompt as a slash command; both are the user's choice of
// context, so they do not go through the permission broker. Whatever the
// harness, the result reaches it as an ordinary prompt with resources.

// mcpListTimeout bounds asking the session's servers for their lists.
const mcpListTimeout = 5 * time.Second

// ResourceRef names a resource to attach.
type ResourceRef struct {
	Server string `json:"server"`
	URI    string `json:"uri"`
	Name   string `json:"name"`
}

// MCPResourceInfo is a resource one of a session's servers offers.
type MCPResourceInfo struct {
	Server string `json:"server"`
	mcp.Resource
}

// MCPPromptInfo is a prompt one of a session's servers offers; Command is
// its slash command.
type MCPPromptInfo struct {
	Server  string `json:"server"`
	Command string `json:"command"`
	mcp.Prompt
}

// SessionMCPContext is what a session's composer can attach and run.
type SessionMCPContext struct {
	Resources []MCPResourceInfo `json:"resources"`
	Prompts   []MCPPromptInfo   `json:"prompts"`
	// Problems are servers that could not list theirs.
	Problems []string `json:"problems"`
}

// sessionServerIDs are the running servers of a session.
func (e *Engine) sessionServerIDs(ctx context.Context, sid string) ([]string, error) {
	ses, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	src := sessionMCP{e: e, projectID: ses.ProjectID, root: ses.ProjectRoot, profileID: ses.ProfileID}
	var ids []string
	for _, s := range e.sessionServers(src.profile()) {
		if e.mcp.Status(s.ID).State == mcp.Running {
			ids = append(ids, s.ID)
		}
	}
	return ids, nil
}

// SessionMCP lists the resources and prompts of a session's running
// servers.
func (e *Engine) SessionMCP(ctx context.Context, sid string) (SessionMCPContext, error) {
	out := SessionMCPContext{Resources: []MCPResourceInfo{}, Prompts: []MCPPromptInfo{}, Problems: []string{}}
	ids, err := e.sessionServerIDs(ctx, sid)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, mcpListTimeout)
	defer cancel()
	for _, id := range ids {
		res, err := e.mcp.Resources(ctx, id)
		if err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: %v", id, err))
		}
		for _, r := range res {
			out.Resources = append(out.Resources, MCPResourceInfo{Server: id, Resource: r})
		}
		prompts, err := e.mcp.Prompts(ctx, id)
		if err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("%s: %v", id, err))
		}
		for _, p := range prompts {
			out.Prompts = append(out.Prompts, MCPPromptInfo{Server: id, Command: mcpToolName(id, p.Name), Prompt: p})
		}
	}
	return out, nil
}

// readResources reads attached resources for a prompt, under the same size
// limits as @ mentions; those that cannot be read are reported in a notice.
func (e *Engine) readResources(ctx context.Context, r *runtime, refs []ResourceRef) []harness.Resource {
	if len(refs) == 0 {
		return nil
	}
	ids, _ := e.sessionServerIDs(ctx, r.sid)
	var out []harness.Resource
	var skipped []string
	total := 0
	for _, ref := range refs {
		name := ref.Name
		if name == "" {
			name = ref.URI
		}
		label := ref.Server + ": " + name
		var text string
		var err error
		switch {
		case len(out) == maxMentions:
			err = errors.New("too many attachments")
		case !slices.Contains(ids, ref.Server):
			err = errors.New("the server is not running for this session")
		default:
			rctx, cancel := context.WithTimeout(ctx, time.Minute)
			text, err = e.mcp.ReadResource(rctx, ref.Server, ref.URI)
			cancel()
		}
		if err == nil && len(text) > maxMentionBytes {
			err = errors.New(mentionTooLarge)
		}
		if err == nil && total+len(text) > maxMentionTotal {
			err = errors.New(mentionOverTotal)
		}
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s (%v)", label, err))
			continue
		}
		total += len(text)
		out = append(out, harness.Resource{Path: label, URI: ref.URI, Text: text, Server: ref.Server})
	}
	if len(skipped) > 0 {
		e.emit(r.sid, event.Notice, map[string]string{"message": "Not attached: " + strings.Join(skipped, ", ")})
	}
	return out
}

// RunMCPPrompt renders one of a session's servers' prompts with args and
// sends it as the user's message, its embedded resources attached.
func (e *Engine) RunMCPPrompt(ctx context.Context, sid, server, name string, args map[string]string) (PromptResult, error) {
	ids, err := e.sessionServerIDs(ctx, sid)
	if err != nil {
		return PromptResult{}, err
	}
	if !slices.Contains(ids, server) {
		return PromptResult{}, fmt.Errorf("the MCP server %s is not running for this session", server)
	}
	pctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	p, err := e.mcp.GetPrompt(pctx, server, name, args)
	if err != nil {
		return PromptResult{}, fmt.Errorf("the prompt %s of %s: %w", name, server, err)
	}
	// The server wrote this text, so its @ words attach nothing, and what
	// it embeds is held to the limits of the user's own attachments.
	q := queued{text: p.Text, noMentions: true}
	if q.text == "" {
		q.text = "/" + mcpToolName(server, name)
	}
	if len(q.text) > maxMentionTotal {
		return PromptResult{}, fmt.Errorf("the prompt %s of %s is larger than 1 MB", name, server)
	}
	total := len(q.text)
	var skipped []string
	for _, r := range p.Resources {
		switch {
		case len(q.inline) == maxMentions:
			skipped = append(skipped, r.URI+" (too many attachments)")
		case len(r.Text) > maxMentionBytes:
			skipped = append(skipped, r.URI+" ("+mentionTooLarge+")")
		case total+len(r.Text) > maxMentionTotal:
			skipped = append(skipped, r.URI+" ("+mentionOverTotal+")")
		default:
			total += len(r.Text)
			q.inline = append(q.inline, harness.Resource{Path: server + ": " + r.URI, URI: r.URI, Text: r.Text, Server: server})
		}
	}
	if len(skipped) > 0 {
		e.emit(sid, event.Notice, map[string]string{"message": "Not attached from the prompt: " + strings.Join(skipped, ", ")})
	}
	return e.enqueue(ctx, sid, q)
}
