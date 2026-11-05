import { compact, OTHER, type Day, type Metric } from "./analytics.ts";
import { sourceName } from "./source.ts";

// Recent usage (GUI-023) as the usage chart takes it. The engine sends every
// step, empty ones too, so the time axis stays even.

export interface RecentBucket {
  start: number;
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  calls: number;
  cost: number;
  unpriced: number;
}

// The token types in the order and colours of the usage page.
export function recentSeries(metric: Metric, cacheReads: boolean) {
  if (metric === "cost") return [{ key: "cost", label: "Estimated cost", color: "var(--series-1)" }];
  const all = [
    { key: "input", label: "Input", color: "var(--series-1)" },
    { key: "output", label: "Output", color: "var(--series-2)" },
    { key: "cacheWrite", label: "Cache write", color: "var(--series-3)" },
    { key: "cacheRead", label: "Cache read", color: "var(--series-4)" },
  ];
  return cacheReads ? all : all.filter((s) => s.key !== "cacheRead");
}

// recentChart is the chart's columns; a column's day is the step's start.
export function recentChart(buckets: RecentBucket[], metric: Metric, cacheReads: boolean): Day[] {
  return buckets.map((b) => {
    const values = metric === "cost" ? [b.cost] : [b.input, b.output, b.cacheWrite, ...(cacheReads ? [b.cacheRead] : [])];
    return { day: String(b.start), values, total: values.reduce((a, v) => a + v, 0) };
  });
}

export interface RecentTotals {
  tokens: number;
  output: number;
  calls: number;
  cost: number;
  unpriced: number;
}

export function recentTotals(buckets: RecentBucket[], cacheReads: boolean): RecentTotals {
  const t: RecentTotals = { tokens: 0, output: 0, calls: 0, cost: 0, unpriced: 0 };
  for (const b of buckets) {
    t.tokens += b.input + b.output + b.cacheWrite + (cacheReads ? b.cacheRead : 0);
    t.output += b.output;
    t.calls += b.calls;
    t.cost += b.cost;
    t.unpriced += b.unpriced;
  }
  return t;
}

// timeLabel is a step's start on the axis.
export function timeLabel(start: string | number): string {
  return new Date(Number(start)).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

// stepLabel is a step in the chart's readout: from when to when.
export function stepLabel(start: string | number, stepMinutes: number): string {
  return `${timeLabel(start)} to ${timeLabel(Number(start) + stepMinutes * 60_000)}`;
}

export function formatTokens(v: number): string {
  return compact(v);
}

// One step's use by one project, agent and model, for the pane's filters and groups.
export interface RecentRow extends RecentBucket {
  projectId: string;
  projectRoot: string;
  harnessId: string;
  providerId: string;
  model: string;
}

export type RecentDimension = "project" | "agent" | "model";
export type RecentGroup = "type" | RecentDimension;
export type RecentFilters = Record<RecentDimension, string>;
export type RecentKeys = Record<RecentDimension, (r: RecentRow) => string>;

export const NO_RECENT_FILTERS: RecentFilters = { project: "", agent: "", model: "" };

type Named = { id: string; name: string };

// recentKeys names what a row belongs to, the way the usage page does.
export function recentKeys(projects: readonly Named[], providers: readonly Named[], harnesses: readonly Named[]): RecentKeys {
  const agent = (r: RecentRow) => sourceName(r, providers, harnesses);
  return {
    project: (r) => projects.find((p) => p.id === r.projectId)?.name ?? (r.projectRoot.split("/").pop() || "No project"),
    agent,
    model: (r) => r.model || `${agent(r)} (model not reported)`,
  };
}

export function filterRecent(rows: readonly RecentRow[], filters: RecentFilters, keys: RecentKeys): RecentRow[] {
  return rows.filter((r) => (Object.keys(filters) as RecentDimension[]).every((d) => !filters[d] || keys[d](r) === filters[d]));
}

// recentOptions lists what a filter can choose, by name. A chosen one that has left
// the range stays, so the filter does not read as if it were off.
export function recentOptions(rows: readonly RecentRow[], key: (r: RecentRow) => string, chosen = ""): string[] {
  const names = new Set(rows.map(key));
  if (chosen) names.add(chosen);
  return [...names].sort((a, b) => a.localeCompare(b));
}

// bucketsOf puts rows back into every step, empty ones too.
export function bucketsOf(starts: readonly number[], rows: readonly RecentRow[]): RecentBucket[] {
  const out = new Map(starts.map((start): [number, RecentBucket] => [start, { start, input: 0, output: 0, cacheRead: 0, cacheWrite: 0, calls: 0, cost: 0, unpriced: 0 }]));
  for (const r of rows) {
    const b = out.get(r.start);
    if (!b) continue;
    b.input += r.input;
    b.output += r.output;
    b.cacheRead += r.cacheRead;
    b.cacheWrite += r.cacheWrite;
    b.calls += r.calls;
    b.cost += r.cost;
    b.unpriced += r.unpriced;
  }
  return [...out.values()];
}

const rowValue = (r: RecentRow, metric: Metric, cacheReads: boolean) =>
  metric === "cost" ? r.cost : r.input + r.output + r.cacheWrite + (cacheReads ? r.cacheRead : 0);

const GROUP_COLORS = 5; // a sixth group and beyond fold into Other

// groupedChart draws one colour per group. The largest are chosen from all the rows
// of the range, before any filter, so that a filter does not repaint what it leaves;
// a group a filter emptied leaves the legend.
export function groupedChart(
  starts: readonly number[],
  all: readonly RecentRow[],
  rows: readonly RecentRow[],
  key: (r: RecentRow) => string,
  metric: Metric,
  cacheReads: boolean,
): { series: { key: string; label: string; color: string }[]; data: Day[] } {
  const totals = new Map<string, number>();
  for (const r of all) totals.set(key(r), (totals.get(key(r)) ?? 0) + rowValue(r, metric, cacheReads));
  const ranked = [...totals.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([k]) => k);
  const top = ranked.slice(0, GROUP_COLORS);
  const other = ranked.length - top.length;
  const series = [
    ...top.map((k, i) => ({ key: k, label: k, color: `var(--series-${i + 1})` })),
    ...(other > 0 ? [{ key: OTHER, label: `${OTHER} (${other})`, color: "var(--series-other)" }] : []),
  ];
  const at = new Map(series.map((s, i) => [s.key, i]));
  const columns = new Map(starts.map((start) => [start, series.map(() => 0)]));
  const used = series.map(() => false);
  for (const r of rows) {
    const values = columns.get(r.start);
    const i = at.get(key(r)) ?? at.get(OTHER);
    if (!values || i === undefined) continue;
    const v = rowValue(r, metric, cacheReads);
    values[i] += v;
    if (v > 0) used[i] = true;
  }
  const kept = series.map((_, i) => i).filter((i) => used[i]);
  return {
    series: kept.map((i) => series[i]),
    data: starts.map((start) => {
      const values = kept.map((i) => columns.get(start)![i]);
      return { day: String(start), values, total: values.reduce((a, v) => a + v, 0) };
    }),
  };
}

