import assert from "node:assert/strict";
import { test } from "node:test";
import { recentChart, recentSeries, recentTotals, type RecentBucket } from "./recentUsage.ts";

const b = (start: number, over: Partial<RecentBucket> = {}): RecentBucket => ({ start, input: 0, output: 0, cacheRead: 0, cacheWrite: 0, calls: 0, cost: 0, unpriced: 0, ...over });

test("recent usage keeps empty steps, so the time axis stays even (GUI-023)", () => {
  const chart = recentChart([b(1, { input: 5, output: 2 }), b(2)], "tokens", true);
  assert.deepEqual(chart.map((d) => d.total), [7, 0]);
  assert.equal(chart[0].day, "1");
});

test("leaving out cache reads drops their series and their values together", () => {
  const data = [b(1, { input: 1, output: 2, cacheWrite: 3, cacheRead: 100 })];
  assert.equal(recentSeries("tokens", false).length, recentChart(data, "tokens", false)[0].values.length);
  assert.equal(recentSeries("tokens", true).length, recentChart(data, "tokens", true)[0].values.length);
  assert.equal(recentChart(data, "tokens", false)[0].total, 6);
  assert.equal(recentChart(data, "tokens", true)[0].total, 106);
});

test("cost is one series, and totals say what has no price", () => {
  assert.equal(recentSeries("cost", true).length, 1);
  assert.deepEqual(recentChart([b(1, { cost: 0.5 })], "cost", true)[0].values, [0.5]);
  const t = recentTotals([b(1, { input: 5, cacheRead: 10, calls: 1, cost: 1 }), b(2, { input: 1, calls: 2, unpriced: 2 })], false);
  assert.deepEqual(t, { tokens: 6, output: 0, calls: 3, cost: 1, unpriced: 2 });
});
