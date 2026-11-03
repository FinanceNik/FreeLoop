// An account's limits as agents reported them (GUI-028). Pure, so limits.test.ts
// covers it. A report comes back with a model call, so what is shown may be old.

export type LimitPeriodRow = {
  source: string;
  window: string;
  label: string;
  resetsAt: string | Date;
  used: number;
  peak: number;
  lastReport: string | Date;
  warned: number;
  refused: number;
};

export type CurrentLimit = {
  key: string;
  source: string;
  label: string;
  // The share used, or null when the run reported has reset and nothing has since.
  used: number | null;
  resetsAt: Date;
  lastReport: Date;
  state: "ok" | "warning" | "refused" | "reset";
};

const time = (t: string | Date) => new Date(t).getTime() || 0;
const keyOf = (p: LimitPeriodRow) => `${p.source} ${p.window}`;

// A share this high is called close, whatever the agent has said so far: it warns
// late, and some reports never do.
const CLOSE_PERCENT = 80;

// currentLimits is each limit's newest run, the soonest to reset first. Parallel
// sessions report in any order, so a run in progress shows its peak: use only rises
// until the reset.
export function currentLimits(periods: readonly LimitPeriodRow[], now: Date = new Date()): CurrentLimit[] {
  const newest = new Map<string, LimitPeriodRow>();
  for (const p of periods) {
    const had = newest.get(keyOf(p));
    if (!had || time(p.resetsAt) > time(had.resetsAt)) newest.set(keyOf(p), p);
  }
  const out = [...newest.values()].map((p): CurrentLimit => {
    const over = time(p.resetsAt) <= now.getTime();
    const state = over ? "reset" : p.refused > 0 ? "refused" : p.warned > 0 || p.peak >= CLOSE_PERCENT ? "warning" : "ok";
    return {
      key: keyOf(p),
      source: p.source,
      label: p.label,
      used: over ? null : p.peak,
      resetsAt: new Date(p.resetsAt),
      lastReport: new Date(p.lastReport),
      state,
    };
  });
  return out.sort((a, b) => a.resetsAt.getTime() - b.resetsAt.getTime() || a.key.localeCompare(b.key));
}

export type LimitSummary = { key: string; source: string; label: string; runs: number; averagePeak: number; highest: number; closeRuns: number; refusedRuns: number };

// limitSummaries says how each limit has gone over the runs that have finished: the
// one in progress would only pull the averages down.
export function limitSummaries(periods: readonly LimitPeriodRow[], now: Date = new Date()): LimitSummary[] {
  const groups = new Map<string, LimitPeriodRow[]>();
  for (const p of periods) {
    if (time(p.resetsAt) > now.getTime()) continue;
    groups.set(keyOf(p), [...(groups.get(keyOf(p)) ?? []), p]);
  }
  return [...groups.entries()]
    .map(([key, runs]) => ({
      key,
      source: runs[0].source,
      label: runs[0].label,
      runs: runs.length,
      averagePeak: runs.reduce((n, r) => n + r.peak, 0) / runs.length,
      highest: Math.max(...runs.map((r) => r.peak)),
      closeRuns: runs.filter((r) => r.warned > 0 || r.peak >= CLOSE_PERCENT).length,
      refusedRuns: runs.filter((r) => r.refused > 0).length,
    }))
    .sort((a, b) => a.key.localeCompare(b.key));
}

export function percentText(used: number): string {
  return used > 0 && used < 1 ? "under 1%" : `${Math.round(used)}%`;
}

// resetText says when a limit starts over, in words.
export function resetText(resetsAt: Date, now: Date = new Date()): string {
  const minutes = Math.round((resetsAt.getTime() - now.getTime()) / 60000);
  if (minutes <= 0) return "has reset";
  if (minutes < 60) return `resets in ${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `resets in ${hours} h ${minutes % 60} min`;
  const days = Math.floor(hours / 24);
  return `resets in ${days} ${days === 1 ? "day" : "days"} ${hours % 24} h`;
}

export const LIMIT_STATE_LABEL: Record<CurrentLimit["state"], string> = {
  ok: "",
  warning: "Close to the limit",
  refused: "Limit reached",
  reset: "Has reset, no report since",
};
