import { HELD_BY } from "./transcript.ts";
import { useState } from "react";
import { decisionStats, duration, heldStats, efficiency, eventCount, forecast, heatLevel, heatmap, perDay, repeatApprovals, sessionStats, timeUnit, toolStats, turnStats, WEEKDAYS, type ActivityRow, type EffRow, type Efficiency } from "./activity.ts";
import { compact, days, weekStart, weeks } from "./analytics.ts";
import { EngineService } from "./api.ts";
import { formatCost } from "./cost.ts";
import Heatmap from "./Heatmap.tsx";
import UsageChart, { type Series } from "./UsageChart.tsx";
import { locale, numberFormat, t, type PlainKey } from "./i18n.ts";

// The dashboard's tabs beyond tokens (GUI-021). Each chart's numbers are also in
// a table, and every tab follows the page's range and filters.

export function Tile({ label, value, note }: { label: string; value: string; note?: string }) {
  return (
    <div className="usage-tile">
      <span className="muted">{label}</span>
      <strong>{value}</strong>
      {note && <span className="muted">{note}</span>}
    </div>
  );
}

const ONE: (label: string) => Series[] = (label) => [{ key: "v", label, color: "var(--series-1)" }];
const month = (d: string) => new Date(`${d}T12:00:00`).toLocaleDateString(locale(), { month: "short", day: "numeric" });
const percent = (share: number) => numberFormat({ style: "percent", maximumFractionDigits: 0 }).format(share);

const REASONS: Record<string, PlainKey> = {
  end_turn: "insights.reason.endTurn",
  cancelled: "insights.reason.cancelled",
  max_tokens: "insights.reason.maxTokens",
  max_turn_requests: "insights.reason.maxTurnRequests",
  refusal: "insights.reason.refusal",
  error: "insights.reason.error",
  unknown: "insights.reason.unknown",
};
const reasonLabel = (r: string) => (Object.hasOwn(REASONS, r) ? t(REASONS[r]) : r.replace(/_/g, " "));

const ACTORS: Record<string, PlainKey> = {
  user: "insights.actor.user",
  rule: "insights.actor.rule",
  mode: "insights.actor.mode",
  sandbox: "insights.actor.sandbox",
  hook: "insights.actor.hook",
  policy: "insights.actor.policy",
  protected_path: "insights.actor.protectedPath",
};
const actorLabel = (a: string) => (Object.hasOwn(ACTORS, a) ? t(ACTORS[a]) : a.replace(/_/g, " "));

function Share({ value, max, label }: { value: number; max: number; label: string }) {
  return (
    <td className="usage-share-col">
      <span className="usage-share">
        <span style={{ width: `${max ? (value / max) * 100 : 0}%`, background: "var(--series-1)" }} />
      </span>
      <span className="muted">{label}</span>
    </td>
  );
}

// A chart of one measure over the range, by day or, past 90 days, by week.
function bucketsOf(rows: ActivityRow[], since: string, until: string) {
  const weekly = days(since, until).length > 90;
  return {
    weekly,
    buckets: weekly ? weeks(since, until) : days(since, until),
    bucketed: weekly ? rows.map((r) => ({ ...r, day: weekStart(r.day) })) : rows,
    bucketLabel: (d: string) => (weekly ? t("insights.weekOfDate", { date: month(d) }) : month(d)),
  };
}

function Empty({ children, note }: { children: string; note: string }) {
  return (
    <div className="usage-empty">
      <p>{children}</p>
      <p className="muted">{note}</p>
    </div>
  );
}

interface ActivityProps {
  rows: ActivityRow[];
  since: string;
  until: string;
  rangeLabel: string;
  onOpenSession: (id: string) => void;
}

export function ActivityView({ rows: all, since, until, rangeLabel, onOpenSession }: ActivityProps) {
  const rows = all.filter((r) => r.day >= since);
  const turns = turnStats(rows);
  const tools = toolStats(rows);
  const calls = tools.reduce((n, t) => n + t.calls, 0);
  const failures = tools.reduce((n, t) => n + t.failures, 0);
  const { weekly, buckets, bucketed, bucketLabel } = bucketsOf(rows, since, until);
  const chart = perDay(bucketed, buckets, "turn", (r) => r.count);
  const heat = heatmap(rows);
  const sessions = sessionStats(rows).slice(0, 10);
  const maxCalls = Math.max(0, ...tools.map((t) => t.calls));
  const maxReason = Math.max(0, ...turns.reasons.map((r) => r.count));
  if (turns.turns === 0 && calls === 0) {
    return <Empty note={t("insights.activity.noneNote")}>{t("insights.activity.none", { range: rangeLabel })}</Empty>;
  }
  const cellsWithTurns = heat.cells.flatMap((row, d) => row.map((n, h) => ({ d, h, n }))).filter((c) => c.n > 0).sort((a, b) => b.n - a.n);
  return (
    <div className="usage-body">
      <div className="usage-figures">
        <div className="usage-hero">
          <span className="muted">{t("insights.turns")}</span>
          <strong>{compact(turns.turns)}</strong>
          <span className="muted">{t("insights.activity.average", { duration: duration(turns.average) })}</span>
          <span className="muted">{t("insights.activity.total", { duration: duration(turns.millis) })}</span>
        </div>
        <Tile label={t("insights.toolCalls")} value={compact(calls)} note={turns.turns ? t("insights.activity.perTurn", { calls: compact(calls / turns.turns) }) : undefined} />
        <Tile label={t("insights.activity.failedCalls")} value={calls ? percent(failures / calls) : "–"} note={t("insights.activity.failedCount", { failures: compact(failures) })} />
        <Tile label={t("insights.activity.edits")} value={compact(eventCount(rows, "edit"))} />
        <Tile label={t("insights.activity.pullRequests")} value={compact(eventCount(rows, "pull_request"))} />
        <Tile label={t("insights.activity.rewinds")} value={compact(eventCount(rows, "rewind"))} />
        <Tile label={t("insights.activity.compactions")} value={compact(eventCount(rows, "compaction"))} />
        <Tile label={t("insights.activity.errors")} value={compact(eventCount(rows, "error"))} />
      </div>

      <div className="usage-card">
        <h3>{t(weekly ? "insights.activity.turnsPerWeek" : "insights.activity.turnsPerDay")}</h3>
        <UsageChart data={chart} series={ONE(t("insights.turns"))} format={compact} bucketLabel={bucketLabel} axisLabel={month} label={t(weekly ? "insights.activity.turnsPerWeek" : "insights.activity.turnsPerDay")} />
        <details className="usage-table-view">
          <summary>{t("insights.asTable")}</summary>
          <table className="usage-table">
            <thead>
              <tr>
                <th>{t(weekly ? "insights.weekOf" : "insights.day")}</th>
                <th className="num">{t("insights.turns")}</th>
              </tr>
            </thead>
            <tbody>
              {[...chart].reverse().map((d) => (
                <tr key={d.day}>
                  <td>{month(d.day)}</td>
                  <td className="num">{d.total || "–"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      </div>

      <div className="usage-card">
        <h3>{t("insights.activity.whenTurnsStart")}</h3>
        <Heatmap data={heat} />
        <div className="usage-heat-legend" aria-hidden="true">
          <span>{t("insights.activity.fewer")}</span>
          {[0, 1, 2, 3, 4, 5].map((l) => (
            <i key={l} className={`usage-heat-cell heat-${l}`} />
          ))}
          <span>{t("insights.activity.more")}</span>
        </div>
        <details className="usage-table-view">
          <summary>{t("insights.asTable")}</summary>
          <table className="usage-table">
            <thead>
              <tr>
                <th>{t("insights.activity.weekday")}</th>
                <th>{t("insights.activity.hour")}</th>
                <th className="num">{t("insights.turns")}</th>
                <th className="num">{t("insights.activity.scale")}</th>
              </tr>
            </thead>
            <tbody>
              {cellsWithTurns.map((c) => (
                <tr key={`${c.d}-${c.h}`}>
                  <td>{WEEKDAYS[c.d]}</td>
                  <td>{String(c.h).padStart(2, "0")}:00</td>
                  <td className="num">{c.n}</td>
                  <td className="num">{t("insights.activity.scaleOf", { level: heatLevel(c.n, heat.max) })}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      </div>

      <div className="usage-card">
        <h3>{t("insights.activity.tools")}</h3>
        <table className="usage-table">
          <thead>
            <tr>
              <th>{t("insights.activity.tool")}</th>
              <th className="usage-share-col">{t("insights.activity.shareOfCalls")}</th>
              <th className="num">{t("insights.activity.calls")}</th>
              <th className="num">{t("insights.failed")}</th>
              <th className="num">{t("insights.activity.failureRate")}</th>
              <th className="num">{t("insights.activity.averageTime")}</th>
              <th className="num">{t("insights.activity.totalTime")}</th>
            </tr>
          </thead>
          <tbody>
            {tools.map((tool) => (
              <tr key={tool.name}>
                <td>{tool.name || t("insights.activity.unnamedTool")}</td>
                <Share value={tool.calls} max={maxCalls} label={percent(calls ? tool.calls / calls : 0)} />
                <td className="num">{compact(tool.calls)}</td>
                <td className="num">{tool.failures ? compact(tool.failures) : "–"}</td>
                <td className="num">{tool.failures ? percent(tool.failureRate) : "–"}</td>
                <td className="num">{duration(tool.average)}</td>
                <td className="num">{duration(tool.millis)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="muted usage-note">{t("insights.activity.toolsNote")}</p>
      </div>

      {turns.reasons.length > 0 && (
        <div className="usage-card">
          <h3>{t("insights.activity.endings")}</h3>
          <table className="usage-table">
            <thead>
              <tr>
                <th>{t("insights.activity.ending")}</th>
                <th className="usage-share-col">{t("insights.share")}</th>
                <th className="num">{t("insights.turns")}</th>
                <th className="num">{t("insights.activity.averageLength")}</th>
              </tr>
            </thead>
            <tbody>
              {turns.reasons.map((r) => (
                <tr key={r.reason}>
                  <td>{reasonLabel(r.reason)}</td>
                  <Share value={r.count} max={maxReason} label={percent(r.count / turns.turns)} />
                  <td className="num">{compact(r.count)}</td>
                  <td className="num">{duration(r.millis / r.count)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="usage-card">
        <h3>{t("insights.activity.busiest")}</h3>
        <table className="usage-table">
          <thead>
            <tr>
              <th>{t("insights.session")}</th>
              <th className="num">{t("insights.turns")}</th>
              <th className="num">{t("insights.toolCalls")}</th>
              <th className="num">{t("insights.failed")}</th>
              <th className="num">{t("insights.activity.timeInTurns")}</th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((s) => {
              const r = rows.find((x) => x.sessionId === s.sessionId);
              return (
                <tr key={s.sessionId}>
                  <td>
                    {r?.deleted ? (
                      <span className="muted">{t("insights.deletedSession")}</span>
                    ) : (
                      <button className="link usage-pick" onClick={() => onOpenSession(s.sessionId)}>
                        {r?.title || t("insights.untitled")}
                      </button>
                    )}
                  </td>
                  <td className="num">{compact(s.turns)}</td>
                  <td className="num">{compact(s.toolCalls)}</td>
                  <td className="num">{s.failures ? compact(s.failures) : "–"}</td>
                  <td className="num">{duration(s.millis)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

interface ApprovalsProps {
  rows: ActivityRow[];
  since: string;
  until: string;
  rangeLabel: string;
  onChanged: () => void;
  onOpenSession: (id: string) => void;
  onError: (err: unknown) => void;
}

export function ApprovalsView({ rows: all, since, until, rangeLabel, onChanged, onOpenSession, onError }: ApprovalsProps) {
  const rows = all.filter((r) => r.day >= since);
  const stats = decisionStats(rows);
  const held = heldStats(rows);
  const repeats = repeatApprovals(rows);
  const { weekly, buckets, bucketed, bucketLabel } = bucketsOf(rows, since, until);
  const waitedMillis = perDay(bucketed, buckets, "decision", (r) => (r.name.startsWith("user:") ? r.millis : 0));
  const unit = timeUnit(Math.max(0, ...waitedMillis.map((d) => d.total)));
  const waits = waitedMillis.map((d) => ({ ...d, values: [d.total / unit], total: d.total / unit }));
  const waitLabel = (v: number) => duration(v * unit);
  const maxActor = Math.max(0, ...stats.actors.map((a) => a.total));
  const [busy, setBusy] = useState("");
  if (stats.total === 0) {
    return <Empty note={t("insights.approvals.noneNote")}>{t("insights.approvals.none", { range: rangeLabel })}</Empty>;
  }
  const allow = (tool: string, command: string, path: string) => {
    setBusy(`${tool}\t${command}\t${path}`);
    EngineService.AllowRepeated(tool, command, path)
      .then(onChanged)
      .catch(onError)
      .finally(() => setBusy(""));
  };
  return (
    <div className="usage-body">
      <div className="usage-figures">
        <div className="usage-hero">
          <span className="muted">{t("insights.approvals.waited")}</span>
          <strong>{duration(stats.waited)}</strong>
          <span className="muted">{t("insights.approvals.averageWait", { duration: duration(stats.averageWait) })}</span>
        </div>
        <Tile label={t("insights.approvals.decisions")} value={compact(stats.total)} />
        <Tile label={t("insights.approvals.askedYou")} value={compact(stats.byUser)} note={t("insights.approvals.ofDecisions", { share: percent(stats.byUser / stats.total) })} />
        <Tile label={t("insights.approvals.denied")} value={compact(stats.denied)} note={t("insights.approvals.ofDecisions", { share: percent(stats.denied / stats.total) })} />
      </div>

      <div className="usage-card">
        <h3>{t("insights.approvals.held")}</h3>
        {held.total === 0 ? (
          <p className="muted">{t("insights.approvals.heldNone")}</p>
        ) : (
          <>
            <p className="muted">
              {t("insights.approvals.heldCount", { count: held.total })} {t("insights.approvals.heldWhere")}
            </p>
            <table className="usage-table">
              <tbody>
                {held.kinds.map((k) => (
                  <tr key={k.kind}>
                    <td>{HELD_BY[k.kind] ?? k.kind.replace(/_/g, " ")}</td>
                    <td className="num">{compact(k.count)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <table className="usage-table">
              <thead>
                <tr>
                  <th>{t("insights.session")}</th>
                  <th className="num">{t("insights.approvals.heldColumn")}</th>
                </tr>
              </thead>
              <tbody>
                {held.sessions.slice(0, 10).map((s) => (
                  <tr key={s.sessionId}>
                    <td>
                      {s.deleted ? (
                        <span className="muted">{s.title || t("insights.deletedSession")}</span>
                      ) : (
                        <button className="link" onClick={() => onOpenSession(s.sessionId)}>
                          {s.title || s.sessionId}
                        </button>
                      )}
                    </td>
                    <td className="num">{compact(s.count)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </div>

      {repeats.length > 0 && (
        <div className="usage-card">
          <h3>{t("insights.approvals.repeats")}</h3>
          <table className="usage-table">
            <thead>
              <tr>
                <th>{t("insights.approvals.commandOrFile")}</th>
                <th>{t("insights.activity.tool")}</th>
                <th className="num">{t("insights.approvals.timesAllowed")}</th>
                <th className="num">{t("insights.approvals.timeWaited")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {repeats.slice(0, 15).map((r) => {
                const id = `${r.tool}\t${r.command}\t${r.path}`;
                return (
                  <tr key={id}>
                    <td className="usage-mono">{r.command || r.path}</td>
                    <td>{r.tool}</td>
                    <td className="num">{compact(r.allowed)}</td>
                    <td className="num">{duration(r.waited)}</td>
                    <td className="num">
                      <button disabled={busy === id} aria-label={t("insights.approvals.alwaysAllowNamed", { what: r.command || r.path })} onClick={() => allow(r.tool, r.command, r.path)}>
                        {t("insights.approvals.alwaysAllow")}
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          <p className="muted usage-note">{t("insights.approvals.repeatsNote")}</p>
        </div>
      )}

      <div className="usage-card">
        <h3>{t("insights.approvals.whoDecided")}</h3>
        <table className="usage-table">
          <thead>
            <tr>
              <th>{t("insights.approvals.decidedBy")}</th>
              <th className="usage-share-col">{t("insights.share")}</th>
              <th className="num">{t("insights.approvals.decisions")}</th>
              <th className="num">{t("insights.approvals.allowed")}</th>
              <th className="num">{t("insights.approvals.denied")}</th>
              <th className="num">{t("insights.approvals.cancelled")}</th>
            </tr>
          </thead>
          <tbody>
            {stats.actors.map((a) => (
              <tr key={a.actor}>
                <td>{actorLabel(a.actor)}</td>
                <Share value={a.total} max={maxActor} label={percent(a.total / stats.total)} />
                <td className="num">{compact(a.total)}</td>
                <td className="num">{a.allowed ? compact(a.allowed) : "–"}</td>
                <td className="num">{a.denied ? compact(a.denied) : "–"}</td>
                <td className="num">{a.cancelled ? compact(a.cancelled) : "–"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {stats.waited > 0 && (
        <div className="usage-card">
          <h3>{t(weekly ? "insights.approvals.waitedPerWeek" : "insights.approvals.waitedPerDay")}</h3>
          <UsageChart
            data={waits}
            series={ONE(t("insights.approvals.waitedSeries"))}
            format={waitLabel}
            bucketLabel={bucketLabel}
            axisLabel={month}
            label={t(weekly ? "insights.approvals.waitedPerWeekLabel" : "insights.approvals.waitedPerDayLabel")}
          />
          <details className="usage-table-view">
            <summary>{t("insights.asTable")}</summary>
            <table className="usage-table">
              <thead>
                <tr>
                  <th>{t(weekly ? "insights.weekOf" : "insights.day")}</th>
                  <th className="num">{t("insights.approvals.waitedSeries")}</th>
                </tr>
              </thead>
              <tbody>
                {[...waits].reverse().map((d) => (
                  <tr key={d.day}>
                    <td>{month(d.day)}</td>
                    <td className="num">{d.total ? waitLabel(d.total) : "–"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </details>
        </div>
      )}
    </div>
  );
}

interface EfficiencyProps<R extends EffRow> {
  usage: R[];
  allUsage: R[]; // beyond the range, for the month so far
  rows: ActivityRow[];
  today: string;
  modelOf: (r: R) => string;
  projectOf: (r: R) => string;
  onOpenPrices: () => void;
}

export function EfficiencyView<R extends EffRow>({ usage, allUsage, rows, today, modelOf, projectOf, onOpenPrices }: EfficiencyProps<R>) {
  const models = efficiency(usage, rows, modelOf);
  const projects = efficiency(usage, rows, projectOf);
  const pace = forecast(allUsage, today);
  const priced = models.some((m) => m.priced);
  const sum = (f: (e: Efficiency) => number) => models.reduce((n, m) => n + f(m), 0);
  const turns = sum((m) => m.turns);
  const tokens = sum((m) => m.tokens);
  const cost = sum((m) => m.cost);
  const saved = sum((m) => m.saved);
  if (models.length === 0) {
    return <Empty note={t("insights.efficiency.noneNote")}>{t("insights.efficiency.none")}</Empty>;
  }
  const monthName = new Date(`${today}T12:00:00`).toLocaleDateString(locale(), { month: "long" });
  return (
    <div className="usage-body">
      <div className="usage-figures">
        <div className="usage-hero">
          <span className="muted">{t("insights.efficiency.tokensPerTurn")}</span>
          <strong>{turns ? compact(tokens / turns) : "–"}</strong>
          <span className="muted">{t("insights.efficiency.tokensInTurns", { tokens: compact(tokens), turns: compact(turns), count: Math.round(turns) })}</span>
        </div>
        {priced && <Tile label={t("insights.efficiency.costPerTurn")} value={turns ? formatCost(cost / turns) : "–"} />}
        {priced && <Tile label={t("insights.efficiency.savedByPromptCache")} value={formatCost(saved)} note={t("insights.efficiency.savedNote")} />}
        {pace &&
          (priced ? (
            <Tile label={t("insights.efficiency.projected", { month: monthName })} value={formatCost(pace.projected)} note={t("insights.efficiency.spentSoFar", { spent: formatCost(pace.spent) })} />
          ) : (
            <Tile label={t("insights.efficiency.projected", { month: monthName })} value={compact(pace.projectedTokens)} note={t("insights.efficiency.tokensSoFar", { tokens: compact(pace.tokens) })} />
          ))}
      </div>
      {!priced && (
        <p className="muted usage-note">
          {t("insights.efficiency.addPricesNote")}{" "}
          <button className="link" onClick={onOpenPrices}>
            {t("insights.efficiency.addPrices")}
          </button>
        </p>
      )}
      <EfficiencyTable heading="insights.efficiency.byModel" column="insights.efficiency.model" groups={models} priced={priced} />
      <EfficiencyTable heading="insights.efficiency.byProject" column="insights.efficiency.project" groups={projects} priced={priced} />
      <p className="muted usage-note">{t("insights.efficiency.note")}</p>
    </div>
  );
}

function EfficiencyTable({ heading, column, groups, priced }: { heading: PlainKey; column: PlainKey; groups: Efficiency[]; priced: boolean }) {
  const max = Math.max(0, ...groups.map((g) => g.tokensPerTurn));
  return (
    <div className="usage-card">
      <h3>{t(heading)}</h3>
      <table className="usage-table">
        <thead>
          <tr>
            <th>{t(column)}</th>
            <th className="usage-share-col">{t("insights.efficiency.tokensPerTurn")}</th>
            <th className="num">{t("insights.turns")}</th>
            <th className="num">{t("insights.efficiency.outputPerCall")}</th>
            <th className="num">{t("insights.efficiency.fromCache")}</th>
            {priced && <th className="num">{t("insights.efficiency.estCostPerTurn")}</th>}
            {priced && <th className="num">{t("insights.efficiency.savedByCache")}</th>}
          </tr>
        </thead>
        <tbody>
          {groups.map((g) => (
            <tr key={g.key}>
              <td>{g.key}</td>
              <Share value={g.tokensPerTurn} max={max} label={g.turns ? compact(g.tokensPerTurn) : "–"} />
              <td className="num">{g.turns ? compact(g.turns) : "–"}</td>
              <td className="num">{compact(g.outputPerCall)}</td>
              <td className="num">{percent(g.cacheShare)}</td>
              {priced && <td className="num">{g.priced && g.turns ? formatCost(g.costPerTurn) : "–"}</td>}
              {priced && <td className="num">{g.priced ? formatCost(g.saved) : "–"}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
