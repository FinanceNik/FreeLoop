import assert from "node:assert/strict";
import { test } from "node:test";
import { recentChart, recentSeries, recentTotals, type RecentBucket, bucketsOf, filterRecent, groupedChart, NO_RECENT_FILTERS, recentKeys, formatTokens, recentOptions, shortTime, stepLabel, timeLabel } from "./recentUsage.ts";
import { setLanguage } from "./i18n.ts";

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

const row = (start: number, project: string, model: string, input: number, more = {}) => ({
  start,
  projectId: project,
  projectRoot: `/x/${project}`,
  harnessId: "claude-code",
  providerId: "",
  model,
  input,
  output: 0,
  cacheRead: 0,
  cacheWrite: 0,
  calls: 1,
  cost: input / 10,
  unpriced: 0,
  ...more,
});
const keys = recentKeys([{ id: "a", name: "Alpha" }], [{ id: "or", name: "OpenRouter" }], [{ id: "claude-code", name: "Claude Code" }]);

test("rows are named as the usage page names them", () => {
  assert.equal(keys.project(row(1, "a", "m", 1)), "Alpha");
  assert.equal(keys.project(row(1, "gone", "m", 1)), "gone");
  assert.equal(keys.project(row(1, "", "m", 1, { projectRoot: "" })), "No project");
  assert.equal(keys.agent(row(1, "a", "m", 1)), "Claude Code");
  assert.equal(keys.agent(row(1, "a", "m", 1, { harnessId: "builtin", providerId: "or" })), "OpenRouter");
  assert.equal(keys.model(row(1, "a", "", 1)), "Claude Code (model not reported)");
});

test("filters combine, and the buckets follow them with every step kept", () => {
  const rows = [row(1, "a", "m1", 10), row(1, "b", "m1", 20), row(2, "a", "m2", 30)];
  const got = filterRecent(rows, { ...NO_RECENT_FILTERS, project: "Alpha" }, keys);
  assert.equal(got.length, 2);
  assert.equal(filterRecent(rows, { project: "Alpha", agent: "", model: "m2" }, keys).length, 1);
  assert.deepEqual(bucketsOf([1, 2, 3], got).map((b) => b.input), [10, 30, 0]);
  assert.equal(filterRecent(rows, NO_RECENT_FILTERS, keys).length, 3);
});

test("a filter's options are sorted and keep what is chosen", () => {
  const rows = [row(1, "b", "m", 1), row(1, "a", "m", 1)];
  assert.deepEqual(recentOptions(rows, keys.project), ["Alpha", "b"]);
  assert.deepEqual(recentOptions(rows, keys.project, "zed"), ["Alpha", "b", "zed"]);
});

test("groups take one colour each, the largest first, and the rest fold into Other", () => {
  const rows = ["a", "b", "c", "d", "e", "f", "g"].map((m, i) => row(1, "a", m, 100 - i));
  const { series, data } = groupedChart([1, 2], rows, rows, keys.model, "tokens", false);
  assert.deepEqual(series.map((s) => s.label), ["a", "b", "c", "d", "e", "Other (2)"]);
  assert.equal(data[0].values[5], 95 + 94);
  assert.equal(data[0].total, 100 + 99 + 98 + 97 + 96 + 95 + 94);
  assert.equal(data[1].total, 0);
});

test("a filter does not repaint what it leaves", () => {
  const all = [row(1, "a", "big", 100), row(1, "a", "small", 1)];
  const { series } = groupedChart([1], all, [all[1]], keys.model, "tokens", false);
  assert.deepEqual(series.map((s) => [s.key, s.color]), [["small", "var(--series-2)"]]);
});

test("cost groups by cost, and cache reads count only when asked", () => {
  const all = [row(1, "a", "m", 10, { cacheRead: 5 })];
  assert.equal(groupedChart([1], all, all, keys.model, "cost", false).data[0].total, 1);
  assert.equal(groupedChart([1], all, all, keys.model, "tokens", false).data[0].total, 10);
  assert.equal(groupedChart([1], all, all, keys.model, "tokens", true).data[0].total, 15);
});


test("a time on the axis is short: AM and PM become a letter", () => {
  assert.equal(shortTime("11:30 AM"), "11:30a");
  assert.equal(shortTime("1:15\u202fPM"), "1:15p");
  assert.equal(shortTime("10:00 p.m."), "10:00p");
  assert.equal(shortTime("23:45"), "23:45");
});

// GUI-048: the axis and the totals follow the language.
test("the axis's times are 24-hour in German and Dutch and as before in English", () => {
  const evening = new Date(2026, 10, 8, 18, 30).getTime();
  const morning = new Date(2026, 10, 8, 9, 5).getTime();
  setLanguage("en");
  assert.equal(timeLabel(evening), "6:30p");
  assert.equal(timeLabel(String(morning)), "9:05a");
  setLanguage("de");
  assert.equal(timeLabel(evening), "18:30");
  assert.equal(timeLabel(morning), "9:05");
  setLanguage("nl");
  assert.equal(timeLabel(evening), "18:30");
  setLanguage("en");
});

test("the series, a step and what has no name are in the language in force", () => {
  const evening = new Date(2026, 10, 8, 18, 30).getTime();
  assert.equal(stepLabel(evening, 15), "6:30p to 6:45p");
  assert.deepEqual(recentSeries("tokens", true).map((s) => s.label), ["Input", "Output", "Cache write", "Cache read"]);
  setLanguage("de");
  assert.equal(stepLabel(evening, 15), "18:30 bis 18:45");
  assert.deepEqual(recentSeries("tokens", true).map((s) => s.label), ["Eingabe", "Ausgabe", "Cache schreiben", "Cache lesen"]);
  assert.deepEqual(recentSeries("cost", true).map((s) => s.label), ["Geschätzte Kosten"]);
  // The keys were made in English; the names follow the language when asked for.
  assert.equal(keys.project(row(1, "", "m", 1, { projectRoot: "" })), "Kein Projekt");
  assert.equal(keys.model(row(1, "a", "", 1)), "Claude Code (Modell nicht gemeldet)");
  const rows = ["a", "b", "c", "d", "e", "f", "g"].map((m, i) => row(1, "a", m, 100 - i));
  assert.equal(groupedChart([1], rows, rows, keys.model, "tokens", false).series.at(-1)?.label, "Sonstige (2)");
  setLanguage("nl");
  assert.equal(stepLabel(evening, 15), "18:30 tot 18:45");
  assert.equal(keys.project(row(1, "", "m", 1, { projectRoot: "" })), "Geen project");
  setLanguage("en");
});

test("the token formatter writes the language's short form", () => {
  setLanguage("en");
  assert.equal(formatTokens(7_900_000), "7.9M");
  assert.equal(formatTokens(12_940), "12.9K");
  assert.equal(formatTokens(950), "950");
  // The space before Mio. and mln. is a no-break space, so the unit stays with its number.
  setLanguage("de");
  assert.equal(formatTokens(7_900_000), "7,9 Mio.");
  assert.equal(formatTokens(1_100_000_000), "1,1 Mrd.");
  // German has no short form for thousands: they are written out.
  assert.equal(formatTokens(12_940), "12.940");
  setLanguage("nl");
  assert.equal(formatTokens(7_900_000), "7,9 mln.");
  assert.equal(formatTokens(12_940), "12,9K");
  setLanguage("en");
});
