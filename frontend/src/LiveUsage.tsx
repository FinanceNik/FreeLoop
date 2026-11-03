import { Events } from "@wailsio/runtime";
import { useEffect, useRef, useState } from "react";
import { EngineService, EVENT_NAME } from "./api.ts";
import { formatCost } from "./cost.ts";
import { formatTokens, recentChart, recentSeries, recentTotals, stepLabel, timeLabel, type RecentBucket } from "./recentUsage.ts";
import UsageChart from "./UsageChart.tsx";
import { Tile } from "./UsageInsights.tsx";

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

export default function LiveUsage({ onError }: { onError: (err: unknown) => void }) {
  const [hours, setHours] = useState(12);
  const [metric, setMetric] = useState<"tokens" | "cost">("tokens");
  const [cacheReads, setCacheReads] = useState(false);
  const [buckets, setBuckets] = useState<RecentBucket[] | null>(null);
  const [step, setStep] = useState(15);
  const latest = useRef(0);

  useEffect(() => {
    let settle: ReturnType<typeof setTimeout> | undefined;
    const load = () => {
      const ask = ++latest.current;
      EngineService.RecentUsage(hours)
        .then((r) => {
          if (ask !== latest.current) return;
          setBuckets((r.buckets ?? []) as RecentBucket[]);
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

  const series = recentSeries(metric, cacheReads);
  const list = buckets ?? [];
  const totals = recentTotals(list, cacheReads);
  const format = metric === "cost" ? formatCost : formatTokens;
  const priced = totals.calls > totals.unpriced;
  const title = `${metric === "cost" ? "Estimated cost" : "Tokens"} per ${step} minutes, last ${hours} hours`;

  return (
    <section className="live-usage" aria-label="Recent usage">
      <div className="live-usage-controls">
        <div className="segmented" role="radiogroup" aria-label="Range">
          {RANGES.map((r) => (
            <button key={r.hours} role="radio" aria-checked={hours === r.hours} className={hours === r.hours ? "on" : ""} onClick={() => setHours(r.hours)}>
              {r.label}
            </button>
          ))}
        </div>
        <div className="segmented" role="radiogroup" aria-label="Measure">
          {(["tokens", "cost"] as const).map((m) => (
            <button key={m} role="radio" aria-checked={metric === m} className={metric === m ? "on" : ""} onClick={() => setMetric(m)}>
              {m === "tokens" ? "Tokens" : "Cost"}
            </button>
          ))}
        </div>
        <label className="row">
          <input type="checkbox" checked={cacheReads} onChange={(e) => setCacheReads(e.target.checked)} /> Cache reads
        </label>
      </div>
      <div className="live-usage-tiles">
        <Tile label="Tokens" value={formatTokens(totals.tokens)} />
        <Tile label="Output" value={formatTokens(totals.output)} />
        <Tile label="Model calls" value={totals.calls.toLocaleString()} />
        <Tile label="Estimated cost" value={priced ? formatCost(totals.cost) : "No price"} note={priced && totals.unpriced ? `${totals.unpriced} calls have no price` : undefined} />
      </div>
      <div className="usage-card">
        <div className="usage-card-head">
          <h3>{title}</h3>
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
        {buckets === null ? (
          <p className="muted">Counting…</p>
        ) : (
          <UsageChart data={recentChart(list, metric, cacheReads)} series={series} format={format} bucketLabel={(d) => stepLabel(d, step)} axisLabel={timeLabel} label={title} />
        )}
        {buckets !== null && totals.calls === 0 && <p className="muted">No model calls in the last {hours} hours.</p>}
      </div>
    </section>
  );
}
