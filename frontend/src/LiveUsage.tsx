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
import { locale, t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

// LiveUsage is the last 12 or 24 hours of usage (GUI-023), on the usage page and
// in a pane of its own beside sessions. It follows model calls as they finish, and
// the clock when none do, so an idle stretch still moves along.

const RANGES = [12, 24];

// How long a burst of model calls waits before asking again.
const SETTLE_MS = 1500;
const REFRESH_MS = 60_000;

type Named = { id: string; name: string };

const GROUPS: { id: RecentGroup; label: PlainKey }[] = [
  { id: "type", label: "liveUsage.group.type" },
  { id: "model", label: "usage.model" },
  { id: "agent", label: "liveUsage.group.agent" },
  { id: "project", label: "sidebar.groupBy.project" },
];

const DIMENSIONS: { id: RecentDimension; plural: PlainKey }[] = [
  { id: "project", plural: "liveUsage.all.project" },
  { id: "agent", plural: "liveUsage.all.agent" },
  { id: "model", plural: "liveUsage.all.model" },
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
  const title = t(metric === "cost" ? "liveUsage.title.cost" : "liveUsage.title.tokens", { step, hours });
  // In a pane the toggles above already say what and how long.
  const heading = compact ? t("liveUsage.perStep", { step }) : title;
  // The pane's totals in full, as facts set side by side.
  const stripTitle = [
    t("liveUsage.strip.output", { tokens: formatTokens(totals.output) }),
    t("liveUsage.strip.calls", { count: totals.calls }),
    ...(priced && totals.unpriced > 0 ? [t("liveUsage.strip.unpriced", { count: totals.unpriced })] : []),
  ].join(" · ");
  const menu = useRef<HTMLDetailsElement>(null);
  useDismiss(menu);

  const active = DIMENSIONS.filter((d) => filters[d.id]);
  // What the filter button holds that is not as it starts.
  const settings = active.length + (cacheReads ? 1 : 0) + (group !== "type" ? 1 : 0);
  const range = (short: boolean) => (
    <div className="segmented" role="radiogroup" aria-label={t("usagePane.range")}>
      {RANGES.map((h) => (
        <button key={h} role="radio" aria-checked={hours === h} aria-label={t("liveUsage.hours", { count: h })} className={hours === h ? "on" : ""} onClick={() => setHours(h)}>
          {short ? t("liveUsage.hoursShort", { hours: h }) : t("liveUsage.hours", { count: h })}
        </button>
      ))}
    </div>
  );
  const measure = (
    <div className="segmented" role="radiogroup" aria-label={t("liveUsage.measure")}>
      {(["tokens", "cost"] as const).map((m) => (
        <button key={m} role="radio" aria-checked={metric === m} className={metric === m ? "on" : ""} onClick={() => setMetric(m)}>
          {m === "tokens" ? t("liveUsage.tokens") : t("usage.cost")}
        </button>
      ))}
    </div>
  );
  const cacheToggle = (
    <label className="row">
      <input type="checkbox" checked={cacheReads} onChange={(e) => setCacheReads(e.target.checked)} /> {t("liveUsage.cacheReads")}
    </label>
  );
  const groupSelect = (
    <label className="row">
      {t("sidebar.groupBy")}
      <select value={group} onChange={(e) => setGroup(e.target.value as RecentGroup)}>
        {GROUPS.map((g) => (
          <option key={g.id} value={g.id}>
            {t(g.label)}
          </option>
        ))}
      </select>
    </label>
  );
  const filterSelects = DIMENSIONS.map((d) => (
    <select
      key={d.id}
      aria-label={t("liveUsage.filter", { name: t(d.plural) })}
      className={filters[d.id] ? "usage-filter-on" : ""}
      value={filters[d.id]}
      onChange={(e) => setFilters({ ...filters, [d.id]: e.target.value })}
    >
      <option value="">{t(d.plural)}</option>
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
          {t("liveUsage.clearFilters")}
        </button>
      )}
    </>
  );

  return (
    <section className={compact ? "live-usage live-usage-compact" : "live-usage"} aria-label={t("liveUsage.label")}>
      {compact ? (
        <>
          {/* In a pane: the range and the measure, and the rest behind one button (GUI-042). */}
          <div className="live-usage-controls live-usage-bar">
            {range(true)}
            {measure}
            <details className="usage-filter-menu" ref={menu}>
              <summary className="icon-button" aria-label={settings ? t("liveUsage.filtersSet", { count: settings }) : t("liveUsage.filters")} title={t("liveUsage.filters")}>
                <FilterIcon />
                {settings > 0 && <span className="filter-count">{settings}</span>}
              </summary>
              <div className="usage-filter-panel">{grouping}</div>
            </details>
          </div>
          {active.length > 0 && (
            <div className="usage-chips" aria-label={t("liveUsage.filtersInUse")}>
              {active.map((d) => (
                <button key={d.id} className="chip" aria-label={t("sidebar.filter.remove", { label: filters[d.id] })} title={t("liveUsage.removeThisFilter")} onClick={() => setFilters({ ...filters, [d.id]: "" })}>
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
          <div className="live-usage-controls" role="group" aria-label={t("liveUsage.filters")}>
            {groupSelect}
            {filterSelects}
            {filtered && (
              <button className="link" onClick={() => setFilters(NO_RECENT_FILTERS)}>
                {t("liveUsage.clearFilters")}
              </button>
            )}
          </div>
        </>
      )}
      {compact ? (
        <p className="live-usage-strip" title={stripTitle}>
          {tNodes("liveUsage.strip", { tokens: <strong>{formatTokens(totals.tokens)}</strong>, cost: <strong>{priced ? formatCost(totals.cost) : t("liveUsage.noPrice")}</strong> })}
          {priced && totals.unpriced > 0 && "+"}
        </p>
      ) : (
        <div className="live-usage-tiles">
          <Tile label={t("liveUsage.tokens")} value={formatTokens(totals.tokens)} />
          <Tile label={t("recentUsage.output")} value={formatTokens(totals.output)} />
          <Tile label={t("liveUsage.calls")} value={totals.calls.toLocaleString(locale())} />
          <Tile label={t("recentUsage.cost")} value={priced ? formatCost(totals.cost) : t("liveUsage.noPrice")} note={priced && totals.unpriced ? t("liveUsage.unpricedCalls", { count: totals.unpriced }) : undefined} />
        </div>
      )}
      <div className="usage-card">
        <div className="usage-card-head">
          <h3>{heading}</h3>
          {series.length > 1 && (
            <ul className="usage-legend" aria-label={t("usagePane.legend")}>
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
          <p className="muted">{t("liveUsage.counting")}</p>
        ) : (
          <UsageChart fill={compact} data={data} series={series} format={format} bucketLabel={(d) => stepLabel(d, step)} axisLabel={timeLabel} label={title} />
        )}
        {rows !== null && totals.calls === 0 && <p className="muted">{t(filtered ? "liveUsage.noneFiltered" : "liveUsage.none", { hours })}</p>}
      </div>
    </section>
  );
}
