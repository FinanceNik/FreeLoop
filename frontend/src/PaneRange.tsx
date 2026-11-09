import { Events } from "@wailsio/runtime";
import { useEffect, useRef, useState } from "react";
import { compact, dailyByType, days, groupBy, OTHER, TOKEN_TYPES, tokensOf, topKeys, type Row } from "./analytics.ts";
import { EngineService, EVENT_NAME, type UsageRow } from "./api.ts";
import TokenFigures from "./TokenFigures.tsx";
import UsageChart, { type Series } from "./UsageChart.tsx";
import { PANE_RANGES, period } from "./usagePane.ts";
import { locale } from "./i18n.ts";

// The usage pane's views over days (GUI-038): the token figures with a chart per day, or
// what each model and each provider or agent used. Cache reads count, as on the usage
// page's default, so the two show the same figures.

type Named = { id: string; name: string };
const SETTLE_MS = 2000;
const TOP = 5;
const day = (d: string) => new Date(`${d}T12:00:00`).toLocaleDateString(locale(), { month: "short", day: "numeric" });

export default function PaneRange({ view, providers, harnesses, onError }: { view: "tokens" | "models"; providers: readonly Named[]; harnesses: readonly Named[]; onError: (err: unknown) => void }) {
  const [range, setRange] = useState<(typeof PANE_RANGES)[number]>(30);
  const [rows, setRows] = useState<(Row & UsageRow)[] | null>(null);
  const [until, setUntil] = useState("");
  const latest = useRef(0);
  useEffect(() => {
    let settle: ReturnType<typeof setTimeout> | undefined;
    const load = () => {
      const ask = ++latest.current;
      EngineService.UsageReport(range * 2)
        .then((r) => {
          if (ask !== latest.current || !r) return;
          setRows((r.rows ?? []) as (Row & UsageRow)[]);
          setUntil(r.until);
        })
        .catch(onError);
    };
    load();
    const off = Events.On(EVENT_NAME, ({ data: ev }) => {
      if (ev?.type !== "usage_update") return;
      clearTimeout(settle);
      settle = setTimeout(load, SETTLE_MS);
    });
    return () => {
      clearTimeout(settle);
      off();
    };
  }, [range]);

  const ranges = (
    <div className="segmented" role="radiogroup" aria-label="Range">
      {PANE_RANGES.map((r) => (
        <button key={r} role="radio" aria-checked={range === r} className={range === r ? "on" : ""} onClick={() => setRange(r)}>
          {r} days
        </button>
      ))}
    </div>
  );
  if (rows === null || !until) return <div className="pane-range">{ranges}</div>;
  const p = period(rows, range, until);
  if (p.rows.length === 0)
    return (
      <div className="pane-range">
        {ranges}
        <p className="muted">No model calls in the last {range} days.</p>
      </div>
    );

  if (view === "tokens") {
    const series: Series[] = TOKEN_TYPES.map((t, i) => ({ key: t.key, label: t.label, color: `var(--series-${i + 1})` }));
    const chart = dailyByType(p.rows, days(p.since, until), "tokens");
    return (
      <div className="pane-range">
        {ranges}
        <TokenFigures now={p.now} delta={p.delta} before={`${range} days`} metric="tokens" priced={p.rows.some((r) => r.priced)} cacheReads />
        <ul className="usage-legend" aria-label="Legend">
          {series.map((s) => (
            <li key={s.key}>
              <span className="usage-swatch" style={{ background: s.color }} />
              {s.label}
            </li>
          ))}
        </ul>
        <UsageChart fill data={chart} series={series} format={compact} bucketLabel={day} label={`Tokens per day by token type, last ${range} days`} />
      </div>
    );
  }

  const agent = (r: UsageRow) =>
    r.harnessId && r.harnessId !== "builtin" ? (harnesses.find((h) => h.id === r.harnessId)?.name ?? r.harnessId) : (providers.find((x) => x.id === r.providerId)?.name ?? (r.providerId || "Unknown provider"));
  const total = tokensOf(p.now);
  const shares = (title: string, key: (r: Row & UsageRow) => string) => {
    const groups = groupBy(p.rows, key, "tokens");
    const top = topKeys(groups, TOP);
    const shown = groups.filter((g) => top.includes(g.key));
    const rest = groups.filter((g) => !top.includes(g.key));
    const restTokens = rest.reduce((n, g) => n + tokensOf(g), 0);
    const list = [...shown.map((g, i) => ({ key: g.key, tokens: tokensOf(g), color: `var(--series-${i + 1})` })), ...(rest.length ? [{ key: `${OTHER} (${rest.length})`, tokens: restTokens, color: "var(--series-other)" }] : [])];
    const most = Math.max(1, ...list.map((g) => g.tokens));
    return (
      <section className="pane-shares" aria-label={title}>
        <h3>{title}</h3>
        <ul>
          {list.map((g) => (
            <li key={g.key}>
              <span className="pane-share-name" title={g.key}>
                {g.key}
              </span>
              <span className="usage-share" aria-hidden="true">
                <span style={{ width: `${(g.tokens / most) * 100}%`, background: g.color }} />
              </span>
              <span className="pane-share-value">
                {compact(g.tokens)} <span className="muted">{total ? `${Math.round((g.tokens / total) * 100)}%` : ""}</span>
              </span>
            </li>
          ))}
        </ul>
      </section>
    );
  };
  return (
    <div className="pane-range">
      {ranges}
      <p className="muted">
        {compact(total)} tokens in the last {range} days
      </p>
      {shares("Models", (r) => r.model || `${agent(r)} (model not reported)`)}
      {shares("Providers and agents", agent)}
    </div>
  );
}
