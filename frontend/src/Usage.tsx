import { useEffect, useState } from "react";
import UsageOverview from "./UsageOverview.tsx";
import TokenFigures from "./TokenFigures.tsx";
import { tabAfterKey } from "./a11y.ts";
import { heldStats, type ActivityRow as FoldedRow, type EffRow } from "./activity.ts";
import { heldNote, reportingNotes } from "./reporting.ts";
import Budgets from "./Budgets.tsx";
import { EngineService, type ActivityReport, type HarnessInfo, type ProjectInfo, type ProviderInfo, type UsageReport, type UsageRow } from "./api.ts";
import { byType, change, compact, daily, dailyByType, days, groupBy, OTHER, TOKEN_TYPES, topKeys, totals, value, weekStart, weeks, withoutCacheReads, type Metric, runKey } from "./analytics.ts";
import { formatCost } from "./cost.ts";
import LiveUsage from "./LiveUsage.tsx";
import UsageChart, { type Series } from "./UsageChart.tsx";
import { ActivityView, ApprovalsView, EfficiencyView } from "./UsageInsights.tsx";
import WorkCostView from "./WorkCost.tsx";
import { basename } from "./paths.ts";
import Limits from "./Limits.tsx";
import CostSummaryPanel from "./CostSummary.tsx";
import { costSummary } from "./costsummary.ts";
import { locale, numberFormat, t, type Key, type PlainKey } from "./i18n.ts";

// The usage dashboard (GUI-018, PRV-008): tokens and estimated cost over a
// range, by kind of token, model, agent or project, and the sessions that
// used the most. Costs
// come from the price table in Settings; tokens of unpriced models are
// counted but cost nothing here. The other tabs (GUI-021) show what the agents
// did, who decided what, and what a turn takes, under the same filters.

interface Props {
  // Shows live usage in a pane beside sessions (GUI-023).
  onShowInPane?: () => void;
  projects: ProjectInfo[];
  providers: ProviderInfo[];
  harnesses: HarnessInfo[];
  onOpenSession: (id: string) => void;
  onOpenPrices: () => void;
  onError: (err: unknown) => void;
}

const RANGES = [7, 30, 90, 365];

// A range in words: 30 days, 12 months.
const rangeLabel = (days: number) => (days === 365 ? t("usagePage.range.year") : t("paneRange.days", { count: days }));

// Tables of labels hold catalogue keys and translate where they render.
const GROUPS = [
  { id: "type", label: "usagePage.group.type", by: "usagePage.by.type", chart: "usagePage.chartLabel.type" },
  { id: "model", label: "usage.model", by: "usagePage.by.model", chart: "usagePage.chartLabel.model" },
  { id: "agent", label: "usagePage.group.agent", by: "usagePage.by.agent", chart: "usagePage.chartLabel.agent" },
  { id: "project", label: "sidebar.groupBy.project", by: "usagePage.by.project", chart: "usagePage.chartLabel.project" },
  { id: "run", label: "usagePage.group.run", by: "usagePage.by.run", chart: "usagePage.chartLabel.run" },
] as const satisfies readonly { id: string; label: PlainKey; by: PlainKey; chart: Key }[];
type GroupID = (typeof GROUPS)[number]["id"];
const groupOf = (id: GroupID) => GROUPS.find((g) => g.id === id)!;

const TABS = [
  { id: "tokens", label: "usagePage.tab.tokens" },
  { id: "activity", label: "usagePage.tab.activity" },
  { id: "approvals", label: "usagePage.tab.approvals" },
  { id: "efficiency", label: "usagePage.tab.efficiency" },
] as const satisfies readonly { id: string; label: PlainKey }[];
type TabID = (typeof TABS)[number]["id"];

const NO_FILTERS = { project: "", agent: "", model: "", tag: "" };

// The month so far is needed for the projection, whatever the range.
const MIN_FETCH_DAYS = 62;

const FILTER_LABELS = { project: "sidebar.groupBy.project", agent: "usagePage.group.agent", model: "usage.model", tag: "sidebar.filter.tag" } as const satisfies Record<string, PlainKey>;
const FILTER_ALL = { project: "usagePage.all.project", agent: "usagePage.all.agent", model: "usagePage.all.model", tag: "usagePage.all.tag" } as const satisfies Record<string, PlainKey>;

// A share of a whole, in the locale's style: 12%, 12 %.
const percent = (ratio: number) => numberFormat({ style: "percent", maximumFractionDigits: 0 }).format(ratio);

const SERIES = 5; // colours; a sixth group and beyond fold into Other

const month = (d: string) => new Date(`${d}T12:00:00`).toLocaleDateString(locale(), { month: "short", day: "numeric" });

export default function Usage({ onShowInPane, projects, providers, harnesses, onOpenSession, onOpenPrices, onError }: Props) {
  const [range, setRange] = useState(30);
  const [group, setGroup] = useState<GroupID>("type");
  const [metric, setMetric] = useState<Metric>("tokens");
  const [cacheReads, setCacheReads] = useState(true);
  const [report, setReport] = useState<UsageReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [exported, setExported] = useState("");
  const [sharing, setSharing] = useState(false);
  const [pricesAsOf, setPricesAsOf] = useState("");
  useEffect(() => {
    EngineService.BuiltinPrices()
      .then((p) => setPricesAsOf(p.asOf))
      .catch(onError);
  }, []);
  const [tab, setTab] = useState<TabID>("tokens");
  const [filters, setFilters] = useState(NO_FILTERS);
  // A filter holds the text of a group's name, which a change of language rewrites
  // (the unnamed provider, the deleted sessions), so a filter set before it would match
  // nothing after it.
  const [filterLocale, setFilterLocale] = useState(locale());
  if (filterLocale !== locale()) {
    setFilterLocale(locale());
    setFilters(NO_FILTERS);
  }
  const [activity, setActivity] = useState<ActivityReport | null>(null);
  const [reloads, setReloads] = useState(0);

  useEffect(() => {
    let live = true;
    setLoading(true);
    // Twice the range, for the comparison with the period before.
    EngineService.UsageReport(Math.max(range * 2, MIN_FETCH_DAYS))
      .then((r) => live && setReport(r))
      .catch(onError)
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [range]);

  // A long history is counted in steps: ask again until none is left.
  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout>;
    const load = () =>
      EngineService.ActivityReport(range)
        .then((r) => {
          if (!live) return;
          setActivity(r);
          if (r.pending > 0) timer = setTimeout(load, 200);
        })
        .catch(onError);
    load();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [range, reloads]);

  const agent = (r: Pick<UsageRow, "harnessId" | "providerId">) => {
    if (r.harnessId && r.harnessId !== "builtin") return harnesses.find((h) => h.id === r.harnessId)?.name ?? r.harnessId;
    return providers.find((p) => p.id === r.providerId)?.name ?? (r.providerId || t("usagePage.unknownProvider"));
  };
  const project = (r: Pick<UsageRow, "projectId" | "projectRoot">) => projects.find((p) => p.id === r.projectId)?.name ?? (r.projectRoot ? basename(r.projectRoot) : t("usagePage.deletedSessions"));
  const keyOf: Record<Exclude<GroupID, "type">, (r: UsageRow) => string> = {
    model: (r) => r.model || t("recentUsage.modelNotReported", { name: agent(r) }),
    agent,
    project,
    run: (r) => runKey(r, t("usagePage.noRun")),
  };

  // A row with no tokens holds only turns an agent reported nothing for (PRV-008):
  // nothing to count, said in words instead. An adjustment has tokens and no calls.
  const reported = (report?.rows ?? []) as (UsageRow & EffRow)[];
  const unfiltered = reported.filter((r) => r.inputTokens + r.outputTokens + r.cacheReadTokens + r.cacheWriteTokens > 0);
  const until = report?.until ?? new Date().toISOString().slice(0, 10);
  const span = report ? days(report.since, until) : [];
  const since = span[Math.max(0, span.length - range)] ?? until;
  const beforeSince = span[Math.max(0, span.length - range * 2)] ?? until;

  // The filters apply to every tab. Activity is per session, so the model
  // filter keeps the sessions that used that model.
  const shared = (r: Pick<UsageRow, "harnessId" | "providerId" | "projectId" | "projectRoot" | "tags">) =>
    (!filters.project || project(r) === filters.project) && (!filters.agent || agent(r) === filters.agent) && (!filters.tag || (r.tags ?? []).includes(filters.tag));
  const fetched = unfiltered.filter((r) => shared(r) && (!filters.model || keyOf.model(r) === filters.model));
  const modelSessions = new Set(fetched.map((r) => r.sessionId));
  const activityAll = activity?.rows ?? [];
  const activityRows = activityAll.filter((r) => shared(r) && (!filters.model || modelSessions.has(r.sessionId))) as FoldedRow[];
  const inRange = unfiltered.filter((r) => r.day >= since);
  const choices = (values: (string | undefined)[], current: string) => [...new Set([...values, current])].filter((v): v is string => !!v).sort((a, b) => a.localeCompare(b));
  const options = {
    project: choices([...inRange.map(project), ...activityAll.map(project)], filters.project),
    agent: choices([...inRange.map(agent), ...activityAll.map(agent)], filters.agent),
    model: choices(inRange.map(keyOf.model), filters.model),
    tag: choices([...inRange.flatMap((r) => r.tags ?? []), ...activityAll.flatMap((r) => r.tags ?? [])], filters.tag),
  };
  const filtering = Object.values(filters).some(Boolean);

  const all = cacheReads ? fetched : withoutCacheReads(fetched);
  const rows = all.filter((r) => r.day >= since);
  const before = all.filter((r) => r.day < since && r.day >= beforeSince);
  const reporting = reportingNotes(
    reported.filter((r) => r.day >= since && shared(r) && (!filters.model || keyOf.model(r) === filters.model || (r.unreported ?? 0) > 0)),
    agent,
  );
  const now = totals(rows);
  const prev = totals(before);
  const priced = rows.some((r) => r.priced);
  const m: Metric = metric === "cost" && !priced ? "tokens" : metric;
  const fmt = m === "cost" ? formatCost : compact;

  // By kind of token the series are fixed, in stacking order; otherwise the
  // largest groups get one each and the rest fold into Other.
  const byKind = group === "type";
  const key = keyOf[byKind ? "model" : group];
  const groups = groupBy(rows, key, m);
  const types = byType(rows);
  const top = byKind ? [] : topKeys(groups, SERIES);
  const series: Series[] = byKind
    ? TOKEN_TYPES.map((type, i) => ({ key: type.key, label: t(type.label), color: `var(--series-${i + 1})` })).filter((s) => cacheReads || s.key !== "cacheRead")
    : [
        ...top.map((k, i) => ({ key: k, label: k, color: `var(--series-${i + 1})` })),
        ...(groups.length > top.length ? [{ key: OTHER, label: t("recentUsage.other", { count: groups.length - top.length }), color: "var(--series-other)" }] : []),
      ];
  const weekly = range > 90;
  const chartTitle = t(m === "cost" ? (weekly ? "usagePage.chart.costWeek" : "usagePage.chart.costDay") : weekly ? "usagePage.chart.tokensWeek" : "usagePage.chart.tokensDay");
  const buckets = weekly ? weeks(since, until) : days(since, until);
  const bucketed = weekly ? rows.map((r) => ({ ...r, day: weekStart(r.day) })) : rows;
  const chart = byKind ? dailyByType(bucketed, buckets, m) : daily(bucketed, buckets, key, top, m);
  const colorOf = (k: string) => series.find((s) => s.key === k)?.color ?? "var(--series-other)";
  const sessions = groupBy(rows, (r) => r.sessionId, m).slice(0, 10);
  const sessionRow = (id: string) => rows.find((r) => r.sessionId === id)!;
  const delta = change(value(now, m), value(prev, m));
  const maxGroup = Math.max(0, ...groups.map((g) => value(g, m)));
  const tokensTotal = value(now, "tokens");

  const exportCSV = () =>
    EngineService.ExportUsage(range)
      .then((p) => {
        if (!p) return;
        setExported(p);
        setTimeout(() => setExported(""), 4000);
      })
      .catch(onError);

  return (
    <section className="page usage-page">
      <h2>{t("usagePage.title")}</h2>
      <UsageOverview cacheReads={cacheReads} onError={onError} />
      <div className="usage-limits">
        <h3>{t("usagePage.limits")}</h3>
        <Limits onError={onError} />
      </div>
      <div className="usage-recent">
        <div className="pane-usage-head">
          <h3>{t("usagePane.view.recent")}</h3>
          <span className="spacer" />
          {onShowInPane && <button onClick={onShowInPane}>{t("usagePage.showInPane")}</button>}
        </div>
        <LiveUsage projects={projects} providers={providers} harnesses={harnesses} onError={onError} />
      </div>
      <div
        className="panel-tabs usage-tabs"
        role="tablist"
        aria-label={t("usagePane.views")}
        onKeyDown={(e) => {
          const tabs = [...e.currentTarget.querySelectorAll<HTMLElement>('[role="tab"]')];
          const next = tabAfterKey(e.key, tabs.indexOf(e.target as HTMLElement), tabs.length);
          if (next < 0) return;
          e.preventDefault();
          tabs[next].focus();
          setTab(TABS[next].id);
        }}
      >
        {TABS.map((item) => (
          <button key={item.id} id={`usage-tab-${item.id}`} role="tab" aria-selected={tab === item.id} aria-controls="usage-panel" tabIndex={tab === item.id ? 0 : -1} className={tab === item.id ? "tab tab-on" : "tab"} onClick={() => setTab(item.id)}>
            {t(item.label)}
          </button>
        ))}
      </div>
      <div className="usage-filters" role="group" aria-label={t("sidebar.filters")}>
        <div className="segmented" role="radiogroup" aria-label={t("usagePane.range")}>
          {RANGES.map((days) => (
            <button key={days} role="radio" aria-checked={range === days} className={range === days ? "on" : ""} onClick={() => setRange(days)}>
              {rangeLabel(days)}
            </button>
          ))}
        </div>
        {(["project", "agent", "model", "tag"] as const).map((dim) => (
          <label key={dim} className="row">
            <span className="sr-only">{t(FILTER_LABELS[dim])}</span>
            <select className={filters[dim] ? "usage-filter-on" : ""} value={filters[dim]} onChange={(e) => setFilters({ ...filters, [dim]: e.target.value })}>
              <option value="">{t(FILTER_ALL[dim])}</option>
              {options[dim].map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          </label>
        ))}
        {filtering && (
          <button className="link" onClick={() => setFilters(NO_FILTERS)}>
            {t("usagePage.clearFilters")}
          </button>
        )}
      </div>
      {tab === "tokens" && (
        <div className="usage-filters" role="group" aria-label={t("usagePage.whatCharts")}>
          <label className="row">
            <span className="muted">{t("usagePage.groupBy")}</span>
            <select value={group} onChange={(e) => setGroup(e.target.value as GroupID)}>
              {GROUPS.map((g) => (
                <option key={g.id} value={g.id}>
                  {t(g.label)}
                </option>
              ))}
            </select>
          </label>
          <div className="segmented" role="radiogroup" aria-label={t("usagePage.measure")}>
            <button role="radio" aria-checked={m === "tokens"} className={m === "tokens" ? "on" : ""} onClick={() => setMetric("tokens")}>
              {t("tokenFigures.tokens")}
            </button>
            <button
              role="radio"
              aria-checked={m === "cost"}
              className={m === "cost" ? "on" : ""}
              disabled={!priced}
              title={priced ? undefined : t("usagePage.needPrices")}
              onClick={() => setMetric("cost")}
            >
              {t("usage.cost")}
            </button>
          </div>
          <label className="row">
            <input type="checkbox" checked={cacheReads} onChange={(e) => setCacheReads(e.target.checked)} /> {t("usagePage.cacheReads")}
          </label>
          <span className="usage-filters-end">
            {exported && <span className="muted">{t("usagePage.saved", { file: basename(exported) })}</span>}
            <button onClick={() => setSharing(!sharing)} disabled={rows.length === 0} aria-expanded={sharing}>
              {t("usagePage.share")}
            </button>
            <button onClick={exportCSV} disabled={rows.length === 0}>
              {t("usagePage.exportCsv")}
            </button>
          </span>
        </div>
      )}

      {sharing && tab === "tokens" && rows.length > 0 && (
        <CostSummaryPanel
          summary={costSummary(rows, { agent: keyOf.agent, model: keyOf.model }, {
            label: rangeLabel(range),
            since,
            until,
            pricesAsOf,
            filtered: Object.values(filters).some(Boolean),
            cacheReads,
          }, [...reporting, heldNote(heldStats(activityRows).total)])}
          onClose={() => setSharing(false)}
          onError={onError}
        />
      )}

      <div id="usage-panel" role="tabpanel" aria-labelledby={`usage-tab-${tab}`}>
        {activity && activity.pending > 0 && tab !== "tokens" && <p className="muted usage-note">{t("usagePage.counting", { pending: activity.pending })}</p>}
        {tab === "activity" && <ActivityView rows={activityRows} since={since} until={until} rangeLabel={rangeLabel(range)} onOpenSession={onOpenSession} />}
        {tab === "approvals" && <ApprovalsView rows={activityRows} since={since} until={until} rangeLabel={rangeLabel(range)} onOpenSession={onOpenSession} onChanged={() => setReloads((n) => n + 1)} onError={onError} />}
        {tab === "efficiency" && <EfficiencyView usage={fetched.filter((r) => r.day >= since)} allUsage={fetched} rows={activityRows} today={until} modelOf={keyOf.model} projectOf={project} onOpenPrices={onOpenPrices} />}
        {tab === "efficiency" && (
          <div className="usage-body">
            <WorkCostView
              matches={(r) => (!filters.project || project(r) === filters.project) && (!filters.agent || agent(r) === filters.agent) && (!filters.model || keyOf.model(r as UsageRow) === filters.model)}
              tagged={!!filters.tag}
              cacheReads={cacheReads}
              onOpenSession={onOpenSession}
              onError={onError}
            />
          </div>
        )}
        {tab === "tokens" && (
          <div className={loading && report ? "usage-body usage-loading" : "usage-body"}>
            {!report && loading && <p className="muted">{t("usagePage.loading")}</p>}
            {report && rows.length === 0 && (
              <div className="usage-empty">
                <p>{t("usagePage.none", { range: rangeLabel(range) })}</p>
                <p className="muted">{t("usagePage.noneNote")}</p>
              </div>
            )}
            <Budgets onOpenPrices={onOpenPrices} />
            {reporting.map((note) => (
              <p key={note} className="muted usage-note usage-reporting">
                {note}
              </p>
            ))}
            {rows.length > 0 && (
              <>
                <TokenFigures now={now} delta={delta} before={rangeLabel(range)} metric={m} priced={priced} cacheReads={cacheReads} />
                {now.unpricedTokens > 0 && (
                  <p className="muted usage-note">
                    {t("usagePage.unpriced", { tokens: compact(now.unpricedTokens) })}{" "}
                    <button className="link" onClick={onOpenPrices}>
                      {t("usagePage.addPrices")}
                    </button>
                  </p>
                )}

                <div className="usage-card">
                  <div className="usage-card-head">
                    <h3>{chartTitle}</h3>
                    <ul className="usage-legend" aria-label={t("usagePane.legend")}>
                      {series.map((s) => (
                        <li key={s.key}>
                          <span className="usage-swatch" style={{ background: s.color }} />
                          {s.label}
                        </li>
                      ))}
                    </ul>
                  </div>
                  <UsageChart
                    data={chart}
                    series={series}
                    format={fmt}
                    bucketLabel={(d) => (weekly ? t("insights.weekOfDate", { date: month(d) }) : month(d))}
                    axisLabel={month}
                    label={t(groupOf(group).chart, { title: chartTitle })}
                  />
                  <details className="usage-table-view">
                    <summary>{t("insights.asTable")}</summary>
                    <table className="usage-table">
                      <thead>
                        <tr>
                          <th>{weekly ? t("insights.weekOf") : t("insights.day")}</th>
                          {series.map((s) => (
                            <th key={s.key} className="num">
                              {s.label}
                            </th>
                          ))}
                          <th className="num">{t("usagePage.total")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {[...chart].reverse().map((d) => (
                          <tr key={d.day}>
                            <td>{month(d.day)}</td>
                            {series.map((s, i) => (
                              <td key={s.key} className="num">
                                {d.values[i] ? fmt(d.values[i]) : "–"}
                              </td>
                            ))}
                            <td className="num">{fmt(d.total)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </details>
                </div>

                {byKind && (
                  <div className="usage-card">
                    <h3>{t("usagePage.by.type")}</h3>
                    <table className="usage-table">
                      <thead>
                        <tr>
                          <th>{t("usagePage.group.type")}</th>
                          <th className="usage-share-col">{t("usagePage.col.shareOfTokens")}</th>
                          <th className="num">{t("tokenFigures.tokens")}</th>
                          <th className="num">{t("tokenFigures.perCall")}</th>
                          <th className="num">{t("usagePage.col.estCost")}</th>
                          <th className="num">{t("usagePage.col.shareOfCost")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {types.filter((type) => cacheReads || type.key !== "cacheRead").map((type, i) => (
                          <tr key={type.key}>
                            <td>
                              <span className="usage-swatch" style={{ background: series[i].color }} />
                              {type.label}
                            </td>
                            <td className="usage-share-col">
                              <span className="usage-share">
                                <span style={{ width: `${tokensTotal ? (type.tokens / Math.max(...types.map((x) => x.tokens))) * 100 : 0}%`, background: series[i].color }} />
                              </span>
                              <span className="muted">{tokensTotal ? percent(type.tokens / tokensTotal) : ""}</span>
                            </td>
                            <td className="num">{compact(type.tokens)}</td>
                            <td className="num">{compact(now.calls ? type.tokens / now.calls : 0)}</td>
                            <td className="num">{priced ? formatCost(type.cost) : "–"}</td>
                            <td className="num">{priced && now.cost ? percent(type.cost / now.cost) : "–"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}

                <div className="usage-card">
                  <h3>{t(byKind ? "usagePage.by.model" : groupOf(group).by)}</h3>
                  <table className="usage-table">
                    <thead>
                      <tr>
                        <th>{t(byKind ? "usage.model" : groupOf(group).label)}</th>
                        <th className="usage-share-col">{t("insights.share")}</th>
                        <th className="num">{t("tokenFigures.tokens")}</th>
                        <th className="num">{t("recentUsage.input")}</th>
                        <th className="num">{t("recentUsage.output")}</th>
                        {cacheReads && <th className="num">{t("recentUsage.cacheRead")}</th>}
                        <th className="num">{t("insights.activity.calls")}</th>
                        <th className="num">{t("usagePage.col.estCost")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {groups.map((g) => (
                        <tr key={g.key}>
                          <td>
                            {!byKind && <span className="usage-swatch" style={{ background: colorOf(top.includes(g.key) ? g.key : OTHER) }} />}
                            {/* A run is no filter of the other tabs, so its row only reads. */}
                            {group === "run" ? (
                              <span>{g.key}</span>
                            ) : (
                              <button className="link usage-pick" title={t("usagePage.showOnly")} onClick={() => setFilters({ ...filters, [byKind ? "model" : group]: g.key })}>
                                {g.key}
                              </button>
                            )}
                          </td>
                          <td className="usage-share-col">
                            <span className="usage-share">
                              <span style={{ width: `${maxGroup ? (value(g, m) / maxGroup) * 100 : 0}%`, background: colorOf(top.includes(g.key) ? g.key : OTHER) }} />
                            </span>
                            <span className="muted">{value(now, m) ? percent(value(g, m) / value(now, m)) : ""}</span>
                          </td>
                          <td className="num">{compact(value(g, "tokens"))}</td>
                          <td className="num">{compact(g.input)}</td>
                          <td className="num">{compact(g.output)}</td>
                          {cacheReads && <td className="num">{compact(g.cacheRead)}</td>}
                          <td className="num">{compact(g.calls)}</td>
                          <td className="num">{g.unpricedTokens === value(g, "tokens") ? "–" : formatCost(g.cost)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                <div className="usage-card">
                  <h3>{t("usagePage.topSessions")}</h3>
                  <table className="usage-table">
                    <thead>
                      <tr>
                        <th>{t("insights.session")}</th>
                        <th>{t("sidebar.groupBy.project")}</th>
                        <th>{t("usagePage.group.agent")}</th>
                        <th className="num">{t("tokenFigures.tokens")}</th>
                        <th className="num">{t("insights.activity.calls")}</th>
                        <th className="num">{t("usagePage.col.estCost")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sessions.map((g) => {
                        const r = sessionRow(g.key);
                        return (
                          <tr key={g.key}>
                            <td>
                              {r.deleted ? (
                                <span className="muted">{t("insights.deletedSession")}</span>
                              ) : (
                                <button className="link" onClick={() => onOpenSession(g.key)}>
                                  {r.title || t("insights.untitled")}
                                </button>
                              )}
                            </td>
                            <td>{project(r)}</td>
                            <td>{agent(r)}</td>
                            <td className="num">{compact(value(g, "tokens"))}</td>
                            <td className="num">{compact(g.calls)}</td>
                            <td className="num">{g.unpricedTokens === value(g, "tokens") ? "–" : formatCost(g.cost)}</td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              </>
            )}
          </div>
        )}
      </div>
    </section>
  );
}
