package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
)

// pacedProvider streams numbered words at a steady pace and remembers when each left.
type pacedProvider struct {
	words int
	every time.Duration
	mu    sync.Mutex
	sent  map[string]time.Time
}

func (p *pacedProvider) ID() string          { return "paced" }
func (p *pacedProvider) Type() provider.Type { return "scripted" }
func (p *pacedProvider) Capabilities(string) provider.ModelCaps {
	return provider.ModelCaps{Tools: true}
}
func (p *pacedProvider) ListModels(context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (p *pacedProvider) Stream(ctx context.Context, _ provider.ChatRequest) (<-chan provider.Event, error) {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		var all strings.Builder
		for i := 0; i < p.words; i++ {
			time.Sleep(p.every)
			w := fmt.Sprintf("w%03d ", i)
			all.WriteString(w)
			p.mu.Lock()
			p.sent[w] = time.Now()
			p.mu.Unlock()
			select {
			case out <- provider.Event{Type: provider.EventTextDelta, Text: w}:
			case <-ctx.Done():
				return
			}
		}
		out <- provider.Event{Type: provider.EventDone, StopReason: provider.StopEndTurn,
			Message: &provider.Message{Role: provider.RoleAssistant, Parts: []provider.Part{{Type: provider.PartText, Text: all.String()}}}}
	}()
	return out, nil
}

// NFR-PERF-005: streamed words reach the event bus, which the window listens to, well
// within 50 ms of arriving from the provider. The window's own drawing is on top and
// is measured by hand in the packaged app.
func TestStreamedWordsArriveQuickly(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	p := &pacedProvider{words: 50, every: 20 * time.Millisecond, sent: map[string]time.Time{}}
	e.mu.Lock()
	e.providers.built["paced"] = &configuredProvider{cfg: config.Provider{ID: "paced", Name: "paced", Type: "scripted"}, p: p}
	e.mu.Unlock()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "paced", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	ch, cancel := e.Subscribe()
	defer cancel()
	if _, err := e.Prompt(ctx, ses.ID, "go"); err != nil {
		t.Fatal(err)
	}
	var lags []time.Duration
	deadline := time.After(10 * time.Second)
	for done := false; !done; {
		select {
		case ev := <-ch:
			switch ev.Type {
			case event.AgentMessageChunk:
				got := time.Now()
				var d event.MessageChunk
				json.Unmarshal(ev.Data, &d)
				p.mu.Lock()
				for _, w := range strings.SplitAfter(d.Content.Text, " ") {
					if at, ok := p.sent[w]; ok {
						lags = append(lags, got.Sub(at))
					}
				}
				p.mu.Unlock()
			case event.TurnEnded:
				done = true
			}
		case <-deadline:
			t.Fatal("the turn did not end")
		}
	}
	if len(lags) != 50 {
		t.Fatalf("%d of 50 words arrived", len(lags))
	}
	slices.Sort(lags)
	p95, worst := lags[len(lags)*95/100], lags[len(lags)-1]
	t.Logf("words reached the bus after %v (median), %v (95th), %v (worst)", lags[len(lags)/2], p95, worst)
	if p95 > 50*time.Millisecond {
		t.Errorf("95%% of words took up to %v; NFR-PERF-005 allows 50 ms", p95)
	}
}
