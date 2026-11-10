// The analytics beyond tokens (GUI-021): what the agents did, who decided what,
// and what a turn costs, from the engine's rows of one hour, session and kind.
// Pure, so activity.test.ts covers it.

import { tokensOf, type Row } from "./analytics.ts";
import { locale, t } from "./i18n.ts";

export interface ActivityRow {
  day: string;
  hour: number;
  sessionId: string;
  kind: string; // tool, turn, decision, asked or event
  name: string;
  count: number;
  failures: number;
  millis: number;
  covered?: boolean; // a rule already answers this question
  title?: string;
  deleted?: boolean;
}

const of = <R extends ActivityRow>(rows: R[], kind: string) => rows.filter((r) => r.kind === kind);

export function eventCount(rows: ActivityRow[], name: string): number {
  return rows.reduce((n, r) => (r.kind === "event" && r.name === name ? n + r.count : n), 0);
}

// duration is a length of time in the largest units that say it, written as the language
// writes its units (GUI-048).
export function duration(ms: number): string {
  if (ms <= 0) return t("activity.duration.zero");
  if (ms < 1000) return t("activity.duration.underSecond");
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return t("activity.duration.seconds", { seconds });
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return seconds < 600 && seconds % 60 ? t("activity.duration.minutesSeconds", { minutes: Math.floor(seconds / 60), seconds: seconds % 60 }) : t("activity.duration.minutes", { minutes });
  const hours = Math.floor(minutes / 60);
  return hours < 24 ? t("activity.duration.hoursMinutes", { hours, minutes: minutes % 60 }) : t("activity.duration.daysHours", { days: Math.floor(hours / 24), hours: hours % 24 });
}

// timeUnit picks the unit a chart of durations counts in, so its axis falls on
// round seconds, minutes or hours.
export function timeUnit(maxMillis: number): number {
  if (maxMillis < 120_000) return 1000;
  return maxMillis < 7_200_000 ? 60_000 : 3_600_000;
}

export interface ToolStat {
  name: string;
  calls: number;
  failures: number;
  failureRate: number; // 0 to 1
  millis: number;
  average: number; // per call
}

export function toolStats(rows: ActivityRow[]): ToolStat[] {
  const by = new Map<string, ToolStat>();
  for (const r of of(rows, "tool")) {
    const t = by.get(r.name) ?? { name: r.name, calls: 0, failures: 0, failureRate: 0, millis: 0, average: 0 };
    t.calls += r.count;
    t.failures += r.failures;
    t.millis += r.millis;
    by.set(r.name, t);
  }
  const out = [...by.values()];
  for (const t of out) {
    t.failureRate = t.calls ? t.failures / t.calls : 0;
    t.average = t.calls ? t.millis / t.calls : 0;
  }
  return out.sort((a, b) => b.calls - a.calls || a.name.localeCompare(b.name));
}

export interface TurnStats {
  turns: number;
  millis: number;
  average: number;
  reasons: { reason: string; count: number; millis: number }[];
}

export function turnStats(rows: ActivityRow[]): TurnStats {
  const by = new Map<string, { reason: string; count: number; millis: number }>();
  let turns = 0;
  let millis = 0;
  for (const r of of(rows, "turn")) {
    const g = by.get(r.name) ?? { reason: r.name, count: 0, millis: 0 };
    g.count += r.count;
    g.millis += r.millis;
    by.set(r.name, g);
    turns += r.count;
    millis += r.millis;
  }
  return { turns, millis, average: turns ? millis / turns : 0, reasons: [...by.values()].sort((a, b) => b.count - a.count) };
}

export interface DecisionStats {
  total: number;
  byUser: number; // answered by the user
  denied: number; // denied by anyone
  waited: number; // milliseconds agents waited for the user
  averageWait: number; // per question answered
  actors: { actor: string; allowed: number; denied: number; cancelled: number; total: number }[];
}

export function decisionStats(rows: ActivityRow[]): DecisionStats {
  const by = new Map<string, DecisionStats["actors"][number]>();
  const out: DecisionStats = { total: 0, byUser: 0, denied: 0, waited: 0, averageWait: 0, actors: [] };
  for (const r of of(rows, "decision")) {
    const sep = r.name.lastIndexOf(":");
    const actor = r.name.slice(0, sep);
    const outcome = r.name.slice(sep + 1);
    const a = by.get(actor) ?? { actor, allowed: 0, denied: 0, cancelled: 0, total: 0 };
    if (outcome === "allowed") a.allowed += r.count;
    else if (outcome === "denied") a.denied += r.count;
    else a.cancelled += r.count;
    a.total += r.count;
    by.set(actor, a);
    out.total += r.count;
    if (outcome === "denied") out.denied += r.count;
    if (actor === "user") {
      out.byUser += r.count;
      out.waited += r.millis;
    }
  }
  out.averageWait = out.byUser ? out.waited / out.byUser : 0;
  out.actors = [...by.values()].sort((a, b) => b.total - a.total);
  return out;
}

export interface HeldStats {
  total: number;
  kinds: { kind: string; count: number }[];
  sessions: { sessionId: string; title: string; deleted: boolean; count: number }[];
}

// heldStats counts what the user's policy stopped (SEC-014): every denied decision
// but a declined form, and an agent's own switch to a looser mode. The kinds are
// those of HELD_BY in transcript.ts.
export function heldStats(rows: ActivityRow[]): HeldStats {
  const kinds = new Map<string, number>();
  const sessions = new Map<string, HeldStats["sessions"][number]>();
  const add = (r: ActivityRow, kind: string, n: number) => {
    kinds.set(kind, (kinds.get(kind) ?? 0) + n);
    const s = sessions.get(r.sessionId) ?? { sessionId: r.sessionId, title: r.title ?? "", deleted: !!r.deleted, count: 0 };
    s.count += n;
    sessions.set(r.sessionId, s);
  };
  for (const r of rows) {
    if (r.kind === "decision" && r.name.endsWith(":denied")) add(r, r.name.slice(0, -":denied".length), r.count);
    else if (r.kind === "event" && r.name === "mode_refused") add(r, "mode_change", r.count);
    else if (r.kind === "event" && r.name === "form_declined") add(r, "user", -r.count);
  }
  const positive = <T extends { count: number }>(l: T[]) => l.filter((x) => x.count > 0).sort((a, b) => b.count - a.count);
  const list = positive([...kinds].map(([kind, count]) => ({ kind, count })));
  return { total: list.reduce((n, k) => n + k.count, 0), kinds: list, sessions: positive([...sessions.values()]) };
}

export interface Repeat {
  tool: string;
  command: string;
  path: string;
  allowed: number;
  waited: number;
}

// repeatApprovals finds what the user keeps allowing and has never denied, where
// no rule answers it yet: each one is a question a rule would stop. The engine
// packs the question into the name, tool, command and path by tabs.
export function repeatApprovals(rows: ActivityRow[], atLeast = 2): Repeat[] {
  const by = new Map<string, Repeat & { denied: number; covered: boolean }>();
  for (const r of of(rows, "asked")) {
    const [tool, command = "", path = ""] = r.name.split("\t");
    const g = by.get(r.name) ?? { tool, command, path, allowed: 0, waited: 0, denied: 0, covered: false };
    g.allowed += r.count;
    g.denied += r.failures;
    g.waited += r.millis;
    g.covered ||= !!r.covered;
    by.set(r.name, g);
  }
  return [...by.values()]
    .filter((g) => g.allowed >= atLeast && g.denied === 0 && !g.covered)
    .sort((a, b) => b.allowed - a.allowed || b.waited - a.waited)
    .map(({ tool, command, path, allowed, waited }) => ({ tool, command, path, allowed, waited }));
}

const DAYS_OF_WEEK = 7;

// weekdayName is the short name of a weekday in the language in force, 0 for Monday: Mon,
// Mo., ma. 2024-01-01 was a Monday.
export function weekdayName(index: number): string {
  return new Intl.DateTimeFormat(locale(), { weekday: "short", timeZone: "UTC" }).format(new Date(Date.UTC(2024, 0, 1 + index)));
}

// WEEKDAYS reads as the list of short names, written when read, so a change of language
// shows at once; use weekdayName where an index is at hand.
export const WEEKDAYS: readonly string[] = new Proxy(Array.from({ length: DAYS_OF_WEEK }, () => ""), {
  get: (days, key) => (typeof key === "string" && /^[0-6]$/.test(key) ? weekdayName(Number(key)) : Reflect.get(days, key)),
});

// weekdayIndex is 0 for Monday.
export function weekdayIndex(day: string): number {
  return (new Date(`${day}T12:00:00`).getDay() + 6) % 7;
}

export interface Heatmap {
  cells: number[][]; // by weekday from Monday, then by hour
  max: number;
}

// heatmap counts turns by the weekday and hour they started.
export function heatmap(rows: ActivityRow[]): Heatmap {
  const cells = Array.from({ length: DAYS_OF_WEEK }, () => new Array<number>(24).fill(0));
  let max = 0;
  for (const r of of(rows, "turn")) {
    const row = cells[weekdayIndex(r.day)];
    row[r.hour] += r.count;
    max = Math.max(max, row[r.hour]);
  }
  return { cells, max };
}

// heatLevel places a count on the five steps of the scale; 0 is only for none.
export function heatLevel(count: number, max: number): number {
  if (count <= 0 || max <= 0) return 0;
  return Math.min(5, Math.ceil((count / max) * 5));
}

// perDay sums something per day over dayList, for a chart of one series.
export function perDay(rows: ActivityRow[], dayList: string[], kind: string, value: (r: ActivityRow) => number) {
  const by = new Map<string, number>();
  for (const r of of(rows, kind)) by.set(r.day, (by.get(r.day) ?? 0) + value(r));
  return dayList.map((day) => {
    const v = by.get(day) ?? 0;
    return { day, values: [v], total: v };
  });
}

export interface SessionStat {
  sessionId: string;
  turns: number;
  toolCalls: number;
  failures: number;
  millis: number; // in turns
}

export function sessionStats(rows: ActivityRow[]): SessionStat[] {
  const by = new Map<string, SessionStat>();
  for (const r of rows) {
    if (r.kind !== "tool" && r.kind !== "turn") continue;
    const s = by.get(r.sessionId) ?? { sessionId: r.sessionId, turns: 0, toolCalls: 0, failures: 0, millis: 0 };
    if (r.kind === "turn") {
      s.turns += r.count;
      s.millis += r.millis;
    } else {
      s.toolCalls += r.count;
      s.failures += r.failures;
    }
    by.set(r.sessionId, s);
  }
  return [...by.values()].sort((a, b) => b.toolCalls - a.toolCalls || b.turns - a.turns);
}

export interface EffRow extends Row {
  savedByCache?: number;
}

export interface Efficiency {
  key: string;
  turns: number; // shared out by model calls, so not always whole
  calls: number;
  tokens: number;
  output: number;
  cost: number;
  saved: number;
  cacheShare: number;
  priced: boolean;
  tokensPerTurn: number;
  costPerTurn: number;
  outputPerCall: number;
}

// efficiency works out what a turn takes per group. Turns are counted per
// session, not per model: a session's turns are shared among its models by their
// share of its model calls, which is exact for the usual session of one model.
export function efficiency<R extends EffRow>(usage: R[], activity: ActivityRow[], key: (r: R) => string): Efficiency[] {
  const turns = new Map<string, number>();
  for (const r of of(activity, "turn")) turns.set(r.sessionId, (turns.get(r.sessionId) ?? 0) + r.count);
  const calls = new Map<string, number>();
  for (const r of usage) calls.set(r.sessionId, (calls.get(r.sessionId) ?? 0) + r.calls);
  const by = new Map<string, Efficiency & { input: number; cacheRead: number }>();
  for (const r of usage) {
    const k = key(r);
    const g = by.get(k) ?? { key: k, turns: 0, calls: 0, tokens: 0, output: 0, cost: 0, saved: 0, cacheShare: 0, priced: false, tokensPerTurn: 0, costPerTurn: 0, outputPerCall: 0, input: 0, cacheRead: 0 };
    const share = (calls.get(r.sessionId) ?? 0) ? r.calls / calls.get(r.sessionId)! : 0;
    g.turns += (turns.get(r.sessionId) ?? 0) * share;
    g.calls += r.calls;
    g.tokens += tokensOf({ input: r.inputTokens, output: r.outputTokens, cacheRead: r.cacheReadTokens, cacheWrite: r.cacheWriteTokens });
    g.output += r.outputTokens;
    g.input += r.inputTokens + r.cacheWriteTokens;
    g.cacheRead += r.cacheReadTokens;
    if (r.priced) {
      g.cost += r.cost;
      g.saved += r.savedByCache ?? 0;
      g.priced = true;
    }
    by.set(k, g);
  }
  return [...by.values()]
    .map(({ input, cacheRead, ...g }) => ({
      ...g,
      cacheShare: input + cacheRead ? cacheRead / (input + cacheRead) : 0,
      tokensPerTurn: g.turns ? g.tokens / g.turns : 0,
      costPerTurn: g.turns ? g.cost / g.turns : 0,
      outputPerCall: g.calls ? g.output / g.calls : 0,
    }))
    .sort((a, b) => b.tokens - a.tokens);
}

export interface Forecast {
  spent: number; // this month so far
  tokens: number;
  projected: number; // by the end of the month
  projectedTokens: number;
  month: string; // the month's first day
}

const PACE_DAYS = 14;

const addDays = (day: string, n: number) => {
  const d = new Date(`${day}T12:00:00`);
  d.setDate(d.getDate() + n);
  const pad = (v: number) => String(v).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};

// forecast adds the pace of the last two weeks to this month so far. It is only a
// projection: nothing when those weeks were empty, as there is no pace to go on.
export function forecast(usage: Row[], today: string): Forecast | null {
  const month = `${today.slice(0, 7)}-01`;
  const paceFrom = addDays(today, 1 - PACE_DAYS);
  const tokensOfRow = (r: Row) => tokensOf({ input: r.inputTokens, output: r.outputTokens, cacheRead: r.cacheReadTokens, cacheWrite: r.cacheWriteTokens });
  const costOf = (r: Row) => (r.priced ? r.cost : 0);
  const sum = (from: string, f: (r: Row) => number) => usage.reduce((n, r) => (r.day >= from && r.day <= today ? n + f(r) : n), 0);
  const paceTokens = sum(paceFrom, tokensOfRow);
  if (paceTokens === 0) return null;
  const d = new Date(`${today}T12:00:00`);
  const left = new Date(d.getFullYear(), d.getMonth() + 1, 0).getDate() - d.getDate();
  const spent = sum(month, costOf);
  const tokens = sum(month, tokensOfRow);
  return {
    spent,
    tokens,
    projected: spent + (sum(paceFrom, costOf) / PACE_DAYS) * left,
    projectedTokens: tokens + (paceTokens / PACE_DAYS) * left,
    month,
  };
}
