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
	} {
		p, ok := FindPrice(prices, c.provider, c.model)
		if ok != c.ok || p.Input != c.input {
			t.Errorf("FindPrice(%s, %s) = %+v, %v", c.provider, c.model, p, ok)
		}
	}
	p := Price{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75}
	if got := p.Cost(1e6, 1e6, 1e6, 1e6); math.Abs(got-22.05) > 1e-9 {
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
