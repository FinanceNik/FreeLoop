package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/builtin"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// Switching profile or harness mid-session (HRN-012): a new session with
// the other profile, in the same project, environment and folder, told
// about the old one through a handoff. A built-in model (the old session's,
// else the new profile's) summarises the old transcript; without one, the
// end of the transcript itself is the handoff. The new session's first
// prompt carries it as an attached resource, so it reaches any harness.

// handoffInput caps the transcript a summary is written from, and
// handoffExcerpt the transcript passed on when there is no model.
const (
	handoffInput   = 100 << 10
	handoffExcerpt = 20 << 10
)

const handoffPrompt = `You are handing a coding session over to another agent that will continue it. Below is the transcript so far. Write a handoff for that agent, in the second person, that lets it carry on without the transcript:

- the user's goal and every request and constraint they gave;
- what was done: files read, created or changed, by path, and commands run with their results;
- decisions made and why, and what was tried and failed;
- the current state and the next steps.

Be specific: paths, names, numbers, error messages. Reply with the handoff only.`

// SwitchProfile continues sid in a new session with profileID and returns
// it.
func (e *Engine) SwitchProfile(ctx context.Context, sid, profileID string) (session.Session, error) {
	src, err := e.store.GetSession(ctx, sid)
	if err != nil {
		return session.Session{}, err
	}
	e.mu.Lock()
	r := e.runtimes[sid]
	e.mu.Unlock()
	if r != nil {
		r.mu.Lock()
		busy, done := r.busy, r.done
		r.mu.Unlock()
		// A turn that is just ending gets a moment to finish.
		if busy {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				return session.Session{}, errors.New("stop the running turn before switching")
			}
		}
	}
	events, err := e.store.Events(ctx, sid, 0)
	if err != nil {
		return session.Session{}, err
	}
	project := e.sessionProject(ctx, src)
	target, err := e.profileFor(project, profileID)
	if err != nil {
		return session.Session{}, err
	}
	summary, usage, generated := e.handoffSummary(ctx, src, target.Harness, target.Provider, target.Model, renderMarkdown(src, events))

	req := CreateSessionRequest{Title: src.Title + " · " + target.Name, ProjectRoot: src.ProjectRoot, EnvironmentID: src.EnvironmentID, ProfileID: profileID, optimizer: cmp.Or(src.Optimizer, config.OptimizerOff)}
	if src.Worktree != "" {
		req.shareWorktree = &src
	}
	ses, err := e.CreateSession(ctx, req)
	if err != nil {
		return session.Session{}, err
	}
	e.emit(ses.ID, event.Handoff, event.HandoffData{FromSessionID: src.ID, FromTitle: src.Title, Summary: summary, Generated: generated})
	if usage != nil {
		e.emit(ses.ID, event.UsageUpdate, *usage)
	}
	e.emit(sid, event.Notice, event.NoticeData{Message: fmt.Sprintf("Continued in a new session with the profile %s.", target.Name)})
	return ses, nil
}

// handoffSummary writes the handoff: a summary from the old session's
// built-in model, else the new profile's, else the end of the transcript.
func (e *Engine) handoffSummary(ctx context.Context, src session.Session, harnessID, providerID, model, transcript string) (string, *event.UsageData, bool) {
	if len(transcript) > handoffInput {
		transcript = "[the start of the transcript is left out]\n" + transcript[len(transcript)-handoffInput:]
	}
	type choice struct{ provider, model string }
	var choices []choice
	if src.HarnessID == builtin.ID && src.ProviderID != "" {
		choices = append(choices, choice{src.ProviderID, src.Model})
	}
	if (harnessID == "" || harnessID == builtin.ID) && providerID != "" && model != "" {
		choices = append(choices, choice{providerID, model})
	}
	for _, c := range choices {
		cp, err := e.provider(c.provider)
		if err != nil {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		text, u, err := complete(cctx, e.limit(cp, src.ID), provider.ChatRequest{Model: c.model, System: handoffPrompt,
			Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{{Type: provider.PartText, Text: transcript}}}}})
		cancel()
		if err == nil && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), &event.UsageData{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
				CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens, ProviderID: c.provider, Model: c.model}, true
		}
	}
	if len(transcript) > handoffExcerpt {
		transcript = "[earlier parts left out]\n" + transcript[len(transcript)-handoffExcerpt:]
	}
	return transcript, nil, false
}

// complete runs one model call and returns its text and usage.
func complete(ctx context.Context, p provider.Provider, req provider.ChatRequest) (string, provider.Usage, error) {
	ch, err := p.Stream(ctx, req)
	if err != nil {
		return "", provider.Usage{}, err
	}
	for ev := range ch {
		switch ev.Type {
		case provider.EventDone:
			var sb strings.Builder
			if ev.Message != nil {
				for _, part := range ev.Message.Parts {
					if part.Type == provider.PartText {
						sb.WriteString(part.Text)
					}
				}
			}
			return sb.String(), ev.Usage, nil
		case provider.EventError:
			return "", provider.Usage{}, ev.Err
		}
	}
	if ctx.Err() != nil {
		return "", provider.Usage{}, ctx.Err()
	}
	return "", provider.Usage{}, errors.New("the provider stream ended without a response")
}

// pendingHandoff is the handoff a session's first prompt carries, or nil
// once a message was sent.
func pendingHandoff(events []event.Event) *harness.Resource {
	var h *event.HandoffData
	for _, ev := range events {
		switch ev.Type {
		case event.UserMessageChunk:
			return nil
		case event.Handoff:
			var d event.HandoffData
			if json.Unmarshal(ev.Data, &d) == nil {
				h = &d
			}
		}
	}
	if h == nil {
		return nil
	}
	return &harness.Resource{Path: "Handoff from " + h.FromTitle, URI: "freeloop://session/" + h.FromSessionID, Text: h.Summary}
}
