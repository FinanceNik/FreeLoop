import { test } from "node:test";
import assert from "node:assert/strict";
import { calendar, dayLevel, longestChat, monthLabels, usageOverview } from "./overview.ts";
import type { Row } from "./analytics.ts";

const row = (day: string, model: string, input: number, extra: Partial<Row> = {}): Row => ({
  day,
  sessionId: "s",
  providerId: "p",
  model,
  inputTokens: input,
  outputTokens: 0,
  cacheReadTokens: 0,
  cacheWriteTokens: 0,
  calls: 1,
  cost: 0,
  priced: false,
  ...extra,
});

test("the overview adds up tokens and finds the biggest day", () => {
  const o = usageOverview([row("2026-11-03", "a", 10), row("2026-11-03", "b", 5, { outputTokens: 5 }), row("2026-11-04", "a", 7)], "2026-11-04");
  assert.equal(o.total, 27);
  assert.equal(o.activeDays, 2);
  assert.deepEqual(o.peak, { day: "2026-11-03", tokens: 20 });
  assert.deepEqual(usageOverview([], "2026-11-04").peak, null);
});

test("a streak is days in a row, and today not being over does not end it", () => {
  const rows = ["2026-10-01", "2026-10-02", "2026-10-03", "2026-10-04", "2026-11-02", "2026-11-03"].map((d) => row(d, "a", 1));
  const yesterdayLast = usageOverview(rows, "2026-11-04");
  assert.equal(yesterdayLast.currentStreak, 2);
  assert.equal(yesterdayLast.longestStreak, 4);
  assert.equal(usageOverview([...rows, row("2026-11-04", "a", 1)], "2026-11-04").currentStreak, 3);
  assert.equal(usageOverview(rows, "2026-11-05").currentStreak, 0);
  // Across a month's end and a change of the clocks.
  const across = ["2026-10-24", "2026-10-25", "2026-10-26"].map((d) => row(d, "a", 1));
  assert.equal(usageOverview(across, "2026-10-26").longestStreak, 3);
  assert.equal(usageOverview(across, "2026-10-26").currentStreak, 3);
});

test("a row with no tokens does not count as a day of use", () => {
  const o = usageOverview([row("2026-11-04", "a", 0)], "2026-11-04");
  assert.equal(o.activeDays, 0);
  assert.equal(o.currentStreak, 0);
  assert.deepEqual(o.models, []);
});

test("models keep a colour by use over the period, and the rest fold", () => {
  const few = usageOverview([row("2026-11-04", "small", 1), row("2026-11-04", "big", 9)], "2026-11-04");
  assert.deepEqual(few.models, ["big", "small"]);
  const many = usageOverview(["a", "b", "c", "d", "e", "f"].map((m, i) => row("2026-11-04", m, 60 - i)), "2026-11-04");
  assert.deepEqual(many.models, ["a", "b", "c", "d"]);
});

test("the calendar is whole weeks, Monday first, ending this week", () => {
  // 2026-11-04 is a Wednesday.
  const weeks = calendar([row("2026-11-02", "a", 3), row("2026-11-02", "b", 8)], "2026-11-04");
  assert.equal(weeks.length, 53);
  assert.ok(weeks.every((w) => w.length === 7));
  const last = weeks[52];
  assert.equal(last[0]?.day, "2026-11-02");
  assert.equal(last[0]?.tokens, 11);
  assert.deepEqual(last[0]?.models.map((m) => m.model), ["b", "a"]);
  assert.equal(last[2]?.day, "2026-11-04");
  assert.equal(last[2]?.tokens, 0);
  assert.equal(last[3], null);
  assert.equal(last[6], null);
  assert.equal(weeks[0][0] && new Date(`${weeks[0][0].day}T12:00:00`).getDay(), 1);
});

test("months are named where they start, without crowding", () => {
  const labels = monthLabels(calendar([], "2026-11-04"));
  assert.ok(labels.length >= 11 && labels.length <= 13, String(labels.length));
  assert.equal(labels.at(-1)?.label, "Nov");
  for (let i = 1; i < labels.length; i++) assert.ok(labels[i].week - labels[i - 1].week >= 3);
});

test("a day's level has a square root, so one huge day leaves others visible", () => {
  assert.equal(dayLevel(0, 100), 0);
  assert.equal(dayLevel(100, 100), 5);
  assert.equal(dayLevel(1, 10_000_000), 1);
  // A tenth of the biggest is on the second step, where a straight scale would put it on the first.
  assert.equal(dayLevel(1, 100), 1);
  assert.equal(dayLevel(10, 100), 2);
  assert.equal(dayLevel(5, 0), 0);
});

test("the longest chat goes by time in turns", () => {
  assert.deepEqual(
    longestChat([
      { sessionId: "a", kind: "turn", millis: 100 },
      { sessionId: "b", kind: "turn", millis: 70 },
      { sessionId: "b", kind: "turn", millis: 70 },
      { sessionId: "c", kind: "tool", millis: 900 },
    ]),
    { sessionId: "b", millis: 140 },
  );
  assert.equal(longestChat([]), null);
});

test("a model no agent named still has a name", () => {
  const o = usageOverview([row("2026-11-04", "", 9), row("2026-11-04", "a", 1)], "2026-11-04");
  assert.deepEqual(o.models, ["Model not reported", "a"]);
  assert.equal(calendar([row("2026-11-04", "", 9)], "2026-11-04")[52][2]?.models[0].model, "Model not reported");
});
