package engine

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// PRV-008: a built-in session's model calls record their model; today's
// estimate uses the price table, including prices added after the calls,
// and a model without a price is reported as unpriced.
func TestUsageCosts(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	p := scripted.New("sp", scripted.Step{Text: "first"}, scripted.Step{Text: "second"})
	e.mu.Lock()
	e.providers.built["sp"] = &configuredProvider{cfg: config.Provider{ID: "sp", Name: "sp", Type: "scripted"}, p: p}
	e.mu.Unlock()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "sp", Model: "big-1", Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	e.Prompt(ctx, ses.ID, "one")
	waitIdle(t, ch, ses.ID)
	e.Prompt(ctx, ses.ID, "two")
	waitIdle(t, ch, ses.ID)

	evs, _ := e.Events(ctx, ses.ID, 0)
	var calls int
	for _, ev := range evs {
		if ev.Type == event.UsageUpdate {
			calls++
			if !strings.Contains(string(ev.Data), `"providerId":"sp","model":"big-1"`) {
				t.Errorf("usage = %s", ev.Data)
			}
		}
	}
	if calls != 2 {
		t.Fatalf("usage events = %d", calls)
	}

	days, err := e.Usage(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || len(days[0].Models) != 1 || days[0].Models[0].Priced || days[0].Cost != 0 || days[0].Models[0].InputTokens != 200 {
		t.Fatalf("without prices = %+v", days)
	}

	c := e.Config()
	c.Prices = []config.Price{{ProviderID: "other", Model: "*", Input: 100}, {Model: "big-*", Input: 3, Output: 15}}
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	days, _ = e.Usage(ctx, 30)
	m := days[0].Models[0]
	want := (200*3.0 + float64(m.OutputTokens)*15) / 1e6
	if !m.Priced || math.Abs(days[0].Cost-want) > 1e-12 {
		t.Errorf("with prices = %+v, want %v", days, want)
	}

	c.Prices = []config.Price{{Model: "x", Input: -1}}
	if _, err := e.SaveSettings(c); err == nil {
		t.Error("a negative price was saved")
	}
}
