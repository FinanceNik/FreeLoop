package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

// bigContext reports a large context on every call: it lists the folder
// first, then finishes once it sees a summary; summary requests get one.
type bigContext struct {
	mu       sync.Mutex
	requests []provider.ChatRequest
}

func (b *bigContext) ID() string                                               { return "big" }
func (b *bigContext) Type() provider.Type                                      { return provider.TypeScripted }
func (b *bigContext) ListModels(context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (b *bigContext) Capabilities(string) provider.ModelCaps                   { return provider.ModelCaps{Tools: true} }

func (b *bigContext) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	b.mu.Lock()
	b.requests = append(b.requests, req)
	b.mu.Unlock()
	msg := provider.Message{Role: provider.RoleAssistant}
	stop := provider.StopEndTurn
	all, _ := json.Marshal(req.Messages)
	switch {
	case req.System == compactPrompt:
		msg.Parts = []provider.Part{{Type: provider.PartText, Text: "The user asked to look at the project; list_dir showed it."}}
	case strings.Contains(string(all), "summary of the session"):
		msg.Parts = []provider.Part{{Type: provider.PartText, Text: "done"}}
	default:
		msg.Parts = []provider.Part{{Type: provider.PartToolUse, ToolCallID: "c1", ToolName: "list_dir", Input: json.RawMessage(`{"path":"."}`)}}
		stop = provider.StopToolUse
	}
	ch := make(chan provider.Event, 1)
	ch <- provider.Event{Type: provider.EventDone, Message: &msg, StopReason: stop, Usage: provider.Usage{InputTokens: 900, OutputTokens: 10}}
	close(ch)
	return ch, nil
}

// HRN-013: past 80 % of the window the session compacts and goes on from
// the summary; the transcript records it, a restart restores the
// compacted history, and /compact compacts on request.
func TestCompaction(t *testing.T) {
	f := setup(t, permission.ModeAsk)
	prov := &bigContext{}
	s := f.sess.(*session)
	s.SetModel(prov, "m", 1000)

	reason, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "look at the project"})
	if err != nil || reason != harness.StopEndTurn {
		t.Fatalf("Prompt = %s, %v", reason, err)
	}
	var updates []event.CompactionData
	for _, ev := range f.log.all() {
		if ev.Type == event.CompactionUpdate {
			var d event.CompactionData
			json.Unmarshal(ev.Data, &d)
			updates = append(updates, d)
		}
	}
	if len(updates) != 2 || updates[0].Status != "started" || updates[1].Status != "completed" || updates[1].Reason != "auto" || !strings.Contains(updates[1].Summary, "list_dir showed it") {
		t.Fatalf("compaction events = %+v", updates)
	}
	last := prov.requests[len(prov.requests)-1]
	if first, _ := json.Marshal(last.Messages[0]); !strings.Contains(string(first), "summary of the session") {
		t.Errorf("the call after compaction starts with %s", first)
	}
	restored := Restore(f.log.all())
	if len(restored) != len(s.history) || !strings.Contains(restored[0].Parts[0].Text, "summary of the session") {
		t.Errorf("restored %d messages, live %d: %+v", len(restored), len(s.history), restored)
	}
	var usage event.UsageData
	for _, ev := range f.log.all() {
		if ev.Type == event.UsageUpdate {
			json.Unmarshal(ev.Data, &usage)
		}
	}
	if usage.ContextTokens != 900 || usage.ContextWindow != 1000 {
		t.Errorf("usage = %+v", usage)
	}

	// /compact compacts without a model turn of its own.
	n := len(prov.requests)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "/compact"}); err != nil {
		t.Fatal(err)
	}
	if len(prov.requests) != n+1 || prov.requests[n].System != compactPrompt {
		t.Errorf("/compact made %d calls", len(prov.requests)-n)
	}
	if len(s.history) != 1 {
		t.Errorf("history after /compact = %d messages", len(s.history))
	}
}

// growing reports 75 % of a 1000-token window, then reads a large file:
// the next call would pass the window, so the session compacts first, and
// the summary request is cut to fit.
type growing struct {
	mu       sync.Mutex
	requests []provider.ChatRequest
}

func (g *growing) ID() string                                               { return "grow" }
func (g *growing) Type() provider.Type                                      { return provider.TypeScripted }
func (g *growing) ListModels(context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (g *growing) Capabilities(string) provider.ModelCaps                   { return provider.ModelCaps{Tools: true} }

func (g *growing) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	g.mu.Lock()
	g.requests = append(g.requests, req)
	n := len(g.requests)
	g.mu.Unlock()
	msg := provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{{Type: provider.PartText, Text: "done"}}}
	stop := provider.StopEndTurn
	switch {
	case req.System == compactPrompt:
		msg.Parts[0].Text = "The user asked to read big.txt."
	case n == 1:
		msg.Parts = []provider.Part{{Type: provider.PartToolUse, ToolCallID: "c1", ToolName: "read_file", Input: json.RawMessage(`{"path":"big.txt"}`)}}
		stop = provider.StopToolUse
	}
	ch := make(chan provider.Event, 1)
	ch <- provider.Event{Type: provider.EventDone, Message: &msg, StopReason: stop, Usage: provider.Usage{InputTokens: 750}}
	close(ch)
	return ch, nil
}

func TestCompactsBeforeALargeToolResultOverflows(t *testing.T) {
	f := setup(t, permission.ModeAcceptEdits)
	os.WriteFile(filepath.Join(f.root, "big.txt"), []byte(strings.Repeat("0123456789 ", 1000)), 0o644)
	prov := &growing{}
	s := f.sess.(*session)
	s.SetModel(prov, "m", 1000)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "read big.txt"}); err != nil {
		t.Fatal(err)
	}
	if len(prov.requests) != 3 || prov.requests[1].System != compactPrompt {
		t.Fatalf("calls = %d, second is a summary request: %v", len(prov.requests), len(prov.requests) > 1 && prov.requests[1].System == compactPrompt)
	}
	if got := estimate(prov.requests[1].Messages); got > int64(summaryBudget*1000) {
		t.Errorf("the summary request is %d tokens by estimate, over the budget", got)
	}
	if got := estimate(prov.requests[2].Messages); got >= 1000 {
		t.Errorf("the call after compaction is %d tokens by estimate", got)
	}
}
