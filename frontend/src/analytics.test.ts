import { test } from "node:test";
import assert from "node:assert/strict";
import { change, compact, daily, days, groupBy, niceTicks, OTHER, topKeys, totals, weekStart, weeks, byType, dailyByType, perCall, withoutCacheReads, type Row } from "./analytics.ts";

const row = (p: Partial<Row>): Row => ({
  day: "2026-10-30",
  sessionId: "s1",
  providerId: "anthropic",
  model: "big",
  inputTokens: 0,
  outputTokens: 0,
  cacheReadTokens: 0,
  cacheWriteTokens: 0,
  calls: 1,
  cost: 0,
  priced: false,
  ...p,
});

const rows: Row[] = [
  row({ day: "2026-10-30", sessionId: "s1", model: "big", inputTokens: 100, outputTokens: 50, cacheReadTokens: 300, cost: 1, priced: true }),
  row({ day: "2026-10-31", sessionId: "s2", model: "small", inputTokens: 10, outputTokens: 5 }),
  row({ day: "2026-10-31", sessionId: "s1", model: "big", inputTokens: 20, outputTokens: 10, cost: 0.5, priced: true }),
];

test("totals add tokens, priced cost, sessions and the cache share", () => {
  const t = totals(rows);
  assert.equal(t.input, 130);
  assert.equal(t.output, 65);
  assert.equal(t.cost, 1.5);
  assert.equal(t.unpricedTokens, 15);
  assert.equal(t.sessions, 2);
  assert.equal(t.days, 2);
  assert.equal(t.cacheShare, 300 / 430);
  assert.equal(totals([]).cacheShare, 0);
});

test("groups, the largest first by the metric", () => {
  const byTokens = groupBy(rows, (r) => r.model, "tokens");
  assert.deepEqual(byTokens.map((g) => [g.key, g.input + g.output + g.cacheRead, g.sessions]), [
    ["big", 480, 1],
    ["small", 15, 1],
  ]);
  const byCost = groupBy(rows, (r) => r.sessionId, "cost");
  assert.equal(byCost[0].key, "s1");
  assert.equal(byCost[0].cost, 1.5);
});

test("top keys keep room for Other", () => {
  const g = (k: string) => ({ key: k, input: 0, output: 0, cacheRead: 0, cacheWrite: 0, calls: 0, cost: 0, unpricedTokens: 0, sessions: 0 });
  assert.deepEqual(topKeys(["a", "b"].map(g), 5), ["a", "b"]);
  assert.deepEqual(topKeys(["a", "b", "c", "d", "e", "f"].map(g), 5), ["a", "b", "c", "d"]);
});

test("every day of the range, with Other only when used", () => {
  assert.deepEqual(days("2026-10-30", "2026-11-02"), ["2026-10-30", "2026-10-31", "2026-11-01", "2026-11-02"]);
  const d = daily(rows, days("2026-10-29", "2026-10-31"), (r) => r.model, ["big"], "tokens");
  assert.deepEqual(
    d.map((x) => x.values),
    [
      [0, 0],
      [450, 0],
      [30, 15],
    ],
  );
  assert.equal(d[2].total, 45);
  const all = daily(rows, days("2026-10-30", "2026-10-31"), (r) => r.model, ["big", "small"], "cost");
  assert.deepEqual(
    all.map((x) => x.values),
    [
      [1, 0],
      [0.5, 0],
    ],
  );
  assert.ok(!all.some((x) => x.values.length > 2), `no ${OTHER} column when every key has a series`);
});

test("change, ticks and compact numbers", () => {
  assert.equal(change(150, 100), 0.5);
  assert.equal(change(5, 0), null);
  assert.deepEqual(niceTicks(950), [0, 250, 500, 750, 1000]);
  assert.deepEqual(niceTicks(0), [0]);
  assert.deepEqual(niceTicks(0.37), [0, 0.1, 0.2, 0.3, 0.4]);
  assert.equal(compact(950), "950");
  assert.equal(compact(12_940), "12.9K");
  assert.equal(compact(4_200_000), "4.2M");
  assert.equal(compact(250_000), "250K");
  assert.equal(compact(-12_940), "-12.9K");
  assert.equal(compact(1_000), "1K");
  assert.equal(compact(0), "0");
});

test("weeks start on Monday", () => {
  assert.equal(weekStart("2026-11-01"), "2026-10-26"); // a Sunday
  assert.equal(weekStart("2026-10-26"), "2026-10-26");
  assert.deepEqual(weeks("2026-10-25", "2026-11-03"), ["2026-10-19", "2026-10-26", "2026-11-02"]);
});

test("tokens and cost by kind of token", () => {
  const priced = row({ inputTokens: 100, outputTokens: 10, cacheReadTokens: 1000, cacheWriteTokens: 50, cost: 2, priced: true, costInput: 0.5, costOutput: 1, costCacheRead: 0.3, costCacheWrite: 0.2 });
  const free = row({ day: "2026-10-31", inputTokens: 5, outputTokens: 1 });
  const g = byType([priced, free]);
  assert.deepEqual(
    g.map((x) => [x.key, x.tokens, x.cost]),
    [
      ["input", 105, 0.5],
      ["output", 11, 1],
      ["cacheWrite", 50, 0.2],
      ["cacheRead", 1000, 0.3],
    ],
  );
  const d = dailyByType([priced, free], ["2026-10-30", "2026-10-31"], "tokens");
  assert.deepEqual(d[0].values, [100, 10, 50, 1000]);
  assert.deepEqual(d[1].values, [5, 1, 0, 0]);
  assert.equal(dailyByType([priced, free], ["2026-10-31"], "cost")[0].total, 0);
  assert.equal(perCall(totals([priced, free])), 1166 / 2);
  assert.equal(perCall(totals([])), 0);
});

test("without cache reads", () => {
  const [r] = withoutCacheReads([row({ inputTokens: 10, cacheReadTokens: 90, cost: 1, priced: true, costCacheRead: 0.25 })]);
  assert.equal(r.cacheReadTokens, 0);
  assert.equal(r.cost, 0.75);
  assert.equal(totals([r]).cacheShare, 0);
});
