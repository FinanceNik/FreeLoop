import { useEffect, useEffectEvent, useState } from "react";
import { Events } from "@wailsio/runtime";
import { EngineService, EVENT_NAME } from "./api.ts";
import { since } from "./board.ts";
import { currentLimits, LIMIT_STATE_LABEL, limitSummaries, percentText, reportAge, resetText, type LimitPeriodRow } from "./limits.ts";
import { t, type PlainKey } from "./i18n.ts";

const COLUMNS: PlainKey[] = ["limits.col.limit", "limits.col.runs", "limits.col.averagePeak", "limits.col.highest", "limits.col.close", "limits.col.reached"];

// Limits shows how much of an account's limits agents have reported using (GUI-028).
// No agent or vendor is asked: a report comes back with a model call, and the page says
// how old it is. Each limit is one number and a reset time, so it is a tile, not a chart.
export default function Limits({ compact = false, onError }: { compact?: boolean; onError: (err: unknown) => void }) {
  const [periods, setPeriods] = useState<LimitPeriodRow[] | null>(null);
  const [now, setNow] = useState(() => new Date());
  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    const load = () =>
      EngineService.Limits()
        .then((list) => {
          setPeriods((list ?? []) as LimitPeriodRow[]);
          setNow(new Date());
        })
        .catch(failed);
    load();
    // A reset comes with no event, so the clock moves the page on as well.
    const every = setInterval(() => setNow(new Date()), 30000);
    const off = Events.On(EVENT_NAME, ({ data: ev }) => {
      if (ev?.type === "limit_update") load();
    });
    return () => {
      clearInterval(every);
      off();
    };
  }, []);
  if (periods === null) return null;
  if (periods.length === 0) {
    return <p className="muted limits-empty">{t("limits.none")}</p>;
  }
  const current = currentLimits(periods, now);
  const summaries = limitSummaries(periods, now);
  const sources = new Set(current.map((l) => l.source));
  return (
    <div className={compact ? "limits limits-compact" : "limits"}>
      <div className="limits-tiles">
        {current.map((l) => (
          <div key={l.key} className={`limit-tile limit-${l.state}`}>
            <span className="limit-head">
              <span className="muted">
                {sources.size > 1 ? `${l.source}: ` : ""}
                {l.label}
              </span>
              {/* The meter says it is used; the words stay for VoiceOver (GUI-042). */}
              <strong>
                {l.used === null ? (
                  t("limits.noFigure")
                ) : (
                  <>
                    <span aria-hidden="true">{percentText(l.used)}</span>
                    <span className="sr-only">{t("limits.percentUsed", { percent: percentText(l.used) })}</span>
                  </>
                )}
              </strong>
            </span>
            {l.used !== null && (
              <div className="limit-meter" role="meter" aria-label={t("limits.meter", { label: l.label, percent: percentText(l.used) })} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(l.used)}>
                <div style={{ width: `${Math.min(100, l.used)}%` }} />
              </div>
            )}
            {LIMIT_STATE_LABEL[l.state] && <span className="limit-state">{t(LIMIT_STATE_LABEL[l.state]!)}</span>}
            <span className="muted">{[l.used !== null ? resetText(l.resetsAt, now) : "", reportAge(l.lastReport, now, since)].filter(Boolean).join(" · ")}</span>
          </div>
        ))}
      </div>
      {!compact && summaries.length > 0 && (
        <table className="limits-history">
          <caption className="muted">{t("limits.history")}</caption>
          <thead>
            <tr>
              {COLUMNS.map((key) => (
                <th key={key} scope="col">
                  {t(key)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {summaries.map((s) => (
              <tr key={s.key}>
                <th scope="row">
                  {sources.size > 1 ? `${s.source}: ` : ""}
                  {s.label}
                </th>
                <td>{s.runs}</td>
                <td>{percentText(s.averagePeak)}</td>
                <td>{percentText(s.highest)}</td>
                <td>{s.closeRuns}</td>
                <td>{s.refusedRuns}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
