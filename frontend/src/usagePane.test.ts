import assert from "node:assert/strict";
import { test } from "node:test";
import type { Row } from "./analytics.ts";
import { cycleView, PANE_VIEWS, period, viewIndex } from "./usagePane.ts";
import { setLanguage, t } from "./i18n.ts";

test("the views go round in both directions", () => {
  const n = PANE_VIEWS.length;
  assert.equal(cycleView(0, 1, n), 1);
  assert.equal(cycleView(n - 1, 1, n), 0);
  assert.equal(cycleView(0, -1, n), n - 1);
  assert.equal(cycleView(2, -5, n), cycleView(2, -1, n));
  assert.equal(cycleView(0, 1, 0), 0);
});

test("a remembered view comes back, and anything else is the first", () => {
  assert.equal(viewIndex("year"), 1);
  assert.equal(viewIndex("tokens"), 2);
  assert.equal(viewIndex(null), 0);
  assert.equal(viewIndex("gone"), 0);
  assert.equal(viewIndex(""), 0);
});

const row = (day: string, input: number): Row =>
  ({ day, sessionId: `s-${day}`, inputTokens: input, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0, calls: 1, cost: 0, priced: false }) as Row;

test("a period is the last days up to today, against the same span before", () => {
  const rows = [row("2026-11-06", 30), row("2026-11-01", 10), row("2026-10-31", 20), row("2026-10-24", 5), row("2026-10-23", 999)];
  const p = period(rows, 7, "2026-11-06");
  assert.equal(p.since, "2026-10-31");
  assert.equal(p.now.input, 60);
  // 2026-10-24 to 2026-10-30 is the week before; 2026-10-23 is outside both.
  assert.equal(p.delta, (60 - 5) / 5);
});

// GUI-048: the views are named in the language in force.
test("the views' names follow the language", () => {
  assert.deepEqual(PANE_VIEWS.map((v) => t(v.label)), ["Recent hours", "Last 12 months", "Tokens", "By model"]);
  setLanguage("de");
  assert.deepEqual(PANE_VIEWS.map((v) => t(v.label)), ["Letzte Stunden", "Letzte 12 Monate", "Token", "Nach Modell"]);
  setLanguage("nl");
  assert.equal(t(PANE_VIEWS[0].label), "Afgelopen uren");
  setLanguage("en");
});

test("with nothing before, there is no change to show", () => {
  assert.equal(period([row("2026-11-06", 1)], 30, "2026-11-06").delta, null);
});
