import { compact, perCall, value, type Metric, type Totals } from "./analytics.ts";
import { formatCost } from "./cost.ts";
import { Tile } from "./UsageInsights.tsx";

// The figures over a range of days, the same on the usage page and in the usage pane
// (GUI-018, GUI-038), so the two cannot come to disagree.
export default function TokenFigures({ now, delta, before, metric, priced, cacheReads }: { now: Totals; delta: number | null; before: string; metric: Metric; priced: boolean; cacheReads: boolean }) {
  return (
    <div className="usage-figures">
      <div className="usage-hero">
        <span className="muted">{metric === "cost" ? "Estimated cost" : "Tokens"}</span>
        <strong>{metric === "cost" ? formatCost(now.cost) : compact(value(now, "tokens"))}</strong>
        <span className="muted">{delta === null ? "Nothing to compare with in the period before" : `${delta >= 0 ? "+" : "−"}${Math.abs(Math.round(delta * 100))}% on the ${before} before`}</span>
        {metric === "tokens" && now.cacheRead > 0 && <span className="muted">{compact(now.input + now.output + now.cacheWrite)} without cache reads</span>}
      </div>
      <Tile label="Input" value={compact(now.input)} />
      <Tile label="Output" value={compact(now.output)} />
      {cacheReads && <Tile label="From the prompt cache" value={`${Math.round(now.cacheShare * 100)}%`} note={`${compact(now.cacheRead)} tokens read`} />}
      <Tile label="Per model call" value={compact(perCall(now))} note={`${compact(now.calls ? now.output / now.calls : 0)} of them output`} />
      <Tile label="Per session" value={compact(now.sessions ? value(now, "tokens") / now.sessions : 0)} note={`${compact(now.calls)} calls in ${now.sessions} ${now.sessions === 1 ? "session" : "sessions"}`} />
      {metric === "tokens" && priced && <Tile label="Estimated cost" value={formatCost(now.cost)} />}
    </div>
  );
}
