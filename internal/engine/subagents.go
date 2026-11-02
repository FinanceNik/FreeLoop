package engine

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Subagents (HRN-014): the built-in harness's task tool starts a child
// session in the parent's project, environment and folder (a worktree is
// shared), with its profile, model and mode. The child's tool calls go
// through the permission broker as its own, so their approvals show in the
// child session and the attention view; the parent waits for the child's
// turn and gets its final reply. A child gets no task tool of its own.

// sessionSubagents is a session's harness.Subagents.
type sessionSubagents struct {
	e   *Engine
	sid string
}

func (s sessionSubagents) Run(ctx context.Context, description, prompt string) (harness.SubagentResult, error) {
	e := s.e
	parent, err := e.store.GetSession(ctx, s.sid)
	if err != nil {
		return harness.SubagentResult{}, err
	}
	req := CreateSessionRequest{
		Title: "Subagent: " + strings.TrimSpace(description), ProjectRoot: parent.ProjectRoot, EnvironmentID: parent.EnvironmentID,
		ProfileID: parent.ProfileID, ProviderID: parent.ProviderID, Model: parent.Model, Mode: parent.Mode, parent: parent.ID, optimizer: parent.Optimizer,
	}
	if parent.Worktree != "" {
		req.shareWorktree = &parent
	}
	child, err := e.CreateSession(ctx, req)
	if err != nil {
		return harness.SubagentResult{}, err
	}
	// The model wrote this text, so its @ words attach nothing.
	if _, err := e.enqueue(ctx, child.ID, queued{text: prompt, noMentions: true}); err != nil {
		return harness.SubagentResult{SessionID: child.ID}, err
	}
	// The child's runtime, and a container it started, stop until the
	// user prompts it again, also when the parent's turn was stopped.
	defer e.stopRuntime(child.ID)
	e.mu.Lock()
	r := e.runtimes[child.ID]
	e.mu.Unlock()
	if r != nil {
		r.mu.Lock()
		done := r.done
		r.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				// The parent's turn was stopped: so is the child's.
				e.stopTurn(child.ID, "Stopped with the turn that started it.")
				<-done
				return harness.SubagentResult{SessionID: child.ID}, ctx.Err()
			}
		}
	}
	events, err := e.store.Events(ctx, child.ID, 0)
	if err != nil {
		return harness.SubagentResult{SessionID: child.ID}, err
	}
	return harness.SubagentResult{SessionID: child.ID, Text: lastReply(events)}, nil
}

// subagentsOf lists the sessions session sid's task tool started.
func (e *Engine) subagentsOf(ctx context.Context, sid string) []session.Session {
	list, err := e.store.ListSessions(ctx)
	if err != nil {
		return nil
	}
	var out []session.Session
	for _, s := range list {
		if s.ParentID == sid {
			out = append(out, s)
		}
	}
	return out
}

// lastReply is the subagent's final answer: the text of the last message
// in its context (the built-in harness records it as history_message),
// else the agent text streamed after the last user message or tool call.
func lastReply(events []event.Event) string {
	var history string
	var streamed strings.Builder
	for _, ev := range events {
		switch ev.Type {
		case event.HistoryMessage:
			var h struct {
				Message provider.Message `json:"message"`
			}
			if json.Unmarshal(ev.Data, &h) == nil && h.Message.Role == provider.RoleAssistant {
				var sb strings.Builder
				for _, p := range h.Message.Parts {
					if p.Type == provider.PartText {
						sb.WriteString(p.Text)
					}
				}
				history = sb.String()
			}
		case event.UserMessageChunk, event.ToolCall:
			streamed.Reset()
		case event.AgentMessageChunk:
			var c event.MessageChunk
			if json.Unmarshal(ev.Data, &c) == nil {
				streamed.WriteString(c.Content.Text)
			}
		}
	}
	if strings.TrimSpace(history) != "" {
		return strings.TrimSpace(history)
	}
	return strings.TrimSpace(streamed.String())
}
