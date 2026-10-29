package config

import (
	"fmt"
	"math"
	"path"
)

// Price is what a model's tokens cost, in US dollars per million tokens
// (PRV-008). Model matches a model ID exactly or as a glob ("claude-*");
// ProviderID, when set, limits the entry to one provider. The first
// matching entry applies. The app ships no prices: they change, and an
// estimate from a stale table would mislead (OQ-023).
type Price struct {
	ProviderID string  `yaml:"providerId,omitempty" json:"providerId"`
	Model      string  `yaml:"model" json:"model"`
	Input      float64 `yaml:"input" json:"input"`
	Output     float64 `yaml:"output" json:"output"`
	CacheRead  float64 `yaml:"cacheRead,omitempty" json:"cacheRead"`
	CacheWrite float64 `yaml:"cacheWrite,omitempty" json:"cacheWrite"`
}

// FindPrice returns the first entry for a provider's model.
func FindPrice(prices []Price, providerID, model string) (Price, bool) {
	for _, p := range prices {
		if p.ProviderID != "" && p.ProviderID != providerID {
			continue
		}
		if ok, _ := path.Match(p.Model, model); ok || p.Model == model {
			return p, true
		}
	}
	return Price{}, false
}

// Cost is the price of tokens in US dollars.
func (p Price) Cost(input, output, cacheRead, cacheWrite int64) float64 {
	return (float64(input)*p.Input + float64(output)*p.Output + float64(cacheRead)*p.CacheRead + float64(cacheWrite)*p.CacheWrite) / 1e6
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
		for _, v := range []float64{p.Input, p.Output, p.CacheRead, p.CacheWrite} {
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
