// The usage overview (GUI-033): a year at a glance, from the same rows as the dashboard.
// Pure, so overview.test.ts covers it.

import { days, tokensOf, type Row } from "./analytics.ts";
import { weekdayIndex } from "./activity.ts";
import { locale } from "./i18n.ts";

export const OTHER_MODEL = "Other";
// An agent need not say which model it ran, as with its default in the early rows.
export const modelName = (model: string) => model || "Model not reported";
// Colours, as on the dashboard; a sixth model and beyond fold into Other.
const SERIES = 5;
const WEEKS = 53;

const tokens = (r: Row) => tokensOf({ input: r.inputTokens, output: r.outputTokens, cacheRead: r.cacheReadTokens, cacheWrite: r.cacheWriteTokens });

export interface CalendarDay {
  day: string;
  tokens: number;
  // Most first; what colours the day is the first.
  models: { model: string; tokens: number }[];
}

export interface Overview {
  total: number;
  activeDays: number;
  peak: { day: string; tokens: number } | null;
  currentStreak: number;
  longestStreak: number;
  // Models with a colour of their own, most used first, for the whole period.
  models: string[];
}

// perDay sums rows into every day that has any.
function perDay(rows: Row[]): Map<string, CalendarDay> {
  const by = new Map<string, Map<string, number>>();
  for (const r of rows) {
    const n = tokens(r);
    if (n <= 0) continue;
    const day = by.get(r.day) ?? new Map<string, number>();
    const model = modelName(r.model);
    day.set(model, (day.get(model) ?? 0) + n);
    by.set(r.day, day);
  }
  const out = new Map<string, CalendarDay>();
  for (const [day, models] of by) {
    const list = [...models].map(([model, n]) => ({ model, tokens: n })).sort((a, b) => b.tokens - a.tokens || a.model.localeCompare(b.model));
    out.set(day, { day, tokens: list.reduce((s, m) => s + m.tokens, 0), models: list });
  }
  return out;
}

// topModels names the models that keep a colour whatever the day, most used first.
function topModels(rows: Row[]): string[] {
  const by = new Map<string, number>();
  for (const r of rows) by.set(modelName(r.model), (by.get(modelName(r.model)) ?? 0) + tokens(r));
  const list = [...by].filter(([, n]) => n > 0).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([m]) => m);
  return list.length <= SERIES ? list : list.slice(0, SERIES - 1);
}

// usageOverview adds up the period. A streak is days in a row with any use; today not
// being over yet, one that reached yesterday is still current.
export function usageOverview(rows: Row[], today: string): Overview {
  const byDay = perDay(rows);
  const active = [...byDay.keys()].sort();
  let longest = 0;
  let run = 0;
  let previous = "";
  for (const day of active) {
    run = previous && nextDay(previous) === day ? run + 1 : 1;
    longest = Math.max(longest, run);
    previous = day;
  }
  let current = 0;
  let at = byDay.has(today) ? today : lastDay(today);
  while (byDay.has(at)) {
    current++;
    at = lastDay(at);
  }
  let peak: Overview["peak"] = null;
  for (const d of byDay.values()) if (!peak || d.tokens > peak.tokens) peak = { day: d.day, tokens: d.tokens };
  return {
    total: [...byDay.values()].reduce((s, d) => s + d.tokens, 0),
    activeDays: active.length,
    peak,
    currentStreak: current,
    longestStreak: longest,
    models: topModels(rows),
  };
}

const shift = (day: string, by: number) => {
  const d = new Date(`${day}T12:00:00`);
  d.setDate(d.getDate() + by);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};
const nextDay = (day: string) => shift(day, 1);
const lastDay = (day: string) => shift(day, -1);

// calendar lays out the last year in columns of a week, Monday first, ending this week.
// Days still to come are null.
export function calendar(rows: Row[], today: string): (CalendarDay | null)[][] {
  const byDay = perDay(rows);
  const start = shift(today, -weekdayIndex(today) - 7 * (WEEKS - 1));
  const end = shift(today, 6 - weekdayIndex(today));
  const list = days(start, end);
  const weeks: (CalendarDay | null)[][] = [];
  for (let i = 0; i < list.length; i += 7) {
    weeks.push(list.slice(i, i + 7).map((day) => (day > today ? null : (byDay.get(day) ?? { day, tokens: 0, models: [] }))));
  }
  return weeks;
}

// monthLabels says which week columns a month starts in, leaving room for its name.
export function monthLabels(weeks: (CalendarDay | null)[][]): { week: number; label: string }[] {
  const out: { week: number; label: string }[] = [];
  let seen = "";
  weeks.forEach((week, i) => {
    const first = week[0]?.day ?? week.find(Boolean)?.day;
    if (!first) return;
    const month = first.slice(0, 7);
    if (month === seen) return;
    seen = month;
    if (out.length > 0 && i - out[out.length - 1].week < 3) out.pop();
    out.push({ week: i, label: new Date(`${first}T12:00:00`).toLocaleDateString(locale(), { month: "short" }) });
  });
  return out;
}

// dayLevel places a day on the five steps of its colour. Under a square root, or one
// enormous day would leave every other pale to the point of looking empty.
export function dayLevel(n: number, max: number): number {
  if (n <= 0 || max <= 0) return 0;
  return Math.min(5, Math.max(1, Math.ceil(Math.sqrt(n / max) * 5)));
}

// longestChat is the session that spent the most time in turns.
export function longestChat(rows: { sessionId: string; kind: string; millis: number }[]): { sessionId: string; millis: number } | null {
  const by = new Map<string, number>();
  for (const r of rows) if (r.kind === "turn") by.set(r.sessionId, (by.get(r.sessionId) ?? 0) + r.millis);
  let best: { sessionId: string; millis: number } | null = null;
  for (const [sessionId, millis] of by) if (millis > 0 && (!best || millis > best.millis)) best = { sessionId, millis };
  return best;
}
