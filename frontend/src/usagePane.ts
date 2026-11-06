import { change, days, tokensOf, totals, type Row } from "./analytics.ts";

// The usage pane's views (GUI-038): one at a time, cycled with the arrows in its header.
export const PANE_VIEWS = [
  { id: "recent", label: "Recent hours" },
  { id: "year", label: "Last 12 months" },
  { id: "tokens", label: "Tokens" },
  { id: "models", label: "By model" },
] as const;
export type PaneView = (typeof PANE_VIEWS)[number]["id"];

export const PANE_RANGES = [7, 30] as const;

// cycleView steps through n views and comes round at either end.
export function cycleView(i: number, step: number, n: number): number {
  if (n <= 0) return 0;
  return (((i + step) % n) + n) % n;
}

// viewIndex reads a remembered view; anything else is the first.
export function viewIndex(stored: string | null | undefined): number {
  const i = PANE_VIEWS.findIndex((v) => v.id === stored);
  return i < 0 ? 0 : i;
}

// period splits rows into a range of days up to until and the same span before it, as
// the usage page does, with the change between them.
export function period<R extends Row>(rows: R[], range: number, until: string) {
  const span = days(addDays(until, -(range * 2 - 1)), until);
  const since = span[span.length - range];
  const beforeSince = span[0];
  const now = rows.filter((r) => r.day >= since && r.day <= until);
  const before = rows.filter((r) => r.day < since && r.day >= beforeSince);
  const t = totals(now);
  const b = totals(before);
  return { since, rows: now, now: t, delta: change(tokensOf(t), tokensOf(b)) };
}

function addDays(day: string, n: number): string {
  const d = new Date(`${day}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
}
