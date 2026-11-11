// What a commit costs, week by week (GUI-051): the statistics the Usage page shows
// from the engine's WorkCost report. Pure, so node --test covers it.

export interface CommitCost {
  week: string;
  cost: number;
  unpriced: boolean;
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  added: number;
  deleted: number;
}

export interface Spend {
  week: string;
  committed: boolean;
  cost: number;
  tokens: number;
}

export interface LimitUse {
  week: string;
  source: string;
  window: string;
  label: string;
  spend: number;
  percent: number;
  runs: number;
}

export interface WeekStats {
  week: string;
  commits: number;
  // Cost per commit: the median, and the middle half around it.
  median: number;
  p25: number;
  p75: number;
  priced: boolean;
  medianTokens: number;
  medianLines: number;
  // All the week's spend in the projects shown, over its commits.
  allIn: number;
  // The share of the week's spend that no commit of its session followed.
  uncommitted: number;
  spend: number;
}

// quantile reads q (0 to 1) off sorted values, between neighbours.
export function quantile(sorted: readonly number[], q: number): number {
  if (sorted.length === 0) return 0;
  const at = (sorted.length - 1) * q;
  const lo = Math.floor(at);
  const hi = Math.ceil(at);
  return sorted[lo] + (sorted[hi] - sorted[lo]) * (at - lo);
}

const sortedOf = (xs: number[]) => [...xs].sort((a, b) => a - b);

// tokensOf counts a commit's tokens, cache reads only when they are counted.
export const tokensOf = (c: CommitCost, cacheReads: boolean) => c.input + c.output + c.cacheWrite + (cacheReads ? c.cacheRead : 0);

// weekStats sums each of weeks up from the commits and spend shown.
export function weekStats(weeks: readonly string[], commits: readonly CommitCost[], spend: readonly Spend[], cacheReads: boolean): WeekStats[] {
  return weeks.map((week) => {
    const mine = commits.filter((c) => c.week === week);
    const costs = sortedOf(mine.map((c) => c.cost));
    const spent = spend.filter((s) => s.week === week);
    const total = spent.reduce((n, s) => n + s.cost, 0);
    const loose = spent.filter((s) => !s.committed).reduce((n, s) => n + s.cost, 0);
    const lines = mine.filter((c) => c.added >= 0).map((c) => c.added + c.deleted);
    return {
      week,
      commits: mine.length,
      median: quantile(costs, 0.5),
      p25: quantile(costs, 0.25),
      p75: quantile(costs, 0.75),
      priced: mine.some((c) => !c.unpriced),
      medianTokens: quantile(sortedOf(mine.map((c) => tokensOf(c, cacheReads))), 0.5),
      medianLines: quantile(sortedOf(lines), 0.5),
      allIn: mine.length ? total / mine.length : 0,
      uncommitted: total > 0 ? loose / total : 0,
      spend: total,
    };
  });
}

// What a percent of one limit bought in one week: spend over percent, across the
// runs counted.
export interface LimitWeek {
  week: string;
  perPercent: number;
  runs: number;
}

export interface LimitSeries {
  key: string;
  source: string;
  label: string;
  weeks: LimitWeek[];
}

export function limitSeries(uses: readonly LimitUse[]): LimitSeries[] {
  const out = new Map<string, LimitSeries>();
  for (const u of uses) {
    const key = `${u.source}\u0000${u.window}`;
    const s = out.get(key) ?? { key, source: u.source, label: u.label, weeks: [] };
    out.set(key, s);
    s.weeks.push({ week: u.week, perPercent: u.percent > 0 ? u.spend / u.percent : 0, runs: u.runs });
  }
  return [...out.values()];
}

// The change the last two weeks show against the eight before, when both sides
// have enough to say it: the median of each side's commits, and how many.
export interface Change {
  ratio: number;
  recent: number;
  earlier: number;
}

export const RECENT_WEEKS = 2;
export const EARLIER_WEEKS = 8;
export const MIN_COMMITS = 10;
export const MIN_RUNS = 3;

// sides splits weeks, oldest first, into the last two and the eight before.
function sides(weeks: readonly string[]): [Set<string>, Set<string>] {
  const recent = weeks.slice(-RECENT_WEEKS);
  const earlier = weeks.slice(-(RECENT_WEEKS + EARLIER_WEEKS), -RECENT_WEEKS);
  return [new Set(recent), new Set(earlier)];
}

export function commitChange<C extends CommitCost>(weeks: readonly string[], commits: readonly C[], value: (c: C) => number): Change | null {
  const [recent, earlier] = sides(weeks);
  const a = sortedOf(commits.filter((c) => recent.has(c.week)).map(value));
  const b = sortedOf(commits.filter((c) => earlier.has(c.week)).map(value));
  if (a.length < MIN_COMMITS || b.length < MIN_COMMITS) return null;
  const before = quantile(b, 0.5);
  if (before <= 0) return null;
  return { ratio: quantile(a, 0.5) / before - 1, recent: a.length, earlier: b.length };
}

// limitChange compares what a percent bought, as a ratio of sums on each side.
export function limitChange(weeks: readonly string[], uses: readonly LimitUse[]): Change | null {
  const [recent, earlier] = sides(weeks);
  const side = (set: Set<string>) => {
    const mine = uses.filter((u) => set.has(u.week));
    const percent = mine.reduce((n, u) => n + u.percent, 0);
    return { value: percent > 0 ? mine.reduce((n, u) => n + u.spend, 0) / percent : 0, runs: mine.reduce((n, u) => n + u.runs, 0) };
  };
  const a = side(recent);
  const b = side(earlier);
  if (a.runs < MIN_RUNS || b.runs < MIN_RUNS || b.value <= 0) return null;
  return { ratio: a.value / b.value - 1, recent: a.runs, earlier: b.runs };
}
