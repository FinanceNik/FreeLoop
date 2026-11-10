import { test } from "node:test";
import assert from "node:assert/strict";
import { language, setLanguage } from "./i18n.ts";
import { decisionStats, duration, heldStats, efficiency, eventCount, forecast, heatLevel, heatmap, perDay, repeatApprovals, sessionStats, timeUnit, toolStats, turnStats, weekdayIndex, weekdayName, WEEKDAYS, type ActivityRow, type EffRow } from "./activity.ts";

const row = (p: Partial<ActivityRow>): ActivityRow => ({ day: "2026-10-12", hour: 9, sessionId: "s1", kind: "tool", name: "shell", count: 1, failures: 0, millis: 0, ...p });
const use = (p: Partial<EffRow>): EffRow => ({ day: "2026-10-12", sessionId: "s1", providerId: "p", model: "big", inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0, calls: 1, cost: 0, priced: false, ...p });

test("duration says a length of time in its largest units", () => {
  assert.equal(duration(0), "0s");
  assert.equal(duration(400), "<1s");
  assert.equal(duration(42_000), "42s");
  assert.equal(duration(125_000), "2m 5s");
  assert.equal(duration(300_000), "5m");
  assert.equal(duration(20 * 60_000), "20m");
  assert.equal(duration(90 * 60_000), "1h 30m");
  assert.equal(duration(26 * 3_600_000), "1d 2h");
});

test("timeUnit counts a chart in round units", () => {
  assert.equal(timeUnit(90_000), 1000);
  assert.equal(timeUnit(10 * 60_000), 60_000);
  assert.equal(timeUnit(5 * 3_600_000), 3_600_000);
});

test("toolStats add up per tool, most used first", () => {
  const s = toolStats([row({ count: 3, failures: 1, millis: 3000 }), row({ day: "2026-10-13", count: 1, millis: 1000 }), row({ name: "read_file", count: 9 }), row({ kind: "turn", name: "end_turn" })]);
  assert.deepEqual(s.map((t) => t.name), ["read_file", "shell"]);
  assert.equal(s[1].calls, 4);
  assert.equal(s[1].failureRate, 0.25);
  assert.equal(s[1].average, 1000);
});

test("turnStats count turns and why they ended", () => {
  const s = turnStats([row({ kind: "turn", name: "end_turn", count: 3, millis: 6000 }), row({ kind: "turn", name: "cancelled", millis: 2000 })]);
  assert.equal(s.turns, 4);
  assert.equal(s.average, 2000);
  assert.deepEqual(s.reasons.map((r) => r.reason), ["end_turn", "cancelled"]);
});

test("decisionStats count who decided, and only the user's wait", () => {
  const s = decisionStats([
    row({ kind: "decision", name: "user:allowed", count: 2, millis: 4000 }),
    row({ kind: "decision", name: "user:denied", count: 2, millis: 4000 }),
    row({ kind: "decision", name: "rule:allowed", count: 5, millis: 9 }),
    row({ kind: "decision", name: "protected_path:denied" }),
  ]);
  assert.equal(s.total, 10);
  assert.equal(s.byUser, 4);
  assert.equal(s.denied, 3);
  assert.equal(s.waited, 8000);
  assert.equal(s.averageWait, 2000);
  assert.deepEqual(s.actors[0], { actor: "rule", allowed: 5, denied: 0, cancelled: 0, total: 5 });
  assert.equal(s.actors.find((a) => a.actor === "protected_path")?.denied, 1);
});

test("repeatApprovals leave out a once, a denial and what a rule covers", () => {
  const asked = (name: string, p: Partial<ActivityRow>) => row({ kind: "asked", name, count: 0, ...p });
  const out = repeatApprovals([
    asked("shell\tnpm test\t", { count: 3, millis: 3000 }),
    asked("shell\tnpm test\t", { day: "2026-10-13", count: 1 }),
    asked("shell\tls\t", { count: 1 }),
    asked("shell\trm x\t", { count: 4, failures: 1 }),
    asked("shell\tmake\t", { count: 5, covered: true }),
    asked("write_file\t\t/a/b.txt", { count: 2 }),
  ]);
  assert.deepEqual(out, [
    { tool: "shell", command: "npm test", path: "", allowed: 4, waited: 3000 },
    { tool: "write_file", command: "", path: "/a/b.txt", allowed: 2, waited: 0 },
  ]);
});

test("heatmap puts turns on their weekday and hour", () => {
  assert.equal(weekdayIndex("2026-10-12"), 0); // a Monday
  assert.equal(weekdayIndex("2026-10-18"), 6);
  const h = heatmap([row({ kind: "turn", count: 2 }), row({ kind: "turn", day: "2026-10-19", count: 3 }), row({ kind: "turn", day: "2026-10-18", hour: 23 }), row({ count: 50 })]);
  assert.equal(h.cells[0][9], 5);
  assert.equal(h.cells[6][23], 1);
  assert.equal(h.max, 5);
});

test("heatLevel keeps zero for none and the top step for the most", () => {
  assert.equal(heatLevel(0, 10), 0);
  assert.equal(heatLevel(1, 100), 1);
  assert.equal(heatLevel(10, 10), 5);
  assert.equal(heatLevel(1, 0), 0);
});

test("perDay fills the empty days", () => {
  const d = perDay([row({ kind: "turn", day: "2026-10-13", count: 2 })], ["2026-10-12", "2026-10-13"], "turn", (r) => r.count);
  assert.deepEqual(d.map((x) => x.total), [0, 2]);
  assert.deepEqual(d[1].values, [2]);
});

test("sessionStats and eventCount", () => {
  const rows = [row({ count: 2, failures: 1 }), row({ kind: "turn", name: "end_turn", millis: 500 }), row({ sessionId: "s2", count: 9 }), row({ kind: "event", name: "edit", count: 4 })];
  const s = sessionStats(rows);
  assert.deepEqual(s.map((x) => x.sessionId), ["s2", "s1"]);
  assert.deepEqual(s[1], { sessionId: "s1", turns: 1, toolCalls: 2, failures: 1, millis: 500 });
  assert.equal(eventCount(rows, "edit"), 4);
  assert.equal(eventCount(rows, "rewind"), 0);
});

test("efficiency shares a session's turns among its models by calls", () => {
  const usage = [
    use({ sessionId: "s1", model: "big", calls: 3, inputTokens: 300, outputTokens: 30, cacheReadTokens: 100, cost: 3, priced: true, savedByCache: 1 }),
    use({ sessionId: "s1", model: "small", calls: 1, inputTokens: 10 }),
    use({ sessionId: "s2", model: "big", calls: 1, inputTokens: 70 }),
  ];
  const e = efficiency(usage, [row({ kind: "turn", name: "end_turn", count: 4 }), row({ kind: "turn", name: "end_turn", sessionId: "s2", count: 1 })], (r) => r.model);
  const big = e.find((g) => g.key === "big")!;
  const small = e.find((g) => g.key === "small")!;
  assert.equal(big.turns, 4); // 3 of the 4 turns, and the other session's one
  assert.equal(small.turns, 1);
  assert.equal(big.tokens, 500);
  assert.equal(big.tokensPerTurn, 125);
  assert.equal(big.costPerTurn, 0.75);
  assert.equal(big.saved, 1);
  assert.equal(big.outputPerCall, 7.5);
  assert.equal(big.cacheShare, 100 / 470);
  assert.equal(big.priced, true);
  assert.equal(small.priced, false);
});

test("forecast adds the last two weeks' pace to the month so far", () => {
  const f = forecast(
    [
      use({ day: "2026-11-10", inputTokens: 100, cost: 14, priced: true }), // in the month and the pace
      use({ day: "2026-11-01", inputTokens: 100, cost: 1, priced: true }), // in the month only
      use({ day: "2026-10-31", inputTokens: 100, cost: 50, priced: true }), // neither
    ],
    "2026-11-20",
  )!;
  assert.equal(f.spent, 15);
  assert.equal(f.month, "2026-11-01");
  assert.equal(f.projected, 15 + 10); // 10 days left at 1 a day
  assert.equal(f.tokens, 200);
  assert.ok(forecast([use({ day: "2026-01-01", inputTokens: 5 })], "2026-11-10") === null);
});

test("heldStats counts what was stopped, without a declined form", () => {
  const s = heldStats([
    row({ kind: "decision", name: "user:denied", count: 3, sessionId: "a", title: "A" }),
    row({ kind: "event", name: "form_declined", count: 1, sessionId: "a", title: "A" }),
    row({ kind: "decision", name: "rule:denied", count: 2, sessionId: "b", title: "B" }),
    row({ kind: "decision", name: "protected_path:denied", sessionId: "b", title: "B" }),
    row({ kind: "decision", name: "user:allowed", count: 9, sessionId: "a" }),
    row({ kind: "decision", name: "user:cancelled", count: 4, sessionId: "a" }),
    row({ kind: "event", name: "mode_refused", sessionId: "c", title: "C", deleted: true }),
  ]);
  assert.equal(s.total, 6);
  assert.deepEqual(s.kinds.find((k) => k.kind === "user"), { kind: "user", count: 2 });
  assert.deepEqual(s.kinds.find((k) => k.kind === "mode_change"), { kind: "mode_change", count: 1 });
  assert.deepEqual(s.sessions.map((x) => [x.sessionId, x.count]), [["b", 3], ["a", 2], ["c", 1]]);
  assert.equal(s.sessions[2].deleted, true);
});

test("heldStats is zero with nothing held, and a declined form alone is nothing", () => {
  assert.deepEqual(heldStats([row({ kind: "decision", name: "user:allowed" })]), { total: 0, kinds: [], sessions: [] });
  const form = heldStats([
    row({ kind: "decision", name: "user:denied", sessionId: "a" }),
    row({ kind: "event", name: "form_declined", sessionId: "a" }),
  ]);
  assert.equal(form.total, 0);
  assert.deepEqual(form.sessions, []);
});

test("durations and weekday names follow the language", () => {
  const before = language();
  setLanguage("de");
  assert.equal(duration(0), "0\u00a0s");
  assert.equal(duration(400), "<1\u00a0s");
  assert.equal(duration(125_000), "2\u00a0min 5\u00a0s");
  assert.equal(duration(90 * 60_000), "1\u00a0h 30\u00a0min");
  assert.equal(duration(26 * 3_600_000), "1\u00a0d 2\u00a0h");
  // The abbreviation's full stop depends on the ICU data.
  assert.deepEqual([0, 1, 6].map((d) => weekdayName(d).replace(/\.$/, "")), ["Mo", "Di", "So"]);
  assert.equal(WEEKDAYS[2].replace(/\.$/, ""), "Mi");
  setLanguage("nl");
  assert.equal(duration(42_000), "42\u00a0s");
  assert.equal(duration(125_000), "2\u00a0min 5\u00a0s");
  assert.equal(duration(90 * 60_000), "1\u00a0uur 30\u00a0min");
  assert.equal(duration(26 * 3_600_000), "1\u00a0d 2\u00a0uur");
  assert.deepEqual([0, 1, 6].map(weekdayName), ["ma", "di", "zo"]);
  setLanguage("en");
  assert.deepEqual([0, 1, 6].map(weekdayName), ["Mon", "Tue", "Sun"]);
  assert.equal(WEEKDAYS.length, 7);
  setLanguage(before);
});
