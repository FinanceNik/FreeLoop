import { test } from "node:test";
import assert from "node:assert/strict";
import { currentLimits, limitSummaries, percentText, reportAge, resetText } from "./limits.ts";
import { setLanguage } from "./i18n.ts";

const now = new Date("2026-11-03T12:00:00Z");
const run = (window: string, resetsAt: string, peak: number, more = {}) => ({
  source: "Agent",
  window,
  label: `${window} limit`,
  resetsAt,
  used: peak,
  peak,
  lastReport: "2026-11-03T11:00:00Z",
  warned: 0,
  refused: 0,
  ...more,
});

test("each limit shows its newest run, the soonest to reset first", () => {
  const got = currentLimits(
    [run("week", "2026-11-09T00:00:00Z", 20), run("hours", "2026-11-03T15:00:00Z", 11), run("hours", "2026-11-03T10:00:00Z", 90)],
    now,
  );
  assert.deepEqual(got.map((l) => [l.label, l.used, l.state]), [
    ["hours limit", 11, "ok"],
    ["week limit", 20, "ok"],
  ]);
});

test("the peak shows, since parallel sessions report in any order", () => {
  const [l] = currentLimits([run("hours", "2026-11-03T15:00:00Z", 40, { used: 30 })], now);
  assert.equal(l.used, 40);
});

test("what is close or refused says so, and a run that has reset claims no number", () => {
  const state = (r: Record<string, unknown>, at = "2026-11-03T15:00:00Z") => currentLimits([run("hours", at, 10, r)], now)[0];
  assert.equal(state({ peak: 80 }).state, "warning");
  assert.equal(state({ warned: 1 }).state, "warning");
  assert.equal(state({ refused: 2, warned: 1 }).state, "refused");
  const over = state({ peak: 99 }, "2026-11-03T11:59:00Z");
  assert.equal(over.state, "reset");
  assert.equal(over.used, null);
});

test("two agents' limits stay apart", () => {
  const got = currentLimits([run("hours", "2026-11-03T15:00:00Z", 1), run("hours", "2026-11-03T15:00:00Z", 2, { source: "Other" })], now);
  assert.equal(got.length, 2);
});

test("the summary takes finished runs only", () => {
  const got = limitSummaries(
    [
      run("hours", "2026-11-03T15:00:00Z", 5),
      run("hours", "2026-11-03T10:00:00Z", 100, { refused: 1 }),
      run("hours", "2026-11-03T05:00:00Z", 85),
      run("hours", "2026-11-02T05:00:00Z", 30),
    ],
    now,
  );
  assert.equal(got.length, 1);
  assert.deepEqual(
    { runs: got[0].runs, average: Math.round(got[0].averagePeak), highest: got[0].highest, close: got[0].closeRuns, refused: got[0].refusedRuns },
    { runs: 3, average: 72, highest: 100, close: 2, refused: 1 },
  );
});

test("words for use and for the reset", () => {
  assert.equal(percentText(0.4), "under 1%");
  assert.equal(percentText(0), "0%");
  assert.equal(percentText(11.4), "11%");
  const after = (minutes: number) => resetText(new Date(now.getTime() + minutes * 60000), now);
  assert.equal(after(-1), "has reset");
  assert.equal(after(45), "resets in 45 min");
  assert.equal(after(125), "resets in 2 h 5 min");
  assert.equal(after(60 * 25), "resets in 1 day 1 h");
  assert.equal(after(60 * 49), "resets in 2 days 1 h");
});

test("a fresh report says nothing of its age, an old one does", () => {
  const since = (_t: string | Date) => "12 min ago";
  assert.equal(reportAge(new Date(now.getTime() - 5 * 60000), now, since), "");
  assert.equal(reportAge(new Date(now.getTime() - 12 * 60000), now, since), "reported 12 min ago");
});

// GUI-048: the words and the percentages follow the language.
test("use, the reset and a report's age are in the language in force", () => {
  const after = (minutes: number) => resetText(new Date(now.getTime() + minutes * 60000), now);
  setLanguage("de");
  // German puts a no-break space before the per cent sign.
  assert.equal(percentText(11.4), "11\u00a0%");
  assert.equal(percentText(0.4), "unter 1\u00a0%");
  assert.equal(after(-1), "wurde zurückgesetzt");
  assert.equal(after(125), "wird in 2 h 5 min zurückgesetzt");
  assert.equal(after(60 * 25), "wird in 1 Tag 1 h zurückgesetzt");
  assert.equal(after(60 * 49), "wird in 2 Tagen 1 h zurückgesetzt");
  assert.equal(reportAge(new Date(now.getTime() - 12 * 60000), now, () => "vor 12 min"), "vor 12 min gemeldet");
  setLanguage("nl");
  assert.equal(percentText(11.4), "11%");
  assert.equal(after(45), "begint over 45 min opnieuw");
  assert.equal(after(60 * 49), "begint over 2 dagen 1 uur opnieuw");
  setLanguage("en");
});
