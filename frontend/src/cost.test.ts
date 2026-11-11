import { test } from "node:test";
import assert from "node:assert/strict";
import { costText, estimate, findPrice, formatCost, globMatch, type ModelCall, type PriceEntry } from "./cost.ts";
import { setLanguage } from "./i18n.ts";

test("globMatch follows path.Match", () => {
  assert.ok(globMatch("claude-*", "claude-big-5"));
  assert.ok(!globMatch("claude-*", "openai/claude-big"));
  assert.ok(!globMatch("claude-*", "xclaude-big"));
  assert.ok(globMatch("gpt-?", "gpt-5"));
  assert.ok(globMatch("m[0-9]", "m7"));
  assert.ok(!globMatch("m[^0-9]", "m7"));
  assert.ok(globMatch("a.b", "a.b"));
  assert.ok(!globMatch("a.b", "axb"));
  assert.ok(!globMatch("[", "["));
  assert.ok(globMatch("qwen3:30b", "qwen3:30b"));
});

const price = (providerId: string, model: string, input: number, output = 0): PriceEntry => ({ providerId, model, input, output, cacheRead: 0.5, cacheWrite: 0 });
const call = (providerId: string, model: string, inputTokens: number, outputTokens = 0): ModelCall => ({
  providerId,
  model,
  inputTokens,
  outputTokens,
  cacheReadTokens: 0,
  cacheWriteTokens: 0,
});

test("findPrice takes the first entry for the provider", () => {
  const prices = [price("work", "big-*", 1), price("", "big-*", 2)];
  assert.equal(findPrice(prices, "work", "big-1")?.input, 1);
  assert.equal(findPrice(prices, "home", "big-1")?.input, 2);
  assert.equal(findPrice(prices, "home", "small"), null);
});

test("findPrice reads an agent's names for a model, as the engine does", () => {
  const prices = [price("", "gpt-x", 3)];
  assert.equal(findPrice(prices, "", "openai/gpt-x")?.input, 3);
  assert.equal(findPrice(prices, "", "gpt-x/high")?.input, 3);
  assert.equal(findPrice(prices, "", "local/stand-in"), null);
  assert.equal(findPrice(prices, "ollama", "someone/gpt-x"), null);
});

test("estimate prices calls and counts the unpriced", () => {
  const prices = [price("", "big-*", 3, 15)];
  const calls = [call("p", "big-1", 1_000_000, 100_000), call("p", "small", 5), call("", "", 1_000_000)];
  const e = estimate(prices, calls, { providerId: "p", model: "big-2" });
  assert.equal(e.unpriced, 1);
  assert.ok(Math.abs(e.cost - (3 + 1.5 + 3)) < 1e-9);
  assert.equal(formatCost(e.cost), "$7.50");
  assert.equal(formatCost(0.004), "<$0.01");
  assert.equal(formatCost(0), "$0.00");
});

test("an hour's cache write and a long prompt pay their own prices", () => {
  const prices: PriceEntry[] = [
    { providerId: "", model: "m (long prompt)", input: 5, output: 25, cacheRead: 0.5, cacheWrite: 6.25, cacheWrite1h: 10 },
    { providerId: "", model: "m", input: 1, output: 5, cacheRead: 0.1, cacheWrite: 1.25, cacheWrite1h: 2, longPromptOver: 100 },
    { providerId: "", model: "plain", input: 0, output: 0, cacheRead: 0, cacheWrite: 2 },
  ];
  const call = (model: string, inputTokens: number, cacheWriteTokens: number, cacheWrite1hTokens: number): ModelCall => ({
    providerId: "",
    model,
    inputTokens,
    outputTokens: 0,
    cacheReadTokens: 0,
    cacheWriteTokens,
    cacheWrite1hTokens,
  });
  const own = { providerId: "", model: "" };
  const near = (got: number, want: number) => assert.ok(Math.abs(got - want) < 1e-12, `${got} is not ${want}`);
  near(estimate(prices, [call("m", 0, 100, 40)], own).cost, (60 * 1.25 + 40 * 2) / 1e6);
  near(estimate(prices, [call("m", 101, 0, 0)], own).cost, (101 * 5) / 1e6);
  near(estimate(prices, [call("m", 100, 0, 0)], own).cost, (100 * 1) / 1e6);
  near(estimate(prices, [call("plain", 0, 10, 10)], own).cost, (10 * 2) / 1e6);
  // A turn's total adds up several calls, so it pays the price of the calls' own size.
  near(estimate(prices, [{ ...call("m", 101, 0, 0), turn: true }], own).cost, (101 * 1) / 1e6);
});

test("a session's cost says how the agent counted it", () => {
  const none = { turns: 0, unreported: 0 };
  assert.equal(costText({ cost: 0, unpriced: 0 }, 0, none), "none yet");
  assert.equal(costText({ cost: 0, unpriced: 0 }, 0, { turns: 0, unreported: 2 }), "not reported by the agent");
  assert.equal(costText({ cost: 0.5, unpriced: 0 }, 3, none), "≈ $0.50");
  assert.equal(costText({ cost: 0, unpriced: 2 }, 2, { turns: 2, unreported: 0 }), "no price set, from the agent's turn totals");
  assert.equal(costText({ cost: 0.5, unpriced: 0 }, 2, { turns: 2, unreported: 1 }), "≈ $0.50, from the agent's turn totals; 1 turn not reported");
});

// GUI-048: costs stay in US dollars, written as the language writes an amount. The
// space in German and Dutch is a no-break space, so the sign stays with its amount.
test("a cost is in dollars, in the language's style", () => {
  setLanguage("en");
  assert.equal(formatCost(146.51), "$146.51");
  assert.equal(formatCost(1250), "$1,250.00");
  setLanguage("de");
  assert.equal(formatCost(146.51), "146,51\u00a0$");
  assert.equal(formatCost(1250), "1.250,00\u00a0$");
  assert.equal(formatCost(0.004), "<0,01\u00a0$");
  setLanguage("nl");
  assert.equal(formatCost(146.51), "$\u00a0146,51");
  // A region the system prefers keeps the dollar a dollar, not US$ or USD.
  setLanguage("en", ["en-CA"]);
  assert.equal(formatCost(146.51), "$146.51");
  setLanguage("en");
});

// GUI-048: a session's cost in words follows the language, its amount too.
test("a session's cost is said in the language set", () => {
  setLanguage("de");
  const none = { turns: 0, unreported: 0 };
  assert.equal(costText({ cost: 0, unpriced: 0 }, 0, none), "noch keine");
  assert.equal(costText({ cost: 0, unpriced: 0 }, 0, { turns: 0, unreported: 2 }), "vom Agenten nicht gemeldet");
  assert.equal(costText({ cost: 0, unpriced: 2 }, 2, { turns: 2, unreported: 0 }), "kein Preis festgelegt, aus den Rundensummen des Agenten");
  assert.equal(costText({ cost: 0.5, unpriced: 1 }, 2, { turns: 2, unreported: 3 }), "≈ 0,50\u00a0$+, aus den Rundensummen des Agenten; 3 Runden nicht gemeldet");
  setLanguage("nl");
  assert.equal(costText({ cost: 0.5, unpriced: 0 }, 2, { turns: 0, unreported: 1 }), "≈ $\u00a00,50; 1 beurt niet gemeld");
  setLanguage("en");
  assert.equal(costText({ cost: 0.5, unpriced: 0 }, 2, { turns: 0, unreported: 3 }), "≈ $0.50; 3 turns not reported");
});
