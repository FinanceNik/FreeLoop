import { Events } from "@wailsio/runtime";
import { useEffect, useRef, useState } from "react";
import { compact, dailyByType, days, groupBy, TOKEN_TYPES, tokensOf, topKeys, type Row, type TokenType } from "./analytics.ts";
import { EngineService, EVENT_NAME, type UsageRow } from "./api.ts";
import TokenFigures from "./TokenFigures.tsx";
import UsageChart, { type Series } from "./UsageChart.tsx";
import { PANE_RANGES, period } from "./usagePane.ts";
import { locale, numberFormat, t, type PlainKey } from "./i18n.ts";

// The usage pane's views over days (GUI-038): the token figures with a chart per day, or
// what each model and each provider or agent used. Cache reads count, as on the usage
// page's default, so the two show the same figures.

type Named = { id: string; name: string };
const SETTLE_MS = 2000;
const TOP = 5;
const day = (d: string) => new Date(`${d}T12:00:00`).toLocaleDateString(locale(), { month: "short", day: "numeric" });
const TYPE_LABEL: Record<TokenType, PlainKey> = {
  input: "recentUsage.input",
  output: "recentUsage.output",
  cacheWrite: "recentUsage.cacheWrite",
  cacheRead: "recentUsage.cacheRead",
};

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
    <div className="segmented" role="radiogroup" aria-label={t("usagePane.range")}>
      {PANE_RANGES.map((r) => (
        <button key={r} role="radio" aria-checked={range === r} className={range === r ? "on" : ""} onClick={() => setRange(r)}>
          {t("paneRange.days", { count: r })}
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
        <p className="muted">{t("paneRange.none", { days: range })}</p>
      </div>
    );

  if (view === "tokens") {
    const series: Series[] = TOKEN_TYPES.map((type, i) => ({ key: type.key, label: t(TYPE_LABEL[type.key]), color: `var(--series-${i + 1})` }));
    const chart = dailyByType(p.rows, days(p.since, until), "tokens");
    return (
      <div className="pane-range">
        {ranges}
        <TokenFigures now={p.now} delta={p.delta} before={t("paneRange.days", { count: range })} metric="tokens" priced={p.rows.some((r) => r.priced)} cacheReads />
        <ul className="usage-legend" aria-label={t("usagePane.legend")}>
          {series.map((s) => (
            <li key={s.key}>
              <span className="usage-swatch" style={{ background: s.color }} />
              {s.label}
            </li>
          ))}
        </ul>
        <UsageChart fill data={chart} series={series} format={compact} bucketLabel={day} label={t("paneRange.chart", { days: range })} />
      </div>
    );
  }

  const agent = (r: UsageRow) =>
    r.harnessId && r.harnessId !== "builtin" ? (harnesses.find((h) => h.id === r.harnessId)?.name ?? r.harnessId) : (providers.find((x) => x.id === r.providerId)?.name ?? (r.providerId || t("app.source.unknown")));
  const total = tokensOf(p.now);
  const shares = (title: string, key: (r: Row & UsageRow) => string) => {
    const groups = groupBy(p.rows, key, "tokens");
    const top = topKeys(groups, TOP);
    const shown = groups.filter((g) => top.includes(g.key));
    const rest = groups.filter((g) => !top.includes(g.key));
    const restTokens = rest.reduce((n, g) => n + tokensOf(g), 0);
    const list = [...shown.map((g, i) => ({ key: g.key, tokens: tokensOf(g), color: `var(--series-${i + 1})` })), ...(rest.length ? [{ key: t("recentUsage.other", { count: rest.length }), tokens: restTokens, color: "var(--series-other)" }] : [])];
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
                {compact(g.tokens)} <span className="muted">{total ? numberFormat({ style: "percent", maximumFractionDigits: 0 }).format(g.tokens / total) : ""}</span>
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
      <p className="muted">{t("paneRange.total", { tokens: compact(total), days: range })}</p>
      {shares(t("paneRange.models"), (r) => r.model || t("recentUsage.modelNotReported", { name: agent(r) }))}
      {shares(t("paneRange.agents"), agent)}
    </div>
  );
}
