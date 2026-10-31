import { useEffect, useState } from "react";
import { EngineService, type HarnessInfo, type ProjectInfo, type ProviderInfo, type UsageReport, type UsageRow } from "./api.ts";
import { byType, change, compact, daily, dailyByType, days, groupBy, OTHER, perCall, TOKEN_TYPES, topKeys, totals, value, weekStart, weeks, withoutCacheReads, type Metric, type Row } from "./analytics.ts";
import { formatCost } from "./cost.ts";
import UsageChart, { type Series } from "./UsageChart.tsx";

// The usage dashboard (GUI-018, PRV-008): tokens and estimated cost over a
// range, by kind of token, model, agent or project, and the sessions that
// used the most. Costs
// come from the price table in Settings; tokens of unpriced models are
// counted but cost nothing here.

interface Props {
  projects: ProjectInfo[];
  providers: ProviderInfo[];
  harnesses: HarnessInfo[];
  onOpenSession: (id: string) => void;
  onOpenPrices: () => void;
  onError: (err: unknown) => void;
}

const RANGES = [
  { days: 7, label: "7 days" },
  { days: 30, label: "30 days" },
  { days: 90, label: "90 days" },
  { days: 365, label: "12 months" },
];

const GROUPS = [
  { id: "type", label: "Token type" },
  { id: "model", label: "Model" },
  { id: "agent", label: "Provider or agent" },
  { id: "project", label: "Project" },
] as const;
type GroupID = (typeof GROUPS)[number]["id"];

const SERIES = 5; // colours; a sixth group and beyond fold into Other

const basename = (p: string) => p.split("/").filter(Boolean).pop() ?? p;
const month = (d: string) => new Date(`${d}T12:00:00`).toLocaleDateString([], { month: "short", day: "numeric" });

export default function Usage({ projects, providers, harnesses, onOpenSession, onOpenPrices, onError }: Props) {
  const [range, setRange] = useState(30);
  const [group, setGroup] = useState<GroupID>("type");
  const [metric, setMetric] = useState<Metric>("tokens");
  const [cacheReads, setCacheReads] = useState(true);
  const [report, setReport] = useState<UsageReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [exported, setExported] = useState("");

  useEffect(() => {
    let live = true;
    setLoading(true);
    // Twice the range, for the comparison with the period before.
    EngineService.UsageReport(range * 2)
      .then((r) => live && setReport(r))
      .catch(onError)
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [range]);

  const agent = (r: UsageRow) => {
    if (r.harnessId && r.harnessId !== "builtin") return harnesses.find((h) => h.id === r.harnessId)?.name ?? r.harnessId;
    return providers.find((p) => p.id === r.providerId)?.name ?? (r.providerId || "Unknown provider");
  };
  const project = (r: UsageRow) => projects.find((p) => p.id === r.projectId)?.name ?? (r.projectRoot ? basename(r.projectRoot) : "Deleted sessions");
  const keyOf: Record<Exclude<GroupID, "type">, (r: UsageRow) => string> = {
    model: (r) => r.model || `${agent(r)} (model not reported)`,
    agent,
    project,
  };

  const fetched = (report?.rows ?? []) as (UsageRow & Row)[];
  const all = cacheReads ? fetched : withoutCacheReads(fetched);
  const until = report?.until ?? new Date().toISOString().slice(0, 10);
  const span = report ? days(report.since, until) : [];
  const since = span[Math.max(0, span.length - range)] ?? until;
  const rows = all.filter((r) => r.day >= since);
  const before = all.filter((r) => r.day < since);
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
    ? TOKEN_TYPES.map((t, i) => ({ key: t.key, label: t.label, color: `var(--series-${i + 1})` })).filter((s) => cacheReads || s.key !== "cacheRead")
    : [
        ...top.map((k, i) => ({ key: k, label: k, color: `var(--series-${i + 1})` })),
        ...(groups.length > top.length ? [{ key: OTHER, label: `${OTHER} (${groups.length - top.length})`, color: "var(--series-other)" }] : []),
      ];
  const weekly = range > 90;
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
      <h2>Usage</h2>
      <div className="usage-filters" role="group" aria-label="Filters">
        <div className="segmented" role="radiogroup" aria-label="Range">
          {RANGES.map((r) => (
            <button key={r.days} role="radio" aria-checked={range === r.days} className={range === r.days ? "on" : ""} onClick={() => setRange(r.days)}>
              {r.label}
            </button>
          ))}
        </div>
        <label className="row">
          <span className="muted">Group by</span>
          <select value={group} onChange={(e) => setGroup(e.target.value as GroupID)}>
            {GROUPS.map((g) => (
              <option key={g.id} value={g.id}>
                {g.label}
              </option>
            ))}
          </select>
        </label>
        <div className="segmented" role="radiogroup" aria-label="Measure">
          <button role="radio" aria-checked={m === "tokens"} className={m === "tokens" ? "on" : ""} onClick={() => setMetric("tokens")}>
            Tokens
          </button>
          <button
            role="radio"
            aria-checked={m === "cost"}
            className={m === "cost" ? "on" : ""}
            disabled={!priced}
            title={priced ? undefined : "Add prices in Settings to see estimated costs"}
            onClick={() => setMetric("cost")}
          >
            Cost
          </button>
        </div>
        <label className="row">
          <input type="checkbox" checked={cacheReads} onChange={(e) => setCacheReads(e.target.checked)} /> Include cache reads
        </label>
        <span className="usage-filters-end">
          {exported && <span className="muted">Saved to {basename(exported)}</span>}
          <button onClick={exportCSV} disabled={rows.length === 0}>
            Export CSV…
          </button>
        </span>
      </div>

      <div className={loading && report ? "usage-body usage-loading" : "usage-body"}>
        {!report && loading && <p className="muted">Loading…</p>}
        {report && rows.length === 0 && (
          <div className="usage-empty">
            <p>No model calls in the last {RANGES.find((r) => r.days === range)?.label}.</p>
            <p className="muted">Usage is counted from every session's model calls, including external agents that report their tokens.</p>
          </div>
        )}
        {rows.length > 0 && (
          <>
            <div className="usage-figures">
              <div className="usage-hero">
                <span className="muted">{m === "cost" ? "Estimated cost" : "Tokens"}</span>
                <strong>{m === "cost" ? formatCost(now.cost) : compact(value(now, "tokens"))}</strong>
                <span className="muted">
                  {delta === null ? "Nothing to compare with in the period before" : `${delta >= 0 ? "+" : "−"}${Math.abs(Math.round(delta * 100))}% on the ${RANGES.find((r) => r.days === range)?.label} before`}
                </span>
                {m === "tokens" && now.cacheRead > 0 && (
                  <span className="muted">
                    {compact(now.input + now.output + now.cacheWrite)} without cache reads
                  </span>
                )}
              </div>
              <Tile label="Input" value={compact(now.input)} />
              <Tile label="Output" value={compact(now.output)} />
              {cacheReads && <Tile label="From the prompt cache" value={`${Math.round(now.cacheShare * 100)}%`} note={`${compact(now.cacheRead)} tokens read`} />}
              <Tile label="Per model call" value={compact(perCall(now))} note={`${compact(now.calls ? now.output / now.calls : 0)} of them output`} />
              <Tile label="Per session" value={compact(now.sessions ? value(now, "tokens") / now.sessions : 0)} note={`${compact(now.calls)} calls in ${now.sessions} ${now.sessions === 1 ? "session" : "sessions"}`} />
              {m === "tokens" && priced && <Tile label="Estimated cost" value={formatCost(now.cost)} />}
            </div>
            {now.unpricedTokens > 0 && (
              <p className="muted usage-note">
                {compact(now.unpricedTokens)} tokens are on models without a price and count as no cost.{" "}
                <button className="link" onClick={onOpenPrices}>
                  Add prices
                </button>
              </p>
            )}

            <div className="usage-card">
              <div className="usage-card-head">
                <h3>{m === "cost" ? "Estimated cost" : "Tokens"} per {weekly ? "week" : "day"}</h3>
                <ul className="usage-legend" aria-label="Legend">
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
                bucketLabel={(d) => (weekly ? `Week of ${month(d)}` : month(d))}
                axisLabel={month}
                label={`${m === "cost" ? "Estimated cost" : "Tokens"} per ${weekly ? "week" : "day"} by ${GROUPS.find((g) => g.id === group)?.label.toLowerCase()}`}
              />
              <details className="usage-table-view">
                <summary>Show as a table</summary>
                <table className="usage-table">
                  <thead>
                    <tr>
                      <th>{weekly ? "Week of" : "Day"}</th>
                      {series.map((s) => (
                        <th key={s.key} className="num">
                          {s.label}
                        </th>
                      ))}
                      <th className="num">Total</th>
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
                <h3>By token type</h3>
                <table className="usage-table">
                  <thead>
                    <tr>
                      <th>Token type</th>
                      <th className="usage-share-col">Share of tokens</th>
                      <th className="num">Tokens</th>
                      <th className="num">Per model call</th>
                      <th className="num">Est. cost</th>
                      <th className="num">Share of cost</th>
                    </tr>
                  </thead>
                  <tbody>
                    {types.filter((t) => cacheReads || t.key !== "cacheRead").map((t, i) => (
                      <tr key={t.key}>
                        <td>
                          <span className="usage-swatch" style={{ background: series[i].color }} />
                          {t.label}
                        </td>
                        <td className="usage-share-col">
                          <span className="usage-share">
                            <span style={{ width: `${tokensTotal ? (t.tokens / Math.max(...types.map((x) => x.tokens))) * 100 : 0}%`, background: series[i].color }} />
                          </span>
                          <span className="muted">{tokensTotal ? `${Math.round((t.tokens / tokensTotal) * 100)}%` : ""}</span>
                        </td>
                        <td className="num">{compact(t.tokens)}</td>
                        <td className="num">{compact(now.calls ? t.tokens / now.calls : 0)}</td>
                        <td className="num">{priced ? formatCost(t.cost) : "–"}</td>
                        <td className="num">{priced && now.cost ? `${Math.round((t.cost / now.cost) * 100)}%` : "–"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            <div className="usage-card">
              <h3>By {byKind ? "model" : GROUPS.find((g) => g.id === group)?.label.toLowerCase()}</h3>
              <table className="usage-table">
                <thead>
                  <tr>
                    <th>{byKind ? "Model" : GROUPS.find((g) => g.id === group)?.label}</th>
                    <th className="usage-share-col">Share</th>
                    <th className="num">Tokens</th>
                    <th className="num">Input</th>
                    <th className="num">Output</th>
                    {cacheReads && <th className="num">Cache read</th>}
                    <th className="num">Calls</th>
                    <th className="num">Est. cost</th>
                  </tr>
                </thead>
                <tbody>
                  {groups.map((g) => (
                    <tr key={g.key}>
                      <td>
                        {!byKind && <span className="usage-swatch" style={{ background: colorOf(top.includes(g.key) ? g.key : OTHER) }} />}
                        {g.key}
                      </td>
                      <td className="usage-share-col">
                        <span className="usage-share">
                          <span style={{ width: `${maxGroup ? (value(g, m) / maxGroup) * 100 : 0}%`, background: colorOf(top.includes(g.key) ? g.key : OTHER) }} />
                        </span>
                        <span className="muted">{value(now, m) ? `${Math.round((value(g, m) / value(now, m)) * 100)}%` : ""}</span>
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
              <h3>Sessions that used the most</h3>
              <table className="usage-table">
                <thead>
                  <tr>
                    <th>Session</th>
                    <th>Project</th>
                    <th>Provider or agent</th>
                    <th className="num">Tokens</th>
                    <th className="num">Calls</th>
                    <th className="num">Est. cost</th>
                  </tr>
                </thead>
                <tbody>
                  {sessions.map((g) => {
                    const r = sessionRow(g.key);
                    return (
                      <tr key={g.key}>
                        <td>
                          {r.deleted ? (
                            <span className="muted">Deleted session</span>
                          ) : (
                            <button className="link" onClick={() => onOpenSession(g.key)}>
                              {r.title || "Untitled"}
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
    </section>
  );
}

function Tile({ label, value, note }: { label: string; value: string; note?: string }) {
  return (
    <div className="usage-tile">
      <span className="muted">{label}</span>
      <strong>{value}</strong>
      {note && <span className="muted">{note}</span>}
    </div>
  );
}
