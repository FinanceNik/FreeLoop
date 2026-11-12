package config

import (
	"fmt"
	"math"
	"path"
	"strings"
	"time"
)

// Price is what a model's tokens cost, in US dollars per million tokens
// (PRV-008). Model matches a model ID exactly or as a glob ("claude-*");
// ProviderID, when set, limits the entry to one provider. The first
// matching entry applies: the user's in config.yaml, then BuiltinPrices.
type Price struct {
	ProviderID string  `yaml:"providerId,omitempty" json:"providerId"`
	Model      string  `yaml:"model" json:"model"`
	Input      float64 `yaml:"input" json:"input"`
	Output     float64 `yaml:"output" json:"output"`
	CacheRead  float64 `yaml:"cacheRead,omitempty" json:"cacheRead"`
	CacheWrite float64 `yaml:"cacheWrite,omitempty" json:"cacheWrite"`
	// CacheWrite1h is a write that lasts an hour; 0 charges it as CacheWrite.
	CacheWrite1h float64 `yaml:"cacheWrite1h,omitempty" json:"cacheWrite1h"`
	// From is the first day (YYYY-MM-DD, local time) the price holds; empty means
	// always. A model may have several rows, the newest first: a day is priced by the
	// first row whose From is not after it, so history keeps the prices it was
	// spent at (PRV-008).
	From string `yaml:"from,omitempty" json:"from"`
	// LongPromptOver is the prompt size above which a call pays the prices of the
	// row named Model+LongPromptSuffix; only the built-in table has one.
	LongPromptOver int64 `yaml:"-" json:"longPromptOver"`
}

// LongPromptSuffix ends the name a call above LongPromptOver counts under.
const LongPromptSuffix = " (long prompt)"

// PricedModel is the model a call's tokens count under. Usage is kept in sums per
// model and a price applies to each call by its prompt's size, so the calls that
// pay more are kept apart.
func PricedModel(model string, prompt int64) string {
	if p, ok := FindPrice(builtinPrices, "", model); ok && p.LongPromptOver > 0 && prompt > p.LongPromptOver {
		return model + LongPromptSuffix
	}
	return model
}

// FindPrice returns the first entry for a provider's model, as of today.
func FindPrice(prices []Price, providerID, model string) (Price, bool) {
	return FindPriceOn(prices, providerID, model, time.Now().Format(time.DateOnly))
}

// FindPriceOn returns the first entry for a provider's model that holds on day
// (YYYY-MM-DD): one with a From after it does not apply yet.
func FindPriceOn(prices []Price, providerID, model, day string) (Price, bool) {
	if p, ok := findPrice(prices, providerID, model, day); ok {
		return p, true
	}
	// An agent may name a model with its provider, openai/gpt-5.5, or with its
	// effort, gpt-5.5/high (PRV-008): without a price for the whole name, try either
	// side of the first slash.
	// Only for an agent's usage, which names no provider: a model on one of the
	// user's own providers (a local one, say) is priced by its own name or not at all.
	if before, after, ok := strings.Cut(model, "/"); ok && providerID == "" {
		if p, ok := findPrice(prices, providerID, after, day); ok {
			return p, true
		}
		return findPrice(prices, providerID, before, day)
	}
	return Price{}, false
}

func findPrice(prices []Price, providerID, model, day string) (Price, bool) {
	for _, p := range prices {
		if p.From > day {
			continue
		}
		if p.ProviderID != "" && p.ProviderID != providerID {
			continue
		}
		if ok, _ := path.Match(p.Model, model); ok || p.Model == model {
			return p, true
		}
	}
	return Price{}, false
}

// Cost is the price of tokens in US dollars. cacheWrite1h is the part of
// cacheWrite that lasts an hour.
func (p Price) Cost(input, output, cacheRead, cacheWrite, cacheWrite1h int64) float64 {
	rate1h := p.CacheWrite1h
	if rate1h == 0 {
		rate1h = p.CacheWrite
	}
	return (float64(input)*p.Input + float64(output)*p.Output + float64(cacheRead)*p.CacheRead +
		float64(cacheWrite-cacheWrite1h)*p.CacheWrite + float64(cacheWrite1h)*rate1h) / 1e6
}

// validatePrices drops entries that cannot apply and reports them.
func validatePrices(prices []Price) ([]Price, []Issue) {
	var issues []Issue
	out := prices[:0:0]
	for i, p := range prices {
		field := fmt.Sprintf("prices[%d]", i)
		bad := ""
		if p.Model == "" {
			bad = "a price needs a model"
		} else if _, err := path.Match(p.Model, ""); err != nil {
			bad = fmt.Sprintf("%q is not a valid pattern", p.Model)
		}
		if p.From != "" {
			if _, err := time.Parse(time.DateOnly, p.From); err != nil {
				bad = fmt.Sprintf("%q is not a date (YYYY-MM-DD)", p.From)
			}
		}
		for _, v := range []float64{p.Input, p.Output, p.CacheRead, p.CacheWrite, p.CacheWrite1h} {
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				bad = "prices must be zero or more"
			}
		}
		if bad != "" {
			issues = append(issues, Issue{Field: field, Message: bad})
			continue
		}
		out = append(out, p)
	}
	return out, issues
}
