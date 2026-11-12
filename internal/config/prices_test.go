package config

import (
	"math"
	"testing"
)

func TestFindPrice(t *testing.T) {
	prices := []Price{
		{ProviderID: "work", Model: "claude-*", Input: 1},
		{Model: "claude-*", Input: 2},
		{Model: "gpt-x", Input: 3},
	}
	for _, c := range []struct {
		provider, model string
		input           float64
		ok              bool
	}{
		{"work", "claude-big", 1, true},
		{"home", "claude-big", 2, true},
		{"home", "gpt-x", 3, true},
		{"home", "gpt-x-mini", 0, false},
		{"home", "", 0, false},
		// An agent's names for a model (PRV-008).
		{"", "openai/gpt-x", 3, true},
		{"", "gpt-x/high", 3, true},
		{"", "local/stand-in", 0, false},
		// A model on the user's own provider is not taken for a cloud model.
		{"ollama", "someone/gpt-x", 0, false},
	} {
		p, ok := FindPrice(prices, c.provider, c.model)
		if ok != c.ok || p.Input != c.input {
			t.Errorf("FindPrice(%s, %s) = %+v, %v", c.provider, c.model, p, ok)
		}
	}
	p := Price{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75}
	if got := p.Cost(1e6, 1e6, 1e6, 1e6, 0); math.Abs(got-22.05) > 1e-9 {
		t.Errorf("cost = %v", got)
	}
}

func TestValidatePrices(t *testing.T) {
	c := Default()
	c.Prices = []Price{{Model: "ok", Input: 1}, {Model: "", Input: 1}, {Model: "[", Input: 1}, {Model: "neg", Output: -2}, {Model: "nan", Input: math.NaN()}}
	issues := c.Validate()
	if len(c.Prices) != 1 || c.Prices[0].Model != "ok" || len(issues) != 4 || issues[0].Field != "prices[1]" {
		t.Errorf("prices = %+v, issues = %+v", c.Prices, issues)
	}
}

// OQ-023: every model with a list price is found under its name and its dated
// names, a long prompt pays the long rate, and an hour's cache write its own.
func TestBuiltinPrices(t *testing.T) {
	prices := BuiltinPrices()
	for _, c := range []struct {
		model         string
		input, output float64
	}{
		{"claude-opus-5-5", 4, 20},
		{"claude-opus-4-5-20251101", 5, 25},
		{"claude-opus-4-20250514", 15, 75},
		{"claude-sonnet-4-5-20250929", 3, 15},
		{"claude-haiku-4-5-20251001", 1, 5},
		{"claude-3-5-haiku-20241022", 0.8, 4},
		{"claude-haiku-5-5", 0.1, 0.5},
		{"claude-haiku-5-5 (long prompt)", 0.5, 2.5},
		{"gpt-5", 1.25, 10},
		{"gpt-5-mini", 0.25, 2},
		{"gpt-5-2025-08-07", 1.25, 10},
		{"gpt-5.5-2026-04-23 (long prompt)", 10, 45},
		{"gemini-2.5-pro (long prompt)", 2.5, 15},
	} {
		p, ok := FindPrice(prices, "", c.model)
		if !ok || p.Input != c.input || p.Output != c.output {
			t.Errorf("%s: %+v, %v; want %v, %v", c.model, p, ok, c.input, c.output)
		}
	}
	for _, unknown := range []string{"", "llama3.2", "gpt-5-unknown", "anthropic.claude-opus-5-5"} {
		if p, ok := FindPrice(prices, "", unknown); ok {
			t.Errorf("%q priced as %+v", unknown, p)
		}
	}
	for _, c := range []struct {
		model  string
		prompt int64
		want   string
	}{
		{"claude-haiku-5-5", 100_000, "claude-haiku-5-5"},
		{"claude-haiku-5-5", 100_001, "claude-haiku-5-5 (long prompt)"},
		{"claude-opus-5-5", 900_000, "claude-opus-5-5"},
		{"gpt-5.4", 272_001, "gpt-5.4 (long prompt)"},
		{"unknown", 900_000, "unknown"},
	} {
		if got := PricedModel(c.model, c.prompt); got != c.want {
			t.Errorf("PricedModel(%s, %d) = %s", c.model, c.prompt, got)
		}
	}
	opus, _ := FindPrice(prices, "", "claude-opus-5-5")
	if got := opus.Cost(0, 0, 0, 3e6, 1e6); math.Abs(got-18) > 1e-9 {
		t.Errorf("2M 5-minute and 1M 1-hour writes cost %v, want 18", got)
	}
	plain := Price{CacheWrite: 2}
	if got := plain.Cost(0, 0, 0, 1e6, 1e6); math.Abs(got-2) > 1e-9 {
		t.Errorf("a row without an hour's rate charged %v, want 2", got)
	}
}

// Through OpenRouter this model's ID has its vendor in front, and a cache read costs
// a tenth of an input, near enough.
func TestQwenFlashIsPriced(t *testing.T) {
	p, ok := FindPrice(BuiltinPrices(), "openrouter", "qwen/qwen3.8-flash")
	if !ok || p.Input != 0.15 || p.Output != 0.47 || p.CacheRead != 0.016 {
		t.Errorf("%v %+v", ok, p)
	}
	if _, ok := FindPrice(BuiltinPrices(), "openrouter", "qwen/qwen3.8-max-prime"); ok {
		t.Error("the row reaches another Qwen model, at the wrong price")
	}
}

// PRV-008: a price holds from a day; a day is priced by the newest row that has
// begun by then, so a change of price leaves earlier days as they were.
func TestPriceHoldsFromADay(t *testing.T) {
	prices := []Price{
		{Model: "m", From: "2026-12-01", Input: 4},
		{Model: "m", Input: 2},
	}
	for _, c := range []struct {
		day   string
		input float64
	}{{"2026-11-30", 2}, {"2026-12-01", 4}, {"2027-01-05", 4}} {
		if p, ok := FindPriceOn(prices, "", "m", c.day); !ok || p.Input != c.input {
			t.Errorf("on %s: %+v, %v", c.day, p, ok)
		}
	}
	// Only a row that has begun applies: with none, the model has no price.
	if _, ok := FindPriceOn(prices[:1], "", "m", "2026-11-30"); ok {
		t.Error("a price that has not begun applied")
	}
	// The built-in table's change of a price is dated too.
	before, _ := FindPriceOn(BuiltinPrices(), "", "gemini-4-argon", "2026-11-30")
	after, _ := FindPriceOn(BuiltinPrices(), "", "gemini-4-argon", "2026-12-01")
	if before.Input != 2 || after.Input != 4 {
		t.Errorf("gemini-4-argon: %v before, %v after", before.Input, after.Input)
	}
	c := Default()
	c.Prices = []Price{{Model: "ok", From: "soon", Input: 1}, {Model: "ok", From: "2026-12-01", Input: 1}}
	if issues := c.Validate(); len(c.Prices) != 1 || len(issues) != 1 {
		t.Errorf("prices = %+v, issues = %+v", c.Prices, issues)
	}
}
