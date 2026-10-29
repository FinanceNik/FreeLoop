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
