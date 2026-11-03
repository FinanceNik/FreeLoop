import { compact, type Day, type Metric } from "./analytics.ts";

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
