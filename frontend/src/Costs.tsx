import { useEffect, useEffectEvent, useState } from "react";
import { t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";
import { EngineService, type DayCost, type Price, type ProviderInfo } from "./api.ts";
import { formatCost } from "./cost.ts";
import { shortTokens } from "./contextMeter.ts";

// The price table and the usage per day on the Settings page (PRV-008).

// The per-token columns of a price row.
const PRICE_COLUMNS = [
  { field: "input", label: "prices.input" },
  { field: "output", label: "prices.output" },
  { field: "cacheRead", label: "prices.cacheRead" },
  { field: "cacheWrite", label: "prices.cacheWrite" },
  { field: "cacheWrite1h", label: "prices.cacheWrite1h" },
] as const;

// What a price row's model can look like; a pattern, the same in every language.
const PRICE_PATTERN = "claude-*";

export function PriceTable({ prices, providers, onChange }: { prices: Price[]; providers: ProviderInfo[]; onChange: (p: Price[]) => void }) {
  const [list, setList] = useState<{ asOf: string; prices: Price[] } | null>(null);
  useEffect(() => {
    EngineService.BuiltinPrices()
      .then((l) => setList({ asOf: l.asOf, prices: l.prices ?? [] }))
      .catch(() => {});
  }, []);
  const set = (i: number, patch: Partial<Price>) => onChange(prices.map((p, j) => (j === i ? { ...p, ...patch } : p)));
  const num = (v: string) => (v === "" ? 0 : Math.max(0, Number(v)));
  return (
    <>
      <p className="muted">
        {t("prices.unit")} {list ? t("prices.listAsOf", { date: list.asOf }) : t("prices.list")} {tNodes("prices.rows", { pattern: <code>{PRICE_PATTERN}</code> })}
      </p>
      {prices.length > 0 && (
        <div className="prices-scroll">
        <table className="prices">
          <thead>
            <tr>
              <th>{t("prices.provider")}</th>
              <th>{t("prices.model")}</th>
              {PRICE_COLUMNS.map((c) => (
                <th key={c.field}>{t(c.label)}</th>
              ))}
              <th />
            </tr>
          </thead>
          <tbody>
            {prices.map((p, i) => (
              <tr key={i}>
                <td>
                  <select aria-label={t("prices.provider")} value={p.providerId} onChange={(e) => set(i, { providerId: e.target.value })}>
                    <option value="">{t("prices.anyProvider")}</option>
                    {providers.map((x) => (
                      <option key={x.id} value={x.id}>
                        {x.name}
                      </option>
                    ))}
                    {p.providerId && !providers.some((x) => x.id === p.providerId) && <option value={p.providerId}>{p.providerId}</option>}
                  </select>
                </td>
                <td>
                  <input aria-label={t("prices.model")} required value={p.model} onChange={(e) => set(i, { model: e.target.value })} />
                </td>
                {PRICE_COLUMNS.map(({ field: k, label }) => (
                  <td key={k}>
                    <input
                      aria-label={t(label)}
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
                    {t("common.remove")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        </div>
      )}
      <div className="row">
        <button type="button" onClick={() => onChange([...prices, { providerId: "", model: "", input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cacheWrite1h: 0, longPromptOver: 0, from: "" }])}>
          {t("prices.add")}
        </button>
      </div>
      {list && (
        <details>
          <summary>{t("prices.listTitle")}</summary>
          <div className="prices-scroll">
          <table className="prices">
            <thead>
              <tr>
                <th>{t("prices.model")}</th>
                {PRICE_COLUMNS.map((c) => (
                  <th key={c.field}>{t(c.label)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {/* A model's dated names have the same price as its name. */}
              {list.prices
                .filter((p) => !p.model.includes("-20*"))
                .map((p) => (
                  <tr key={p.model}>
                    <td>{p.model}</td>
                    <td>{p.input}</td>
                    <td>{p.output}</td>
                    <td>{p.cacheRead || "–"}</td>
                    <td>{p.cacheWrite || "–"}</td>
                    <td>{p.cacheWrite1h || "–"}</td>
                  </tr>
                ))}
            </tbody>
          </table>
          </div>
        </details>
      )}
    </>
  );
}

// UsageTable lists the last 30 days. Costs come from the saved price
// table, so they refresh when version (bumped on save) changes.
export function UsageTable({ version, onPrice, onError }: { version: unknown; onPrice: (providerId: string, model: string) => void; onError: (err: unknown) => void }) {
  const [days, setDays] = useState<DayCost[] | null>(null);
  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    EngineService.Usage(30)
      .then((d) => setDays(d ?? []))
      .catch(failed);
  }, [version]);
  if (!days) return null;
  if (days.length === 0) return <p className="muted">{t("usage.none")}</p>;
  const total = days.reduce((n, d) => n + d.cost, 0);
  const unpriced = new Map<string, { providerId: string; model: string }>();
  for (const d of days) for (const m of d.models ?? []) if (!m.priced) unpriced.set(`${m.providerId}/${m.model}`, m);
  return (
    <>
      <p>
        {days.some((d) => (d.models ?? []).some((m) => m.priced)) ? (
          <>
            {tNodes("usage.last30", { cost: <strong>≈ {formatCost(total)}</strong> })}
            {unpriced.size > 0 && <span className="muted"> {t("usage.plusUnpriced")}</span>}
          </>
        ) : (
          <span className="muted">{t("usage.addPrices")}</span>
        )}
      </p>
      {unpriced.size > 0 && (
        <ul className="unpriced">
          {[...unpriced.values()].map((m) => (
            <li key={`${m.providerId}/${m.model}`}>
              {m.model || t("usage.unnamedModel")} <span className="muted">{m.providerId || t("usage.externalAgent")}</span>{" "}
              {m.model && (
                <button type="button" className="link" onClick={() => onPrice(m.providerId, m.model)}>
                  {t("prices.add")}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      <table className="usage">
        <thead>
          <tr>
            <th>{t("usage.day")}</th>
            <th>{t("usage.model")}</th>
            <th>{t("usage.calls")}</th>
            <th title={t("usage.inTitle")}>{t("usage.in")}</th>
            <th>{t("usage.out")}</th>
            <th>{t("usage.cost")}</th>
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
                <td>{m.model || <span className="muted">{t("usage.unnamed")}</span>}</td>
                <td>{m.calls}</td>
                <td>{shortTokens(m.inputTokens + m.cacheReadTokens + m.cacheWriteTokens)}</td>
                <td>{shortTokens(m.outputTokens)}</td>
                <td>{m.priced ? formatCost(m.cost) : <span className="muted">{t("usage.noPrice")}</span>}</td>
              </tr>
            )),
          )}
        </tbody>
      </table>
    </>
  );
}
