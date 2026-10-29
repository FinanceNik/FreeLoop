import { useEffect, useState } from "react";
import { EngineService, type DayCost, type Price, type ProviderInfo } from "./api.ts";
import { formatCost } from "./cost.ts";

// The price table and the usage per day on the Settings page (PRV-008).

export function PriceTable({ prices, providers, onChange }: { prices: Price[]; providers: ProviderInfo[]; onChange: (p: Price[]) => void }) {
  const set = (i: number, patch: Partial<Price>) => onChange(prices.map((p, j) => (j === i ? { ...p, ...patch } : p)));
  const num = (v: string) => (v === "" ? 0 : Math.max(0, Number(v)));
  return (
    <>
      <p className="muted">
        US dollars per million tokens. The model can be a pattern such as <code>claude-*</code>; the first matching row applies. Freeloop
        ships no prices: copy them from your provider's price list.
      </p>
      {prices.length > 0 && (
        <table className="prices">
          <thead>
            <tr>
              <th>Provider</th>
              <th>Model</th>
              <th>Input</th>
              <th>Output</th>
              <th>Cache read</th>
              <th>Cache write</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {prices.map((p, i) => (
              <tr key={i}>
                <td>
                  <select aria-label="Provider" value={p.providerId} onChange={(e) => set(i, { providerId: e.target.value })}>
                    <option value="">Any</option>
                    {providers.map((x) => (
                      <option key={x.id} value={x.id}>
                        {x.name}
                      </option>
                    ))}
                    {p.providerId && !providers.some((x) => x.id === p.providerId) && <option value={p.providerId}>{p.providerId}</option>}
                  </select>
                </td>
                <td>
                  <input aria-label="Model" required value={p.model} onChange={(e) => set(i, { model: e.target.value })} />
                </td>
                {(["input", "output", "cacheRead", "cacheWrite"] as const).map((k) => (
                  <td key={k}>
                    <input
                      aria-label={k}
                      type="number"
                      min={0}
                      step="any"
                      value={p[k] || ""}
                      placeholder="0"
                      onChange={(e) => set(i, { [k]: num(e.target.value) })}
                    />
                  </td>
                ))}
                <td>
                  <button type="button" className="link" onClick={() => onChange(prices.filter((_, j) => j !== i))}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div className="row">
        <button type="button" onClick={() => onChange([...prices, { providerId: "", model: "", input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }])}>
          Add a price
        </button>
      </div>
    </>
  );
}

const tokens = (n: number) => (n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${Math.round(n / 1e3)}k` : String(n));

// UsageTable lists the last 30 days. Costs come from the saved price
// table, so they refresh when version (bumped on save) changes.
export function UsageTable({ version, onPrice, onError }: { version: unknown; onPrice: (providerId: string, model: string) => void; onError: (err: unknown) => void }) {
  const [days, setDays] = useState<DayCost[] | null>(null);
  useEffect(() => {
    EngineService.Usage(30)
      .then((d) => setDays(d ?? []))
      .catch(onError);
  }, [version]);
  if (!days) return null;
  if (days.length === 0) return <p className="muted">No model calls in the last 30 days.</p>;
  const total = days.reduce((n, d) => n + d.cost, 0);
  const unpriced = new Map<string, { providerId: string; model: string }>();
  for (const d of days) for (const m of d.models ?? []) if (!m.priced) unpriced.set(`${m.providerId}/${m.model}`, m);
  return (
    <>
      <p>
        {days.some((d) => (d.models ?? []).some((m) => m.priced)) ? (
          <>
            Last 30 days: <strong>≈ {formatCost(total)}</strong>
            {unpriced.size > 0 && <span className="muted"> plus calls to models without a price</span>}
          </>
        ) : (
          <span className="muted">Add prices to estimate what these calls cost.</span>
        )}
      </p>
      {unpriced.size > 0 && (
        <ul className="unpriced">
          {[...unpriced.values()].map((m) => (
            <li key={`${m.providerId}/${m.model}`}>
              {m.model || "an unnamed model"} <span className="muted">{m.providerId || "external agent"}</span>{" "}
              {m.model && (
                <button type="button" className="link" onClick={() => onPrice(m.providerId, m.model)}>
                  Add a price
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      <table className="usage">
        <thead>
          <tr>
            <th>Day</th>
            <th>Model</th>
            <th>Calls</th>
            <th title="Input, including cache reads and writes">In</th>
            <th>Out</th>
            <th>Cost</th>
          </tr>
        </thead>
        <tbody>
          {days.flatMap((d) =>
            (d.models ?? []).map((m, i) => (
              <tr key={`${d.day}/${m.providerId}/${m.model}`} className={i === 0 ? "day-first" : undefined}>
                <td>
                  {i === 0 && (
                    <>
                      {d.day} {(d.models ?? []).some((m) => m.priced) && <span className="muted">≈ {formatCost(d.cost)}</span>}
                    </>
                  )}
                </td>
                <td>{m.model || <span className="muted">unnamed</span>}</td>
                <td>{m.calls}</td>
                <td>{tokens(m.inputTokens + m.cacheReadTokens + m.cacheWriteTokens)}</td>
                <td>{tokens(m.outputTokens)}</td>
                <td>{m.priced ? formatCost(m.cost) : <span className="muted">no price</span>}</td>
              </tr>
            )),
          )}
        </tbody>
      </table>
    </>
  );
}
