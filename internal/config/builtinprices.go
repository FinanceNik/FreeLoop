package config

import "slices"

// PricesAsOf is the day BuiltinPrices was copied from the vendors' price lists
// (OQ-023). The app does not fetch them: nothing leaves it unasked (SEC-009).
const PricesAsOf = "2026-11-03"

// listPrice is one model's standard rate, per million tokens.
type listPrice struct {
	model                               string
	input, output, read, write, write1h float64
	longOver                            int64
}

// Sources, standard tier, global endpoints:
//   - https://platform.claude.com/docs/en/about-claude/pricing
//   - https://developers.openai.com/api/docs/pricing
//   - https://aistudio.google.com/docs/pricing
//
// Not counted, since a call's usage does not say: Claude's fast mode and US-only
// inference, batch rates, Gemini's audio rates and hourly cache storage, OpenAI's
// cache writes. Claude on Bedrock or Vertex has other prices and other names.
//
// Due to change: gemini-4-argon doubles on 2026-12-01, and gemini-3.8-flash and
// gemini-3.6-flash on 2027-01-01. gemini-2.5-pro and gemini-2.5-flash go elsewhere
// on 2026-12-01.
//
// A long prompt's row comes first: the pattern of its model's dated names would
// match it too.
var listPrices = []listPrice{
	{model: "claude-fable-5-1", input: 10, output: 50, read: 0.25, write: 12.5, write1h: 20},
	{model: "claude-mythos-5-1", input: 10, output: 50, read: 0.25, write: 12.5, write1h: 20},
	{model: "claude-fable-5", input: 10, output: 50, read: 1, write: 12.5, write1h: 20},
	{model: "claude-mythos-5", input: 10, output: 50, read: 1, write: 12.5, write1h: 20},
	{model: "claude-opus-5-5", input: 4, output: 20, read: 0.2, write: 5, write1h: 8},
	{model: "claude-opus-5", input: 5, output: 25, read: 0.5, write: 6.25, write1h: 10},
	{model: "claude-opus-4-8", input: 5, output: 25, read: 0.5, write: 6.25, write1h: 10},
	{model: "claude-opus-4-7", input: 5, output: 25, read: 0.5, write: 6.25, write1h: 10},
	{model: "claude-opus-4-6", input: 5, output: 25, read: 0.5, write: 6.25, write1h: 10},
	{model: "claude-opus-4-5", input: 5, output: 25, read: 0.5, write: 6.25, write1h: 10},
	{model: "claude-opus-4-1", input: 15, output: 75, read: 1.5, write: 18.75, write1h: 30},
	{model: "claude-opus-4-0", input: 15, output: 75, read: 1.5, write: 18.75, write1h: 30},
	{model: "claude-opus-4", input: 15, output: 75, read: 1.5, write: 18.75, write1h: 30},
	{model: "claude-sonnet-5-5", input: 2, output: 10, read: 0.1, write: 2.5, write1h: 4},
	{model: "claude-sonnet-5", input: 2, output: 10, read: 0.2, write: 2.5, write1h: 4},
	{model: "claude-sonnet-4-6", input: 3, output: 15, read: 0.3, write: 3.75, write1h: 6},
	{model: "claude-sonnet-4-5", input: 3, output: 15, read: 0.3, write: 3.75, write1h: 6},
	{model: "claude-sonnet-4-0", input: 3, output: 15, read: 0.3, write: 3.75, write1h: 6},
	{model: "claude-sonnet-4", input: 3, output: 15, read: 0.3, write: 3.75, write1h: 6},
	{model: "claude-haiku-5-5" + LongPromptSuffix, input: 0.5, output: 2.5, read: 0.05, write: 0.625, write1h: 1},
	{model: "claude-haiku-5-5", input: 0.1, output: 0.5, read: 0.01, write: 0.125, write1h: 0.2, longOver: 100_000},
	{model: "claude-haiku-4-5", input: 1, output: 5, read: 0.1, write: 1.25, write1h: 2},
	{model: "claude-3-5-haiku", input: 0.8, output: 4, read: 0.08, write: 1, write1h: 1.6},

	{model: "gpt-6-astra" + LongPromptSuffix, input: 20, output: 75, read: 2},
	{model: "gpt-6-astra", input: 10, output: 50, read: 1, longOver: 272_000},
	{model: "gpt-6.1-sol" + LongPromptSuffix, input: 4, output: 15, read: 0.2},
	{model: "gpt-6.1-sol", input: 2, output: 10, read: 0.1, longOver: 272_000},
	{model: "gpt-6.1-terra" + LongPromptSuffix, input: 2, output: 7.5, read: 0.1},
	{model: "gpt-6.1-terra", input: 1, output: 5, read: 0.05, longOver: 272_000},
	{model: "gpt-6-luna" + LongPromptSuffix, input: 0.2, output: 0.75, read: 0.02},
	{model: "gpt-6-luna", input: 0.1, output: 0.5, read: 0.01, longOver: 272_000},
	{model: "gpt-6-sol" + LongPromptSuffix, input: 4, output: 15, read: 0.4},
	{model: "gpt-6-sol", input: 2, output: 10, read: 0.2, longOver: 272_000},
	{model: "gpt-5.6-sol" + LongPromptSuffix, input: 8, output: 30, read: 0.8},
	{model: "gpt-5.6-sol", input: 4, output: 20, read: 0.4, longOver: 272_000},
	{model: "gpt-5.6-terra" + LongPromptSuffix, input: 4, output: 18, read: 0.4},
	{model: "gpt-5.6-terra", input: 2, output: 12, read: 0.2, longOver: 272_000},
	{model: "gpt-5.6-luna" + LongPromptSuffix, input: 0.4, output: 1.8, read: 0.04},
	{model: "gpt-5.6-luna", input: 0.2, output: 1.2, read: 0.02, longOver: 272_000},
	{model: "gpt-5.5" + LongPromptSuffix, input: 10, output: 45, read: 1},
	{model: "gpt-5.5", input: 5, output: 30, read: 0.5, longOver: 272_000},
	{model: "gpt-5.5-pro" + LongPromptSuffix, input: 60, output: 270},
	{model: "gpt-5.5-pro", input: 30, output: 180, longOver: 272_000},
	{model: "gpt-5.4" + LongPromptSuffix, input: 5, output: 22.5, read: 0.5},
	{model: "gpt-5.4", input: 2.5, output: 15, read: 0.25, longOver: 272_000},
	{model: "gpt-5.4-pro" + LongPromptSuffix, input: 60, output: 270},
	{model: "gpt-5.4-pro", input: 30, output: 180, longOver: 272_000},
	{model: "gpt-5.4-mini", input: 0.75, output: 4.5, read: 0.075},
	{model: "gpt-5.4-nano", input: 0.2, output: 1.25, read: 0.02},
	{model: "gpt-5.3-codex", input: 1.75, output: 14, read: 0.175},
	{model: "gpt-5.2", input: 1.75, output: 14, read: 0.175},
	{model: "gpt-5.2-pro", input: 21, output: 168},
	{model: "gpt-5.1", input: 1.25, output: 10, read: 0.125},
	{model: "gpt-5", input: 1.25, output: 10, read: 0.125},
	{model: "gpt-5-mini", input: 0.25, output: 2, read: 0.025},
	{model: "gpt-5-nano", input: 0.05, output: 0.4, read: 0.005},
	{model: "gpt-5-pro", input: 15, output: 120},
	{model: "gpt-5-search-api", input: 1.25, output: 10, read: 0.125},
	{model: "gpt-4.1", input: 2, output: 8, read: 0.5},
	{model: "gpt-4.1-mini", input: 0.4, output: 1.6, read: 0.1},
	{model: "gpt-4o", input: 2.5, output: 10, read: 1.25},
	{model: "gpt-4o-mini", input: 0.15, output: 0.6, read: 0.075},
	{model: "o3-pro", input: 20, output: 80},
	{model: "o3", input: 2, output: 8, read: 0.5},
	{model: "chat-latest", input: 5, output: 30, read: 0.5},

	{model: "gemini-4-argon", input: 2, output: 10, read: 0.1},
	{model: "gemini-3.8-flash", input: 0.75, output: 3.75, read: 0.075},
	{model: "gemini-3.6-flash", input: 0.75, output: 3.75, read: 0.075},
	{model: "gemini-3.5-flash-lite", input: 0.3, output: 2.5, read: 0.03},
	{model: "gemini-3.1-flash-lite", input: 0.25, output: 1.5, read: 0.025},
	{model: "gemini-3.1-pro-preview" + LongPromptSuffix, input: 4, output: 18, read: 0.4},
	{model: "gemini-3.1-pro-preview", input: 2, output: 12, read: 0.2, longOver: 200_000},
	{model: "gemini-3.1-pro-preview-customtools" + LongPromptSuffix, input: 4, output: 18, read: 0.4},
	{model: "gemini-3.1-pro-preview-customtools", input: 2, output: 12, read: 0.2, longOver: 200_000},
	{model: "gemini-3-flash-preview", input: 0.5, output: 3, read: 0.05},
	{model: "gemini-2.5-pro" + LongPromptSuffix, input: 2.5, output: 15, read: 0.25},
	{model: "gemini-2.5-pro", input: 1.25, output: 10, read: 0.125, longOver: 200_000},
	{model: "gemini-2.5-flash", input: 0.3, output: 2.5, read: 0.03},
	{model: "gemini-2.5-flash-lite", input: 0.1, output: 0.4, read: 0.01},
}

// builtinPrices holds a row for each model's name and one for its dated names,
// such as claude-haiku-4-5-20251001.
var builtinPrices = func() []Price {
	var out []Price
	for _, l := range listPrices {
		name, long, _ := cutLongPrompt(l.model)
		p := Price{Model: name + long, Input: l.input, Output: l.output, CacheRead: l.read, CacheWrite: l.write, CacheWrite1h: l.write1h, LongPromptOver: l.longOver}
		dated := p
		dated.Model = name + "-20*" + long
		out = append(out, p, dated)
	}
	return out
}()

func cutLongPrompt(model string) (name, long string, ok bool) {
	if n := len(model) - len(LongPromptSuffix); n > 0 && model[n:] == LongPromptSuffix {
		return model[:n], LongPromptSuffix, true
	}
	return model, "", false
}

// BuiltinPrices are the vendors' list prices, which come after the user's.
func BuiltinPrices() []Price { return slices.Clone(builtinPrices) }

// EffectivePrices is what prices a call: the user's rows, then the built-in ones.
func EffectivePrices(c Config) []Price {
	return append(slices.Clone(c.Prices), builtinPrices...)
}
