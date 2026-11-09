import { Events } from "@wailsio/runtime";
import { useEffect, useMemo, useRef, useState } from "react";
import { EngineService, EVENT_NAME } from "./api.ts";
import { formatCost } from "./cost.ts";
import {
  bucketsOf,
  filterRecent,
  formatTokens,
  groupedChart,
  NO_RECENT_FILTERS,
  recentChart,
  recentKeys,
  recentOptions,
  recentSeries,
  recentTotals,
  stepLabel,
  timeLabel,
  type RecentDimension,
  type RecentGroup,
  type RecentRow,
} from "./recentUsage.ts";
import UsageChart from "./UsageChart.tsx";
import { CloseIcon, FilterIcon } from "./icons.tsx";
import { useDismiss } from "./useDismiss.ts";
import { Tile } from "./UsageInsights.tsx";
import { locale } from "./i18n.ts";

// LiveUsage is the last 12 or 24 hours of usage (GUI-023), on the usage page and
// in a pane of its own beside sessions. It follows model calls as they finish, and
// the clock when none do, so an idle stretch still moves along.

const RANGES = [
  { hours: 12, label: "12 hours" },
  { hours: 24, label: "24 hours" },
];

// How long a burst of model calls waits before asking again.
const SETTLE_MS = 1500;
const REFRESH_MS = 60_000;

type Named = { id: string; name: string };

const GROUPS: { id: RecentGroup; label: string }[] = [
  { id: "type", label: "Token type" },
  { id: "model", label: "Model" },
  { id: "agent", label: "Provider or agent" },
  { id: "project", label: "Project" },
];

const DIMENSIONS: { id: RecentDimension; plural: string }[] = [
  { id: "project", plural: "All projects" },
  { id: "agent", plural: "All providers and agents" },
  { id: "model", plural: "All models" },
];

export default function LiveUsage({
  projects,
  providers,
  harnesses,
  compact = false,
  onError,
}: {
  projects: readonly Named[];
  providers: readonly Named[];
  harnesses: readonly Named[];
  // In a pane: one line of totals, and the chart takes the height that is left.
  compact?: boolean;
  onError: (err: unknown) => void;
}) {
  const [hours, setHours] = useState(12);
  const [metric, setMetric] = useState<"tokens" | "cost">("tokens");
  const [cacheReads, setCacheReads] = useState(false);
  const [rows, setRows] = useState<RecentRow[] | null>(null);
  const [starts, setStarts] = useState<number[]>([]);
  const [group, setGroup] = useState<RecentGroup>("type");
  const [filters, setFilters] = useState(NO_RECENT_FILTERS);
  const [step, setStep] = useState(15);
  const latest = useRef(0);

  useEffect(() => {
    let settle: ReturnType<typeof setTimeout> | undefined;
    const load = () => {
      const ask = ++latest.current;
      EngineService.RecentUsage(hours)
        .then((r) => {
          if (ask !== latest.current) return;
          setRows((r.rows ?? []) as RecentRow[]);
          setStarts((r.buckets ?? []).map((b) => b.start));
          setStep(r.stepMinutes);
        })
        .catch(onError);
    };
    load();
    const every = setInterval(load, REFRESH_MS);
    const off = Events.On(EVENT_NAME, ({ data: ev }) => {
      if (ev?.type !== "usage_update") return;
      clearTimeout(settle);
      settle = setTimeout(load, SETTLE_MS);
    });
    return () => {
      clearInterval(every);
      clearTimeout(settle);
      off();
    };
  }, [hours]);

  const keys = useMemo(() => recentKeys(projects, providers, harnesses), [projects, providers, harnesses]);
  const all = rows ?? [];
  const chosen = filterRecent(all, filters, keys);
  const list = bucketsOf(starts, chosen);
  const totals = recentTotals(list, cacheReads);
  const byGroup = group === "type" ? null : groupedChart(starts, all, chosen, keys[group], metric, cacheReads);
  const series = byGroup?.series ?? recentSeries(metric, cacheReads);
  const data = byGroup?.data ?? recentChart(list, metric, cacheReads);
  const filtered = Object.values(filters).some(Boolean);
  const format = metric === "cost" ? formatCost : formatTokens;
  const priced = totals.calls > totals.unpriced;
  const measureName = metric === "cost" ? "Estimated cost" : "Tokens";
  const title = `${measureName} per ${step} minutes, last ${hours} hours`;
  // In a pane the toggles above already say what and how long.
  const heading = compact ? `Per ${step} minutes` : title;
  const menu = useRef<HTMLDetailsElement>(null);
  useDismiss(menu);

  const active = DIMENSIONS.filter((d) => filters[d.id]);
  // What the filter button holds that is not as it starts.
  const settings = active.length + (cacheReads ? 1 : 0) + (group !== "type" ? 1 : 0);
  const range = (short: boolean) => (
    <div className="segmented" role="radiogroup" aria-label="Range">
      {RANGES.map((r) => (
        <button key={r.hours} role="radio" aria-checked={hours === r.hours} aria-label={r.label} className={hours === r.hours ? "on" : ""} onClick={() => setHours(r.hours)}>
          {short ? `${r.hours} h` : r.label}
        </button>
      ))}
    </div>
  );
  const measure = (
    <div className="segmented" role="radiogroup" aria-label="Measure">
      {(["tokens", "cost"] as const).map((m) => (
        <button key={m} role="radio" aria-checked={metric === m} className={metric === m ? "on" : ""} onClick={() => setMetric(m)}>
          {m === "tokens" ? "Tokens" : "Cost"}
        </button>
      ))}
    </div>
  );
  const cacheToggle = (
    <label className="row">
      <input type="checkbox" checked={cacheReads} onChange={(e) => setCacheReads(e.target.checked)} /> Cache reads
    </label>
  );
  const groupSelect = (
    <label className="row">
      Group by
      <select value={group} onChange={(e) => setGroup(e.target.value as RecentGroup)}>
        {GROUPS.map((g) => (
          <option key={g.id} value={g.id}>
            {g.label}
          </option>
        ))}
      </select>
    </label>
  );
  const filterSelects = DIMENSIONS.map((d) => (
    <select
      key={d.id}
      aria-label={`Filter: ${d.plural}`}
      className={filters[d.id] ? "usage-filter-on" : ""}
      value={filters[d.id]}
      onChange={(e) => setFilters({ ...filters, [d.id]: e.target.value })}
    >
      <option value="">{d.plural}</option>
      {recentOptions(all, keys[d.id], filters[d.id]).map((name) => (
        <option key={name} value={name}>
          {name}
        </option>
      ))}
    </select>
  ));
  const grouping = (
    <>
      {groupSelect}
      {filterSelects}
      {cacheToggle}
      {filtered && (
        <button className="link" onClick={() => setFilters(NO_RECENT_FILTERS)}>
          Clear filters
        </button>
      )}
    </>
  );

  return (
    <section className={compact ? "live-usage live-usage-compact" : "live-usage"} aria-label="Recent usage">
      {compact ? (
        <>
          {/* In a pane: the range and the measure, and the rest behind one button (GUI-042). */}
          <div className="live-usage-controls live-usage-bar">
            {range(true)}
            {measure}
            <details className="usage-filter-menu" ref={menu}>
              <summary className="icon-button" aria-label={settings ? `Filters and grouping, ${settings} set` : "Filters and grouping"} title="Filters and grouping">
                <FilterIcon />
                {settings > 0 && <span className="filter-count">{settings}</span>}
              </summary>
              <div className="usage-filter-panel">{grouping}</div>
            </details>
          </div>
          {active.length > 0 && (
            <div className="usage-chips" aria-label="Filters in use">
              {active.map((d) => (
                <button key={d.id} className="chip" aria-label={`Remove the filter ${filters[d.id]}`} title="Remove this filter" onClick={() => setFilters({ ...filters, [d.id]: "" })}>
                  {filters[d.id]}
                  <CloseIcon />
                </button>
              ))}
            </div>
          )}
        </>
      ) : (
        <>
          <div className="live-usage-controls">
            {range(false)}
            {measure}
            {cacheToggle}
          </div>
          <div className="live-usage-controls" role="group" aria-label="Filters and grouping">
            {groupSelect}
            {filterSelects}
            {filtered && (
              <button className="link" onClick={() => setFilters(NO_RECENT_FILTERS)}>
                Clear filters
              </button>
            )}
          </div>
        </>
      )}
      {compact ? (
        <p className="live-usage-strip" title={`${formatTokens(totals.output)} output · ${totals.calls.toLocaleString(locale())} model calls${priced && totals.unpriced > 0 ? ` · ${totals.unpriced} without a price` : ""}`}>
          <strong>{formatTokens(totals.tokens)}</strong> tokens · <strong>{priced ? formatCost(totals.cost) : "No price"}</strong>
          {priced && totals.unpriced > 0 && "+"}
        </p>
      ) : (
        <div className="live-usage-tiles">
          <Tile label="Tokens" value={formatTokens(totals.tokens)} />
          <Tile label="Output" value={formatTokens(totals.output)} />
          <Tile label="Model calls" value={totals.calls.toLocaleString(locale())} />
          <Tile label="Estimated cost" value={priced ? formatCost(totals.cost) : "No price"} note={priced && totals.unpriced ? `${totals.unpriced} calls have no price` : undefined} />
        </div>
      )}
      <div className="usage-card">
        <div className="usage-card-head">
          <h3>{heading}</h3>
          {series.length > 1 && (
            <ul className="usage-legend" aria-label="Legend">
              {series.map((s) => (
                <li key={s.key}>
                  <span className="usage-swatch" style={{ background: s.color }} />
                  {s.label}
                </li>
              ))}
            </ul>
          )}
        </div>
        {rows === null ? (
          <p className="muted">Counting…</p>
        ) : (
          <UsageChart fill={compact} data={data} series={series} format={format} bucketLabel={(d) => stepLabel(d, step)} axisLabel={timeLabel} label={title} />
        )}
        {rows !== null && totals.calls === 0 && <p className="muted">{filtered ? "Nothing matches these filters" : "No model calls"} in the last {hours} hours.</p>}
      </div>
    </section>
  );
}
