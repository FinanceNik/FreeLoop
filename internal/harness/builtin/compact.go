package builtin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

// Context compaction (HRN-013): when a model call's context reaches
// compactAt of the model's window, or on /compact, the model summarises the
// conversation and the session continues from the summary. The current turn
// is kept as it is when it is small enough; the transcript keeps everything
// and records the compaction, and history_message events with reset set
// tell Restore to start over.

// CompactCommand is the slash command that compacts on request.
const CompactCommand = "compact"

// compactAt is the share of the context window that triggers compaction.
const compactAt = 0.8

// keepTurnAt caps how much of the window the kept current turn may use.
const keepTurnAt = 0.25

const compactPrompt = `You are compacting the history of a coding session so the work can continue in a fresh context. Write a summary for yourself that keeps everything needed to carry on:

- the user's goal and every request and constraint they gave, in their words where it matters;
- decisions made and why;
- files read, created or changed, with what changed, by path;
- commands run and their results, errors included;
- the current state and the next steps.

Be specific: paths, names, numbers, error messages. Do not call tools. Reply with the summary only.`

// needsCompaction reports whether the next model call's context passes
// compactAt of the window: the last call's context plus an estimate of what
// was added since, so one large tool result cannot push a call past the
// window unnoticed.
func (s *session) needsCompaction() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.window > 0 && float64(s.projected()) >= compactAt*float64(s.window) && len(s.history) > 2
}

// projected is the context the next call would send. Callers hold s.mu.
func (s *session) projected() int64 {
	since := s.history[min(s.contextLen, len(s.history)):]
	if s.context == 0 {
		since = s.history
	}
	return s.context + estimate(since)
}

// summaryBudget is the share of the window a summary request may fill;
// larger tool results are cut to fit, the longest first.
const summaryBudget = 0.7

// fitForSummary shortens tool results until msgs fit budget tokens (by
// estimate), so a history too big for the window can still be summarised.
func fitForSummary(msgs []provider.Message, budget int64) []provider.Message {
	if budget <= 0 || estimate(msgs) <= budget {
		return msgs
	}
	out := make([]provider.Message, len(msgs))
	for i, m := range msgs {
		out[i] = m
		out[i].Parts = slices.Clone(m.Parts)
	}
	const keep = 1000
	for estimate(out) > budget {
		mi, pi, longest := -1, -1, keep
		for i, m := range out {
			for j, p := range m.Parts {
				if p.Type == provider.PartToolResult && len(p.Text) > longest {
					mi, pi, longest = i, j, len(p.Text)
				}
			}
		}
		if mi < 0 {
			break
		}
		p := &out[mi].Parts[pi]
		p.Text = p.Text[:keep] + fmt.Sprintf("\n[%d more characters left out to fit the summary]", len(p.Text)-keep)
	}
	return out
}

// estimate is a rough token count of messages: four characters a token.
func estimate(msgs []provider.Message) int64 {
	n := 0
	for _, m := range msgs {
		for _, p := range m.Parts {
			n += len(p.Text) + len(p.Input) + len(p.ToolName) + 16
		}
	}
	return int64(n / 4)
}

// compact summarises the history and replaces it with the summary, plus the
// current turn when that is small.
func (s *session) compact(ctx context.Context, reason string) error {
	s.mu.Lock()
	before, window := s.projected(), s.window
	history := slices.Clone(s.history)
	s.mu.Unlock()
	if len(history) == 0 {
		return errors.New("there is nothing to compact yet")
	}
	s.p.Emit(event.CompactionUpdate, event.CompactionData{Status: "started", Reason: reason, ContextTokens: before})

	// The summary request: the history, then the instruction as the last
	// user message.
	ask := provider.Part{Type: provider.PartText, Text: "Summarise the session so far as instructed."}
	msgs := slices.Clone(history)
	if last := len(msgs) - 1; msgs[last].Role == provider.RoleUser {
		m := msgs[last]
		m.Parts = append(slices.Clone(m.Parts), ask)
		msgs[last] = m
	} else {
		msgs = append(msgs, provider.Message{Role: provider.RoleUser, Parts: []provider.Part{ask}})
	}
	if window > 0 {
		msgs = fitForSummary(msgs, int64(summaryBudget*float64(window)))
	}
	prov, model := s.current()
	summary, err := s.complete(ctx, prov, provider.ChatRequest{Model: model, System: compactPrompt, Messages: msgs, Tools: s.toolSpecs(s.Mode()), Effort: s.p.Effort})
	if err != nil {
		return err
	}
	if strings.TrimSpace(summary) == "" {
		return errors.New("the model returned an empty summary")
	}

	head := provider.Part{Type: provider.PartText, Text: fmt.Sprintf("<summary of the session so far>\n%s\n</summary>\n\nThe conversation above was compacted into this summary. Continue from it.", strings.TrimSpace(summary))}
	next := []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{head}}}
	// Mid-turn, keep the turn in progress (from its prompt on) when it
	// fits; it starts with the user's message, which joins the summary
	// message. Between turns (/compact) nothing is in progress.
	if start := currentTurn(history); reason == "auto" && start >= 0 && window > 0 && float64(estimate(history[start:])) < keepTurnAt*float64(window) {
		next[0].Parts = append(next[0].Parts, history[start].Parts...)
		next = append(next, history[start+1:]...)
	} else if reason == "auto" {
		next[0].Parts = append(next[0].Parts, provider.Part{Type: provider.PartText, Text: "Continue the task."})
	}
	s.replaceHistory(next)
	after := estimate(next)
	s.mu.Lock()
	s.context, s.contextLen = after, len(next)
	s.mu.Unlock()
	s.p.Emit(event.CompactionUpdate, event.CompactionData{Status: "completed", Reason: reason, ContextTokens: before, AfterTokens: after, Summary: strings.TrimSpace(summary)})
	if window > 0 && after >= int64(window) {
		return errors.New("the conversation does not fit the model's context even after compaction")
	}
	return nil
}

// currentTurn returns the index of the user message that started the turn
// in progress, or -1.
func currentTurn(history []provider.Message) int {
	for i := len(history) - 1; i >= 0; i-- {
		m := history[i]
		if m.Role == provider.RoleUser && len(m.Parts) > 0 && m.Parts[0].Type != provider.PartToolResult {
			return i
		}
	}
	return -1
}

// replaceHistory swaps the whole history and records it so Restore
// rebuilds the compacted one.
func (s *session) replaceHistory(msgs []provider.Message) {
	s.mu.Lock()
	s.history = slices.Clone(msgs)
	s.mu.Unlock()
	for i, m := range msgs {
		d := map[string]any{"index": i, "message": m}
		if i == 0 {
			d["reset"] = true
		}
		s.p.Emit(event.HistoryMessage, d)
	}
}

// complete runs one model call without streaming it into the transcript
// and returns its text. Its tokens are recorded for cost estimates.
func (s *session) complete(ctx context.Context, prov provider.Provider, req provider.ChatRequest) (string, error) {
	ch, err := prov.Stream(ctx, req)
	if err != nil {
		return "", err
	}
	for ev := range ch {
		switch ev.Type {
		case provider.EventDone:
			u := ev.Usage
			s.p.Emit(event.UsageUpdate, event.UsageData{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
				CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens, ProviderID: prov.ID(), Model: req.Model})
			var sb strings.Builder
			if ev.Message != nil {
				for _, p := range ev.Message.Parts {
					if p.Type == provider.PartText {
						sb.WriteString(p.Text)
					}
				}
			}
			return sb.String(), nil
		case provider.EventError:
			return "", ev.Err
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return "", errors.New("the provider stream ended without a response")
}
