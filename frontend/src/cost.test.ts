import { test } from "node:test";
import assert from "node:assert/strict";
import { estimate, findPrice, formatCost, globMatch, type ModelCall, type PriceEntry } from "./cost.ts";

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
});
