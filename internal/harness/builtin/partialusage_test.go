package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

// cutOff reports what it has spent (or not), streams some text and then either
// fails or waits to be stopped.
type cutOff struct {
	usage   *provider.Usage
	fail    error
	started chan struct{}
	once    sync.Once
}

func (c *cutOff) ID() string                                               { return "cut" }
func (c *cutOff) Type() provider.Type                                      { return provider.TypeScripted }
func (c *cutOff) ListModels(context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (c *cutOff) Capabilities(string) provider.ModelCaps                   { return provider.ModelCaps{Tools: true} }

func (c *cutOff) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, 4)
	if c.usage != nil {
		ch <- provider.Event{Type: provider.EventUsage, Usage: provider.Usage{InputTokens: c.usage.InputTokens}}
	}
	ch <- provider.Event{Type: provider.EventTextDelta, Text: "partial "}
	if c.usage != nil {
		// A later report is the running total, not an addition.
		ch <- provider.Event{Type: provider.EventUsage, Usage: *c.usage}
	}
	if c.fail != nil {
		ch <- provider.Event{Type: provider.EventError, Err: c.fail}
		close(ch)
		return ch, nil
	}
	go func() {
		defer close(ch)
		c.once.Do(func() { close(c.started) })
		<-ctx.Done()
	}()
	return ch, nil
}

func usageEvents(f *fixture) []event.UsageData {
	var out []event.UsageData
	for _, ev := range f.log.all() {
		if ev.Type == event.UsageUpdate {
			var d event.UsageData
			json.Unmarshal(ev.Data, &d)
			out = append(out, d)
		}
	}
	return out
}

// PRV-008: a call that was stopped or failed after the provider reported tokens
// records them, marked partial; one that reported none records nothing.
func TestStoppedCallRecordsItsTokens(t *testing.T) {
	spent := &provider.Usage{InputTokens: 500, OutputTokens: 20, CacheReadTokens: 100}

	f := setup(t, permission.ModeAsk)
	p := &cutOff{usage: spent, started: make(chan struct{})}
	f.sess.(*session).SetModel(p, "m", 0)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-p.started; cancel() }()
	reason, err := f.sess.Prompt(ctx, harness.Prompt{Text: "hi"})
	if err != nil || reason != harness.StopCancelled {
		t.Fatalf("Prompt = %s, %v", reason, err)
	}
	got := usageEvents(f)
	if len(got) != 1 || !got[0].Partial || got[0].InputTokens != 500 || got[0].OutputTokens != 20 || got[0].CacheReadTokens != 100 ||
		got[0].ProviderID != "cut" || got[0].Model != "m" {
		t.Errorf("stopped call: usage = %+v", got)
	}

	f = setup(t, permission.ModeAsk)
	p = &cutOff{started: make(chan struct{})}
	f.sess.(*session).SetModel(p, "m", 0)
	ctx, cancel = context.WithCancel(context.Background())
	go func() { <-p.started; cancel() }()
	if _, err := f.sess.Prompt(ctx, harness.Prompt{Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if got := usageEvents(f); len(got) != 0 {
		t.Errorf("a stopped call that reported nothing recorded %+v", got)
	}

	f = setup(t, permission.ModeAsk)
	p = &cutOff{usage: spent, fail: errors.New("the stream broke")}
	f.sess.(*session).SetModel(p, "m", 0)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "hi"}); err == nil {
		t.Fatal("the failing call did not fail")
	}
	got = usageEvents(f)
	if len(got) != 1 || !got[0].Partial || got[0].InputTokens != 500 {
		t.Errorf("failed call: usage = %+v", got)
	}

	// A call that finished is not partial, and records once.
	f = setup(t, permission.ModeAsk)
	f.sess.(*session).SetModel(&flaky{}, "m", 0)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if got := usageEvents(f); len(got) != 1 || got[0].Partial {
		t.Errorf("finished call: usage = %+v", got)
	}
}
