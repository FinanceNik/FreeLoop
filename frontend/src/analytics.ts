// The usage dashboard's numbers (PRV-008): totals, groups and the daily
// series, from the engine's rows of one day, session and model. Pure, so
// analytics.test.ts covers it.

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

export const OTHER = "Other";

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

// compact writes a count the short way: 950, 12.9K, 4.2M, 1.1B.
export function compact(n: number): string {
  const a = Math.abs(n);
  const unit = (d: number, s: string) => {
    const v = n / d;
    return `${Math.abs(v) >= 100 ? Math.round(v) : Number(v.toFixed(1))}${s}`;
  };
  if (a >= 1e9) return unit(1e9, "B");
  if (a >= 1e6) return unit(1e6, "M");
  if (a >= 1e3) return unit(1e3, "K");
  return String(Math.round(n));
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
