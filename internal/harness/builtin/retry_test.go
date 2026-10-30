package builtin

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// flaky fails its first fails calls with err, before or after streaming
// some text, then answers.
type flaky struct {
	mu     sync.Mutex
	calls  int
	fails  int
	err    error
	midway bool
}

func (f *flaky) ID() string                                               { return "flaky" }
func (f *flaky) Type() provider.Type                                      { return provider.TypeScripted }
func (f *flaky) ListModels(context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (f *flaky) Capabilities(string) provider.ModelCaps                   { return provider.ModelCaps{Tools: true} }

func (f *flaky) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	if n <= f.fails && !f.midway {
		return nil, f.err
	}
	ch := make(chan provider.Event, 2)
	if n <= f.fails {
		ch <- provider.Event{Type: provider.EventTextDelta, Text: "partial "}
		ch <- provider.Event{Type: provider.EventError, Err: f.err}
	} else {
		ch <- provider.Event{Type: provider.EventDone, Message: &provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{{Type: provider.PartText, Text: "ok"}}}, StopReason: provider.StopEndTurn}
	}
	close(ch)
	return ch, nil
}

func notices(f *fixture) []string {
	var out []string
	for _, ev := range f.log.all() {
		if ev.Type == event.Notice {
			out = append(out, string(ev.Data))
		}
	}
	return out
}

// PRV-010: a rate-limited call is retried and the retries are shown as
// notices; a provider that keeps failing hands the call to the profile's
// fallback; an error after text streamed is not retried.
func TestRetriesAndFallback(t *testing.T) {
	defer func(b time.Duration) { retryBase = b }(retryBase)
	retryBase = time.Millisecond
	limited := &provider.HTTPError{Status: 429, Message: "rate limited (429): slow down"}

	f := setup(t, permission.ModeAsk)
	p := &flaky{fails: 2, err: limited}
	f.sess.(*session).SetModel(p, "m", 0)
	if reason, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "hi"}); err != nil || reason != harness.StopEndTurn {
		t.Fatalf("Prompt = %s, %v", reason, err)
	}
	if n := notices(f); p.calls != 3 || len(n) != 2 || !strings.Contains(n[0], "retrying in") || !strings.Contains(n[1], "attempt 3 of 4") {
		t.Errorf("calls = %d, notices = %v", p.calls, n)
	}

	f = setup(t, permission.ModeAsk)
	down := &flaky{fails: 100, err: &provider.HTTPError{Status: 503, Message: "the endpoint returned 503: overloaded"}}
	s := f.sess.(*session)
	s.SetModel(down, "m", 0)
	fb := scripted.New("fb", scripted.Step{Text: "from the fallback"})
	s.p.Fallback = &harness.Fallback{Provider: fb, Model: "small"}
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if n := notices(f); down.calls != maxAttempts || len(n) != maxAttempts || !strings.Contains(n[len(n)-1], "fallback model small") || len(fb.Requests()) != 1 {
		t.Errorf("calls = %d, fallback calls = %d, notices = %v", down.calls, len(fb.Requests()), n)
	}

	f = setup(t, permission.ModeAsk)
	mid := &flaky{fails: 1, err: limited, midway: true}
	f.sess.(*session).SetModel(mid, "m", 0)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "hi"}); err == nil || mid.calls != 1 {
		t.Errorf("an error after streaming: err = %v, calls = %d", err, mid.calls)
	}

	f = setup(t, permission.ModeAsk)
	bad := &flaky{fails: 1, err: errors.New("the endpoint rejected the API key (401)")}
	f.sess.(*session).SetModel(bad, "m", 0)
	if _, err := f.sess.Prompt(context.Background(), harness.Prompt{Text: "hi"}); err == nil || bad.calls != 1 {
		t.Errorf("a non-retryable error: err = %v, calls = %d", err, bad.calls)
	}
}
