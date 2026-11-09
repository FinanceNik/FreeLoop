import { HELD_BY } from "./transcript.ts";
import { useState } from "react";
import { decisionStats, duration, heldStats, efficiency, eventCount, forecast, heatLevel, heatmap, perDay, repeatApprovals, sessionStats, timeUnit, toolStats, turnStats, WEEKDAYS, type ActivityRow, type EffRow, type Efficiency } from "./activity.ts";
import { compact, days, weekStart, weeks } from "./analytics.ts";
import { EngineService } from "./api.ts";
import { formatCost } from "./cost.ts";
import Heatmap from "./Heatmap.tsx";
import UsageChart, { type Series } from "./UsageChart.tsx";
import { locale } from "./i18n.ts";

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
const percent = (share: number) => `${Math.round(share * 100)}%`;

const REASONS: Record<string, string> = {
  end_turn: "Finished",
  cancelled: "Stopped",
  max_tokens: "Hit the token limit",
  max_turn_requests: "Hit the step limit",
  refusal: "Refused by the model",
  error: "Ended on an error",
  unknown: "Not reported",
};
const reasonLabel = (r: string) => REASONS[r] ?? r.replace(/_/g, " ");

const ACTORS: Record<string, string> = {
  user: "You",
  rule: "A rule",
  mode: "The permission mode",
  sandbox: "The sandbox",
  hook: "A hook",
  policy: "Policy",
  protected_path: "Protected paths",
};
const actorLabel = (a: string) => ACTORS[a] ?? a.replace(/_/g, " ");

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
    bucketLabel: (d: string) => (weekly ? `Week of ${month(d)}` : month(d)),
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
    return <Empty note="Tools, turns and decisions are counted from every session's transcript.">{`No activity in the last ${rangeLabel}.`}</Empty>;
  }
  const cellsWithTurns = heat.cells.flatMap((row, d) => row.map((n, h) => ({ d, h, n }))).filter((c) => c.n > 0).sort((a, b) => b.n - a.n);
  return (
    <div className="usage-body">
      <div className="usage-figures">
        <div className="usage-hero">
          <span className="muted">Turns</span>
          <strong>{compact(turns.turns)}</strong>
          <span className="muted">{duration(turns.average)} each on average</span>
          <span className="muted">{duration(turns.millis)} in all</span>
        </div>
        <Tile label="Tool calls" value={compact(calls)} note={turns.turns ? `${compact(calls / turns.turns)} per turn` : undefined} />
        <Tile label="Tool calls that failed" value={calls ? percent(failures / calls) : "–"} note={`${compact(failures)} of them`} />
        <Tile label="Edits" value={compact(eventCount(rows, "edit"))} />
        <Tile label="Pull requests" value={compact(eventCount(rows, "pull_request"))} />
        <Tile label="Rewinds" value={compact(eventCount(rows, "rewind"))} />
        <Tile label="Compactions" value={compact(eventCount(rows, "compaction"))} />
        <Tile label="Errors" value={compact(eventCount(rows, "error"))} />
      </div>

      <div className="usage-card">
        <h3>Turns per {weekly ? "week" : "day"}</h3>
        <UsageChart data={chart} series={ONE("Turns")} format={compact} bucketLabel={bucketLabel} axisLabel={month} label={`Turns per ${weekly ? "week" : "day"}`} />
        <details className="usage-table-view">
          <summary>Show as a table</summary>
          <table className="usage-table">
            <thead>
              <tr>
                <th>{weekly ? "Week of" : "Day"}</th>
                <th className="num">Turns</th>
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
        <h3>When turns start</h3>
        <Heatmap data={heat} />
        <div className="usage-heat-legend" aria-hidden="true">
          <span>Fewer</span>
          {[0, 1, 2, 3, 4, 5].map((l) => (
            <i key={l} className={`usage-heat-cell heat-${l}`} />
          ))}
          <span>More</span>
        </div>
        <details className="usage-table-view">
          <summary>Show as a table</summary>
          <table className="usage-table">
            <thead>
              <tr>
                <th>Weekday</th>
                <th>Hour</th>
                <th className="num">Turns</th>
                <th className="num">Scale</th>
              </tr>
            </thead>
            <tbody>
              {cellsWithTurns.map((c) => (
                <tr key={`${c.d}-${c.h}`}>
                  <td>{WEEKDAYS[c.d]}</td>
                  <td>{String(c.h).padStart(2, "0")}:00</td>
                  <td className="num">{c.n}</td>
                  <td className="num">{heatLevel(c.n, heat.max)} of 5</td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      </div>

      <div className="usage-card">
        <h3>Tools</h3>
        <table className="usage-table">
          <thead>
            <tr>
              <th>Tool</th>
              <th className="usage-share-col">Share of calls</th>
              <th className="num">Calls</th>
              <th className="num">Failed</th>
              <th className="num">Failure rate</th>
              <th className="num">Average time</th>
              <th className="num">Total time</th>
            </tr>
          </thead>
          <tbody>
            {tools.map((t) => (
              <tr key={t.name}>
                <td>{t.name || "Unnamed"}</td>
                <Share value={t.calls} max={maxCalls} label={percent(calls ? t.calls / calls : 0)} />
                <td className="num">{compact(t.calls)}</td>
                <td className="num">{t.failures ? compact(t.failures) : "–"}</td>
                <td className="num">{t.failures ? percent(t.failureRate) : "–"}</td>
                <td className="num">{duration(t.average)}</td>
                <td className="num">{duration(t.millis)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="muted usage-note">A call you or a rule denied is not a failure, and time waiting for an approval is not counted.</p>
      </div>

      {turns.reasons.length > 0 && (
        <div className="usage-card">
          <h3>How turns ended</h3>
          <table className="usage-table">
            <thead>
              <tr>
                <th>Ending</th>
                <th className="usage-share-col">Share</th>
                <th className="num">Turns</th>
                <th className="num">Average length</th>
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
        <h3>Busiest sessions</h3>
        <table className="usage-table">
          <thead>
            <tr>
              <th>Session</th>
              <th className="num">Turns</th>
              <th className="num">Tool calls</th>
              <th className="num">Failed</th>
              <th className="num">Time in turns</th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((s) => {
              const r = rows.find((x) => x.sessionId === s.sessionId);
              return (
                <tr key={s.sessionId}>
                  <td>
                    {r?.deleted ? (
                      <span className="muted">Deleted session</span>
                    ) : (
                      <button className="link usage-pick" onClick={() => onOpenSession(s.sessionId)}>
                        {r?.title || "Untitled"}
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
    return <Empty note="Every request an agent makes to do something is decided by you, a rule, the permission mode or the sandbox.">{`No permission requests in the last ${rangeLabel}.`}</Empty>;
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
          <span className="muted">Agents waited on you</span>
          <strong>{duration(stats.waited)}</strong>
          <span className="muted">{duration(stats.averageWait)} per question on average</span>
        </div>
        <Tile label="Decisions" value={compact(stats.total)} />
        <Tile label="Asked you" value={compact(stats.byUser)} note={`${percent(stats.byUser / stats.total)} of decisions`} />
        <Tile label="Denied" value={compact(stats.denied)} note={`${percent(stats.denied / stats.total)} of decisions`} />
      </div>

      <div className="usage-card">
        <h3>Held by your policy</h3>
        {held.total === 0 ? (
          <p className="muted">Nothing in this range: no agent or repository was stopped from doing something.</p>
        ) : (
          <>
            <p className="muted">
              {compact(held.total)} {held.total === 1 ? "thing" : "things"} an agent or a repository would have gone ahead with. A session's Info tab lists them, each with a link to the moment.
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
                  <th>Session</th>
                  <th className="num">Held</th>
                </tr>
              </thead>
              <tbody>
                {held.sessions.slice(0, 10).map((s) => (
                  <tr key={s.sessionId}>
                    <td>
                      {s.deleted ? (
                        <span className="muted">{s.title || "Deleted session"}</span>
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
          <h3>Questions you keep answering yes to</h3>
          <table className="usage-table">
            <thead>
              <tr>
                <th>Command or file</th>
                <th>Tool</th>
                <th className="num">Times allowed</th>
                <th className="num">Time waited</th>
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
                      <button disabled={busy === id} aria-label={`Always allow ${r.command || r.path}`} onClick={() => allow(r.tool, r.command, r.path)}>
                        Always allow
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          <p className="muted usage-note">A rule covers exactly this command or file, in every project. Deny rules and protected paths still win.</p>
        </div>
      )}

      <div className="usage-card">
        <h3>Who decided</h3>
        <table className="usage-table">
          <thead>
            <tr>
              <th>Decided by</th>
              <th className="usage-share-col">Share</th>
              <th className="num">Decisions</th>
              <th className="num">Allowed</th>
              <th className="num">Denied</th>
              <th className="num">Cancelled</th>
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
          <h3>Time agents waited on you, per {weekly ? "week" : "day"}</h3>
          <UsageChart data={waits} series={ONE("Waited")} format={waitLabel} bucketLabel={bucketLabel} axisLabel={month} label={`Time agents waited on you per ${weekly ? "week" : "day"}`} />
          <details className="usage-table-view">
            <summary>Show as a table</summary>
            <table className="usage-table">
              <thead>
                <tr>
                  <th>{weekly ? "Week of" : "Day"}</th>
                  <th className="num">Waited</th>
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
    return <Empty note="Efficiency compares model calls with the turns they made up.">No model calls in this range.</Empty>;
  }
  const monthName = new Date(`${today}T12:00:00`).toLocaleDateString(locale(), { month: "long" });
  return (
    <div className="usage-body">
      <div className="usage-figures">
        <div className="usage-hero">
          <span className="muted">Tokens per turn</span>
          <strong>{turns ? compact(tokens / turns) : "–"}</strong>
          <span className="muted">
            {compact(tokens)} tokens in {compact(turns)} {Math.round(turns) === 1 ? "turn" : "turns"}
          </span>
        </div>
        {priced && <Tile label="Estimated cost per turn" value={turns ? formatCost(cost / turns) : "–"} />}
        {priced && <Tile label="Saved by the prompt cache" value={formatCost(saved)} note="Against the same reads at the input price" />}
        {pace && (priced ? <Tile label={`Projected for ${monthName}`} value={formatCost(pace.projected)} note={`${formatCost(pace.spent)} so far, at the last 14 days' pace`} /> : <Tile label={`Projected for ${monthName}`} value={compact(pace.projectedTokens)} note={`${compact(pace.tokens)} tokens so far, at the last 14 days' pace`} />)}
      </div>
      {!priced && (
        <p className="muted usage-note">
          Add prices to see the cost of a turn, what the cache saved and a projection in money.{" "}
          <button className="link" onClick={onOpenPrices}>
            Add prices
          </button>
        </p>
      )}
      <EfficiencyTable title="By model" head="Model" groups={models} priced={priced} />
      <EfficiencyTable title="By project" head="Project" groups={projects} priced={priced} />
      <p className="muted usage-note">A session's turns are shared among its models by their share of its model calls. Estimates depend on the prices in Settings.</p>
    </div>
  );
}

function EfficiencyTable({ title, head, groups, priced }: { title: string; head: string; groups: Efficiency[]; priced: boolean }) {
  const max = Math.max(0, ...groups.map((g) => g.tokensPerTurn));
  return (
    <div className="usage-card">
      <h3>{title}</h3>
      <table className="usage-table">
        <thead>
          <tr>
            <th>{head}</th>
            <th className="usage-share-col">Tokens per turn</th>
            <th className="num">Turns</th>
            <th className="num">Output per call</th>
            <th className="num">From the cache</th>
            {priced && <th className="num">Est. cost per turn</th>}
            {priced && <th className="num">Saved by the cache</th>}
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
