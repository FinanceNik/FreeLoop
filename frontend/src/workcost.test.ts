import { test } from "node:test";
import assert from "node:assert/strict";
import { weeks as mondays } from "./analytics.ts";
import { commitChange, limitChange, limitSeries, quantile, weekStats, type CommitCost, type LimitUse } from "./workcost.ts";

const commit = (week: string, cost: number, extra: Partial<CommitCost> = {}): CommitCost => ({
  week,
  cost,
  unpriced: false,
  input: 100,
  output: 10,
  cacheRead: 1000,
  cacheWrite: 0,
  added: 5,
  deleted: 1,
  ...extra,
});

test("quantiles read between neighbours", () => {
  assert.equal(quantile([], 0.5), 0);
  assert.equal(quantile([4], 0.5), 4);
  assert.equal(quantile([1, 2, 3, 4], 0.5), 2.5);
  assert.equal(quantile([1, 2, 3, 4, 5], 0.25), 2);
  assert.equal(quantile([0, 10], 0.75), 7.5);
});

test("a week's commits: the median, the middle half, all-in and what no commit followed", () => {
  const commits = [commit("w1", 1), commit("w1", 2), commit("w1", 30, { added: -1, deleted: -1 }), commit("w2", 5)];
  const spend = [
    { week: "w1", committed: true, cost: 33, tokens: 0 },
    { week: "w1", committed: false, cost: 11, tokens: 0 },
  ];
  const [w1, w2, w3] = weekStats(["w1", "w2", "w3"], commits, spend, true);
  assert.equal(w1.commits, 3);
  assert.equal(w1.median, 2);
  assert.equal(w1.p25, 1.5);
  assert.equal(w1.p75, 16);
  // A huge commit moves the all-in figure, not the median.
  assert.ok(Math.abs(w1.allIn - 44 / 3) < 1e-9);
  assert.equal(w1.uncommitted, 0.25);
  assert.equal(w1.medianTokens, 1110);
  // Lines are counted where they were: an untrusted project's commit has none.
  assert.equal(w1.medianLines, 6);
  assert.equal(weekStats(["w1"], commits, spend, false)[0].medianTokens, 110);
  assert.equal(w2.commits, 1);
  assert.equal(w2.uncommitted, 0);
  assert.equal(w3.commits, 0);
  assert.equal(w3.median, 0);
});

test("a change is said only with enough on both sides", () => {
  const weeks = mondays("2026-09-07", "2026-11-09");
  assert.equal(weeks.length, 10);
  assert.equal(weeks[9], "2026-11-09");
  const earlier = weeks.slice(0, 8).flatMap((w) => [commit(w, 1), commit(w, 1)]);
  const recent = weeks.slice(8).flatMap((w) => Array.from({ length: 5 }, () => commit(w, 1.5)));
  const c = commitChange(weeks, [...earlier, ...recent], (x) => x.cost);
  assert.ok(c && Math.abs(c.ratio - 0.5) < 1e-9 && c.recent === 10 && c.earlier === 16);
  assert.equal(commitChange(weeks, [...earlier, ...recent.slice(1)], (x) => x.cost), null);
  assert.equal(commitChange(weeks, recent, (x) => x.cost), null);
});

test("what a percent of a limit bought, and its change", () => {
  const weeks = mondays("2026-09-07", "2026-11-09");
  const use = (week: string, spend: number, percent: number, runs = 1): LimitUse => ({ week, source: "Agent", window: "five_hour", label: "5-hour limit", spend, percent, runs });
  const uses = [...weeks.slice(0, 8).map((w) => use(w, 40, 20)), use(weeks[8], 10, 20, 2), use(weeks[9], 30, 60, 2)];
  const [series] = limitSeries(uses);
  assert.equal(series.label, "5-hour limit");
  assert.equal(series.weeks.length, 10);
  assert.equal(series.weeks[0].perPercent, 2);
  // Ratio of sums: 40 for 80 percent is 0.5 a percent, against 2 before.
  const c = limitChange(weeks, uses);
  assert.ok(c && Math.abs(c.ratio + 0.75) < 1e-9 && c.recent === 4 && c.earlier === 8);
  assert.equal(limitChange(weeks, uses.slice(0, 9)), null);
});
