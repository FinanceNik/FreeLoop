// The usage dashboard's numbers (PRV-008): totals, groups and the daily
// series, from the engine's rows of one day, session and model. Pure, so
// analytics.test.ts covers it.
import { numberFormat, t, type PlainKey } from "./i18n.ts";

export interface Row {
  day: string;
  sessionId: string;
  providerId: string;
  model: string;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  calls: number;
  cost: number;
  priced: boolean;
  // Cost per kind of token; missing on rows from before they were added.
  costInput?: number;
  costOutput?: number;
  costCacheRead?: number;
  costCacheWrite?: number;
}

export type Metric = "tokens" | "cost";

export interface Sums {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  calls: number;
  cost: number;
  // Tokens of models without a price, which the cost leaves out.
  unpricedTokens: number;
}

export const tokensOf = (s: Pick<Sums, "input" | "output" | "cacheRead" | "cacheWrite">) => s.input + s.output + s.cacheRead + s.cacheWrite;

const empty = (): Sums => ({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0, calls: 0, cost: 0, unpricedTokens: 0 });

function add(s: Sums, r: Row) {
  s.input += r.inputTokens;
  s.output += r.outputTokens;
  s.cacheRead += r.cacheReadTokens;
  s.cacheWrite += r.cacheWriteTokens;
  s.calls += r.calls;
  if (r.priced) s.cost += r.cost;
  else s.unpricedTokens += r.inputTokens + r.outputTokens + r.cacheReadTokens + r.cacheWriteTokens;
}

export function value(s: Sums, m: Metric): number {
  return m === "cost" ? s.cost : tokensOf(s);
}

export interface Totals extends Sums {
  sessions: number;
  days: number;
  // The share of input read from the prompt cache, 0 to 1.
  cacheShare: number;
}

// totals adds up rows; days counts the days with any use.
export function totals(rows: Row[]): Totals {
  const s = empty();
  const sessions = new Set<string>();
  const days = new Set<string>();
  for (const r of rows) {
    add(s, r);
    sessions.add(r.sessionId);
    days.add(r.day);
  }
  const prompt = s.input + s.cacheRead + s.cacheWrite;
  return { ...s, sessions: sessions.size, days: days.size, cacheShare: prompt ? s.cacheRead / prompt : 0 };
}

export interface Group extends Sums {
  key: string;
  sessions: number;
}

// groupBy adds up rows per key, the largest first by the metric.
export function groupBy<R extends Row>(rows: R[], key: (r: R) => string, metric: Metric): Group[] {
  const groups = new Map<string, Group & { ids: Set<string> }>();
  for (const r of rows) {
    const k = key(r);
    let g = groups.get(k);
    if (!g) groups.set(k, (g = { key: k, ...empty(), sessions: 0, ids: new Set() }));
    add(g, r);
    g.ids.add(r.sessionId);
  }
  return [...groups.values()]
    .map(({ ids, ...g }) => ({ ...g, sessions: ids.size }))
    .sort((a, b) => value(b, metric) - value(a, metric) || tokensOf(b) - tokensOf(a) || a.key.localeCompare(b.key));
}

// The key of the series the rest fold into: a name no group has; its label is the language's.
export const OTHER = "\u0000other";

// topKeys returns the keys that get a series of their own, at most n; the
// rest fold into Other. A key keeps its series as long as it stays among
// the top, whatever the order among them.
export function topKeys(groups: Group[], n: number): string[] {
  const keys = groups.map((g) => g.key);
  return keys.length <= n ? keys : keys.slice(0, n - 1);
}

// days lists every date from since to until, inclusive (YYYY-MM-DD).
export function days(since: string, until: string): string[] {
  const out: string[] = [];
  const d = new Date(`${since}T12:00:00Z`);
  const end = new Date(`${until}T12:00:00Z`);
  for (let i = 0; d <= end && i < 1000; i++) {
    out.push(d.toISOString().slice(0, 10));
    d.setUTCDate(d.getUTCDate() + 1);
  }
  return out;
}

export interface Day {
  day: string;
  // One value per series, in the series' order; Other last when present.
  values: number[];
  total: number;
}

// daily returns each day's value per series: the keys given, then Other
// for every other key when any.
export function daily<R extends Row>(rows: R[], dayList: string[], key: (r: R) => string, series: string[], metric: Metric): Day[] {
  const index = new Map(series.map((k, i) => [k, i]));
  const width = series.length + (series.includes(OTHER) ? 0 : 1);
  const byDay = new Map(dayList.map((d) => [d, new Array<number>(width).fill(0)]));
  for (const r of rows) {
    const vals = byDay.get(r.day);
    if (!vals) continue;
    const v = metric === "cost" ? (r.priced ? r.cost : 0) : r.inputTokens + r.outputTokens + r.cacheReadTokens + r.cacheWriteTokens;
    vals[index.get(key(r)) ?? width - 1] += v;
  }
  const hasOther = dayList.some((d) => byDay.get(d)![width - 1] > 0) && !series.includes(OTHER);
  return dayList.map((d) => {
    const vals = byDay.get(d)!;
    const values = hasOther || series.includes(OTHER) ? vals : vals.slice(0, series.length);
    return { day: d, values, total: values.reduce((a, b) => a + b, 0) };
  });
}

// change is the relative change from before to now, or null when there is
// nothing to compare with.
export function change(now: number, before: number): number | null {
  if (before <= 0) return null;
  return (now - before) / before;
}

// niceTicks returns about count round tick values from 0 to at least max.
export function niceTicks(max: number, count = 4): number[] {
  if (!(max > 0)) return [0];
  const raw = max / count;
  const mag = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? 10 * mag;
  const out: number[] = [];
  for (let v = 0; v < max + step * 0.999 && out.length < 20; v += step) out.push(Number(v.toPrecision(12)));
  return out;
}

// compact writes a count the short way, as the locale shortens it (GUI-048): 950, 12.9K,
// 4.2M, 1.1B in English, 7,9 Mio. in German, 7,9 mln. in Dutch. One decimal at most, and
// none from 100 up; a count the locale has no short form for (German thousands) is
// written whole.
export function compact(n: number): string {
  const short = numberFormat({ notation: "compact", maximumFractionDigits: 1 });
  const parts = short.formatToParts(n);
  if (!parts.some((p) => p.type === "compact")) return numberFormat({ maximumFractionDigits: 0 }).format(n);
  const whole = parts.filter((p) => p.type === "integer").reduce((len, p) => len + p.value.length, 0);
  return whole >= 3 ? numberFormat({ notation: "compact", maximumFractionDigits: 0 }).format(n) : short.format(n);
}

// weekStart returns the Monday on or before day (YYYY-MM-DD), for ranges
// too long to show a column per day.
export function weekStart(day: string): string {
  const d = new Date(`${day}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() - ((d.getUTCDay() + 6) % 7));
  return d.toISOString().slice(0, 10);
}

// weeks lists the Mondays that start the weeks from since to until.
export function weeks(since: string, until: string): string[] {
  return [...new Set(days(since, until).map(weekStart))];
}

// The kinds of token, in the order the dashboard stacks them: what the
// model was sent fresh, what it wrote, what went into the prompt cache and
// what was read from it.
export const TOKEN_TYPES = [
  { key: "input", label: "recentUsage.input" },
  { key: "output", label: "recentUsage.output" },
  { key: "cacheWrite", label: "recentUsage.cacheWrite" },
  { key: "cacheRead", label: "recentUsage.cacheRead" },
] as const satisfies readonly { key: string; label: PlainKey }[];

export type TokenType = (typeof TOKEN_TYPES)[number]["key"];

// typeValue is one kind of token's count or cost in a row.
export function typeValue(r: Row, t: TokenType, m: Metric): number {
  if (m === "cost") {
    if (!r.priced) return 0;
    return { input: r.costInput, output: r.costOutput, cacheWrite: r.costCacheWrite, cacheRead: r.costCacheRead }[t] ?? 0;
  }
  return { input: r.inputTokens, output: r.outputTokens, cacheWrite: r.cacheWriteTokens, cacheRead: r.cacheReadTokens }[t];
}

export interface TypeGroup {
  key: TokenType;
  label: string;
  tokens: number;
  cost: number;
}

// byType adds up each kind of token, in TOKEN_TYPES order, named in the language in force.
export function byType(rows: Row[]): TypeGroup[] {
  return TOKEN_TYPES.map((type) => ({
    key: type.key,
    label: t(type.label),
    tokens: rows.reduce((n, r) => n + typeValue(r, type.key, "tokens"), 0),
    cost: rows.reduce((n, r) => n + typeValue(r, type.key, "cost"), 0),
  }));
}

// dailyByType returns each day's value per kind of token.
export function dailyByType(rows: Row[], dayList: string[], metric: Metric): Day[] {
  const byDay = new Map(dayList.map((d) => [d, new Array<number>(TOKEN_TYPES.length).fill(0)]));
  for (const r of rows) {
    const vals = byDay.get(r.day);
    if (!vals) continue;
    TOKEN_TYPES.forEach((type, i) => (vals[i] += typeValue(r, type.key, metric)));
  }
  return dayList.map((d) => {
    const values = byDay.get(d)!;
    return { day: d, values, total: values.reduce((a, b) => a + b, 0) };
  });
}

// perCall is the average tokens of a model call, 0 without calls.
export function perCall(s: Sums): number {
  return s.calls ? tokensOf(s) / s.calls : 0;
}

// withoutCacheReads drops cache reads from rows, tokens and cost, for a
// view of what was sent fresh and generated: cache reads are cheap and can
// dwarf every other kind.
export function withoutCacheReads<R extends Row>(rows: R[]): R[] {
  return rows.map((r) => ({ ...r, cacheReadTokens: 0, cost: r.cost - (r.costCacheRead ?? 0), costCacheRead: 0 }));
}
