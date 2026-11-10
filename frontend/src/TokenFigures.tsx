import { compact, perCall, value, type Metric, type Totals } from "./analytics.ts";
import { formatCost } from "./cost.ts";
import { numberFormat, t } from "./i18n.ts";
import { Tile } from "./UsageInsights.tsx";

const percent = (share: number) => numberFormat({ style: "percent", maximumFractionDigits: 0 }).format(share);

// The figures over a range of days, the same on the usage page and in the usage pane
// (GUI-018, GUI-038), so the two cannot come to disagree.
export default function TokenFigures({ now, delta, before, metric, priced, cacheReads }: { now: Totals; delta: number | null; before: string; metric: Metric; priced: boolean; cacheReads: boolean }) {
  return (
    <div className="usage-figures">
      <div className="usage-hero">
        <span className="muted">{metric === "cost" ? t("tokenFigures.cost") : t("tokenFigures.tokens")}</span>
        <strong>{metric === "cost" ? formatCost(now.cost) : compact(value(now, "tokens"))}</strong>
        <span className="muted">{delta === null ? t("tokenFigures.noComparison") : t("tokenFigures.change", { change: `${delta >= 0 ? "+" : "−"}${percent(Math.abs(Math.round(delta * 100)) / 100)}`, before })}</span>
        {metric === "tokens" && now.cacheRead > 0 && <span className="muted">{t("tokenFigures.withoutCacheReads", { tokens: compact(now.input + now.output + now.cacheWrite) })}</span>}
      </div>
      <Tile label={t("tokenFigures.input")} value={compact(now.input)} />
      <Tile label={t("tokenFigures.output")} value={compact(now.output)} />
      {cacheReads && <Tile label={t("tokenFigures.fromPromptCache")} value={percent(now.cacheShare)} note={t("tokenFigures.tokensRead", { tokens: compact(now.cacheRead) })} />}
      <Tile label={t("tokenFigures.perCall")} value={compact(perCall(now))} note={t("tokenFigures.outputOfThem", { tokens: compact(now.calls ? now.output / now.calls : 0) })} />
      <Tile
        label={t("tokenFigures.perSession")}
        value={compact(now.sessions ? value(now, "tokens") / now.sessions : 0)}
        note={t("tokenFigures.callsInSessions", { calls: compact(now.calls), count: now.sessions })}
      />
      {metric === "tokens" && priced && <Tile label={t("tokenFigures.cost")} value={formatCost(now.cost)} />}
    </div>
  );
}
