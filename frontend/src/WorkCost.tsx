import { useEffect, useEffectEvent, useState } from "react";
import { compact, weeks as weeksOf } from "./analytics.ts";
import { EngineService, type WorkCommitCost, type WorkReport } from "./api.ts";
import { formatCost } from "./cost.ts";
import { locale, numberFormat, t } from "./i18n.ts";
import UsageChart from "./UsageChart.tsx";
import { commitChange, limitChange, limitSeries, tokensOf, weekStats, type Change } from "./workcost.ts";

// What a commit costs, week by week (GUI-051), in the Usage page's Efficiency tab.
// Models and subscriptions change under the user; the same kind of work costing
// more, or a percent of a limit buying less, is what this shows. Costs are at
// today's prices, so only use moves them.

// What the page's filters ask of a row.
export interface WorkFilter {
  (r: { projectId: string; projectRoot: string; harnessId: string; providerId: string; model: string }): boolean;
}

interface Props {
  matches: WorkFilter;
  // A tag filter is set, which commits cannot follow.
  tagged: boolean;
  cacheReads: boolean;
  onOpenSession: (id: string) => void;
  onError: (err: unknown) => void;
}

// How far back it reads: half a year, in weeks.
const WEEKS = 26;

const signed = (ratio: number) => numberFormat({ style: "percent", maximumFractionDigits: 0, signDisplay: "exceptZero" }).format(ratio);
const share = (ratio: number) => numberFormat({ style: "percent", maximumFractionDigits: 0 }).format(ratio);
const weekLabel = (monday: string) => t("work.weekOf", { day: new Date(`${monday}T12:00:00`).toLocaleDateString(locale(), { month: "short", day: "numeric" }) });
const axisLabel = (monday: string) => new Date(`${monday}T12:00:00`).toLocaleDateString(locale(), { month: "short", day: "numeric" });

export default function WorkCostView({ matches, tagged, cacheReads, onOpenSession, onError }: Props) {
  const [report, setReport] = useState<WorkReport | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [exported, setExported] = useState("");
  // A long history is folded in steps: ask again until none is left.
  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout>;
    const load = () =>
      EngineService.WorkCost(WEEKS)
        .then((r) => {
          if (!live) return;
          setReport(r);
          if (r.pending > 0) timer = setTimeout(load, 200);
        })
        .catch(failed);
    load();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, []);

  if (!report) return <p className="muted">{t("work.loading")}</p>;
  const today = new Date();
  const until = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}-${String(today.getDate()).padStart(2, "0")}`;
  const all = weeksOf(report.since, until);
  const commits = (report.commits ?? []).filter(matches);
  const spend = (report.spend ?? []).filter(matches);
  const limits = report.limits ?? [];
  const first = all.findIndex((w) => commits.some((c) => c.week === w) || spend.some((s) => s.week === w) || limits.some((l) => l.week === w));
  const weeks = first < 0 ? [] : all.slice(first);
  const stats = weekStats(weeks, commits, spend, cacheReads);
  const priced = commits.some((c) => !c.unpriced);
  const per = (c: WorkCommitCost) => (priced ? c.cost : tokensOf(c, cacheReads));
  const fmt = priced ? formatCost : compact;
  const series = limitSeries(limits);
  const change = commitChange(weeks, commits, per);
  const said = (c: Change | null, key: "work.change" | "work.limitChange", label?: string) =>
    c ? t(key, { change: signed(c.ratio), recent: c.recent, earlier: c.earlier, label: label ?? "" }) : null;

  const exportCSV = () =>
    EngineService.ExportWorkCost(WEEKS)
      .then((path) => path && setExported(path))
      .catch(onError);

  return (
    <div className="usage-card work-cost">
      <h3>{t("work.title")}</h3>
      <p className="muted">{t("work.intro")}</p>
      {report.pending > 0 && <p className="muted usage-note">{t("work.counting", { pending: report.pending })}</p>}
      {tagged && <p className="muted usage-note">{t("work.tagNote")}</p>}
      {commits.length === 0 ? (
        <p className="muted">{t("work.none")}</p>
      ) : (
        <>
          <p role="status">{said(change, "work.change") ?? t("work.tooFew", { weeks: 2 + 8, commits: 10 })}</p>
          <UsageChart
            data={stats.map((s) => ({ day: s.week, values: [priced ? s.median : s.medianTokens], total: priced ? s.median : s.medianTokens }))}
            series={[{ key: "median", label: t(priced ? "work.chart.cost" : "work.chart.tokens"), color: "var(--series-1)" }]}
            format={fmt}
            bucketLabel={weekLabel}
            axisLabel={axisLabel}
            label={t(priced ? "work.chart.cost" : "work.chart.tokens")}
          />
        </>
      )}
      {series.map((s) => {
        const c = limitChange(weeks, limits.filter((l) => `${l.source}\u0000${l.window}` === s.key));
        return (
          <p key={s.key}>
            {said(c, "work.limitChange", `${s.source}, ${s.label}`) ?? t("work.limitTooFew", { label: `${s.source}, ${s.label}` })}
          </p>
        );
      })}
      {weeks.length > 0 && (
        <table className="usage-table work-table">
          <thead>
            <tr>
              <th>{t("work.week")}</th>
              <th className="num">{t("work.commits")}</th>
              <th className="num">{t(priced ? "work.median" : "work.medianTokens")}</th>
              {priced && <th className="num">{t("work.middleHalf")}</th>}
              {priced && <th className="num">{t("work.allIn")}</th>}
              <th className="num">{t("work.uncommitted")}</th>
              <th className="num">{t("work.lines")}</th>
              {series.map((s) => (
                <th key={s.key} className="num">
                  {t("work.perPercent", { label: s.label })}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {[...stats].reverse().map((s) => {
              const mine = commits.filter((c) => c.week === s.week).sort((a, b) => per(b) - per(a));
              return [
                <tr key={s.week}>
                  <td>
                    {s.commits > 0 ? (
                      <button className="link" aria-expanded={open === s.week} onClick={() => setOpen(open === s.week ? null : s.week)}>
                        {weekLabel(s.week)}
                      </button>
                    ) : (
                      weekLabel(s.week)
                    )}
                  </td>
                  <td className="num">{s.commits}</td>
                  <td className="num">{s.commits ? fmt(priced ? s.median : s.medianTokens) : "–"}</td>
                  {priced && <td className="num">{s.commits ? `${formatCost(s.p25)}–${formatCost(s.p75)}` : "–"}</td>}
                  {priced && <td className="num">{s.commits ? formatCost(s.allIn) : "–"}</td>}
                  <td className="num">{s.spend > 0 ? share(s.uncommitted) : "–"}</td>
                  <td className="num">{s.commits ? compact(s.medianLines) : "–"}</td>
                  {series.map((x) => {
                    const w = x.weeks.find((v) => v.week === s.week);
                    return (
                      <td key={x.key} className="num" title={w ? t("work.runs", { count: w.runs }) : undefined}>
                        {w ? formatCost(w.perPercent) : "–"}
                      </td>
                    );
                  })}
                </tr>,
                open === s.week && (
                  <tr key={`${s.week}-commits`} className="work-commits">
                    <td colSpan={5 + (priced ? 2 : 0) + series.length}>
                      <ul>
                        {mine.slice(0, 10).map((c) => (
                          <li key={c.sha}>
                            <code>{c.sha.slice(0, 7)}</code>{" "}
                            <button className="link" onClick={() => onOpenSession(c.sessionId)}>
                              {c.title || c.sessionId}
                            </button>{" "}
                            <span className="muted">
                              {t("work.commitLine", { cost: fmt(per(c)), lines: c.added >= 0 ? compact(c.added + c.deleted) : "–", model: c.model || "–" })}
                            </span>
                          </li>
                        ))}
                      </ul>
                    </td>
                  </tr>
                ),
              ];
            })}
          </tbody>
        </table>
      )}
      <p className="muted usage-note">
        {t("work.note")} {report.unattributed > 0 && t("work.unattributed", { count: report.unattributed })} {series.length > 0 && t("work.limitNote")}
      </p>
      <p className="row">
        <button onClick={exportCSV}>{t("work.export")}</button>
        {exported && <span className="muted">{t("work.exported", { path: exported })}</span>}
      </p>
    </div>
  );
}
