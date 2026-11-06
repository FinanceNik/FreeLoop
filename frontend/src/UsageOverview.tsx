import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { compact, withoutCacheReads, type Row } from "./analytics.ts";
import { duration, type ActivityRow } from "./activity.ts";
import { EngineService } from "./api.ts";
import { calendar, dayLevel, longestChat, modelName, monthLabels, OTHER_MODEL, usageOverview, type CalendarDay } from "./overview.ts";

// The overview at the top of the usage page (GUI-033): a year's totals and a calendar with
// one square per day, in the colour of the model used most that day and as strong as the
// day was big. One stop in the Tab order; the arrow keys walk the days, as in Heatmap.

const YEAR = 371; // the calendar's 53 weeks
const WEEKDAY_LABELS = ["Mon", "", "Wed", "", "Fri", "", ""];
const STEP = [32, 48, 64, 80, 100];

const dateLabel = (day: string) => new Date(`${day}T12:00:00`).toLocaleDateString("en", { weekday: "short", year: "numeric", month: "short", day: "numeric" });

// compact is for the usage pane (GUI-038): its header names the view, the grid opens on
// today, and the days as a table stay on the usage page.
export default function UsageOverview({ cacheReads, compact: inPane = false, onError }: { cacheReads: boolean; compact?: boolean; onError: (err: unknown) => void }) {
  const [rows, setRows] = useState<Row[] | null>(null);
  const [today, setToday] = useState("");
  const [turns, setTurns] = useState<ActivityRow[]>([]);
  const [titles, setTitles] = useState<Map<string, string>>(new Map());
  useEffect(() => {
    let live = true;
    EngineService.UsageReport(YEAR)
      .then((r) => {
        if (!live || !r) return;
        setRows((r.rows ?? []) as Row[]);
        setToday(r.until);
      })
      .catch(onError);
    // A long history is counted in steps: ask again until none is left.
    let timer: ReturnType<typeof setTimeout>;
    const load = () =>
      EngineService.ActivityReport(YEAR)
        .then((r) => {
          if (!live || !r) return;
          setTurns((r.rows ?? []) as ActivityRow[]);
          if (r.pending > 0) timer = setTimeout(load, 200);
        })
        .catch(onError);
    load();
    EngineService.ListSessions()
      .then((list) => live && setTitles(new Map((list ?? []).map((s) => [s.id, s.title]))))
      .catch(onError);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, []);

  const shown = useMemo(() => (rows ? (cacheReads ? rows : (withoutCacheReads(rows) as Row[])) : []), [rows, cacheReads]);
  const overview = useMemo(() => usageOverview(shown, today), [shown, today]);
  const weeks = useMemo(() => (today ? calendar(shown, today) : []), [shown, today]);
  const months = useMemo(() => monthLabels(weeks), [weeks]);
  const chat = useMemo(() => longestChat(turns), [turns]);
  const max = overview.peak?.tokens ?? 0;

  const colorOf = (model: string) => {
    const i = overview.models.indexOf(model);
    return i >= 0 ? `var(--series-${i + 1})` : "var(--series-other)";
  };
  const fill = (d: CalendarDay) => {
    if (d.tokens <= 0) return undefined;
    const pct = STEP[dayLevel(d.tokens, max) - 1];
    return `color-mix(in srgb, ${colorOf(d.models[0].model)} ${pct}%, var(--surface))`;
  };
  const folded = shown.some((r) => !overview.models.includes(modelName(r.model)) && r.inputTokens + r.outputTokens + r.cacheReadTokens + r.cacheWriteTokens > 0);

  const say = (d: CalendarDay) =>
    d.tokens > 0
      ? `${dateLabel(d.day)}: ${compact(d.tokens)} tokens, ${d.models
          .slice(0, 3)
          .map((m) => `${m.model} ${compact(m.tokens)}`)
          .join(", ")}${d.models.length > 3 ? `, and ${d.models.length - 3} more` : ""}`
      : `${dateLabel(d.day)}: no use`;

  const grid = useRef<HTMLDivElement>(null);
  // Today is at the right end: in a narrow pane, start there, and stay there while the
  // grid and the pane settle their sizes. Scrolling by hand changes neither.
  useEffect(() => {
    const box = grid.current;
    if (!inPane || !box) return;
    const toToday = () => {
      box.scrollLeft = box.scrollWidth;
    };
    toToday();
    const sizes = new ResizeObserver(toToday);
    sizes.observe(box);
    if (box.firstElementChild) sizes.observe(box.firstElementChild);
    return () => sizes.disconnect();
  }, [inPane, weeks.length]);
  const [cursor, setCursor] = useState<{ w: number; d: number } | null>(null);
  const [pointed, setPointed] = useState<CalendarDay | null>(null);
  // The newest days are what a narrow window should open on.
  useEffect(() => {
    if (grid.current) grid.current.scrollLeft = grid.current.scrollWidth;
  }, [weeks.length]);
  const lastCell = useMemo(() => {
    const w = weeks.length - 1;
    if (w < 0) return { w: 0, d: 0 };
    let d = 6;
    while (d > 0 && !weeks[w][d]) d--;
    return { w, d };
  }, [weeks]);
  const at = cursor ?? lastCell;
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const moves: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] };
    const move = moves[e.key];
    if (!move) return;
    e.preventDefault();
    const w = Math.min(weeks.length - 1, Math.max(0, at.w + move[0]));
    const d = Math.min(6, Math.max(0, at.d + move[1]));
    if (!weeks[w]?.[d]) return;
    setCursor({ w, d });
    e.currentTarget.querySelector<HTMLElement>(`[data-day="${w}-${d}"]`)?.focus();
  };

  if (rows === null) return null;
  const activeDays = weeks.flat().filter((d): d is CalendarDay => !!d && d.tokens > 0).reverse();
  const streak = (n: number) => `${n} ${n === 1 ? "day" : "days"}`;

  return (
    <div className={inPane ? "usage-overview usage-overview-compact" : "usage-overview"}>
      {!inPane && <h3>Last 12 months</h3>}
      <div className="usage-overview-tiles">
        <div>
          <strong>{compact(overview.total)}</strong>
          <span className="muted">Tokens{cacheReads ? "" : ", without cache reads"}</span>
        </div>
        <div>
          <strong>{overview.peak ? compact(overview.peak.tokens) : "None"}</strong>
          <span className="muted">Biggest day{overview.peak ? `, ${dateLabel(overview.peak.day)}` : ""}</span>
        </div>
        <div>
          <strong>{chat ? duration(chat.millis) : "None"}</strong>
          <span className="muted" title={chat ? titles.get(chat.sessionId) : undefined}>
            Longest chat, working{chat && titles.get(chat.sessionId) ? `: ${titles.get(chat.sessionId)}` : ""}
          </span>
        </div>
        <div>
          <strong>{streak(overview.currentStreak)}</strong>
          <span className="muted">Current streak</span>
        </div>
        <div>
          <strong>{streak(overview.longestStreak)}</strong>
          <span className="muted">Longest streak, of {streak(overview.activeDays)} active</span>
        </div>
      </div>
      <div ref={grid} className="usage-calendar-scroll">
        <div className="usage-calendar" role="group" aria-label="Tokens per day, last 12 months" onKeyDown={onKey} onPointerLeave={() => setPointed(null)}>
          <span />
          {weeks.map((_, w) => (
            <span key={w} className="usage-calendar-month">
              {months.find((m) => m.week === w)?.label ?? ""}
            </span>
          ))}
          {WEEKDAY_LABELS.map((label, d) => (
            <div key={d} style={{ display: "contents" }}>
              <span className="usage-calendar-weekday">{label}</span>
              {weeks.map((week, w) => {
                const cell = week[d];
                if (!cell) return <span key={w} />;
                return (
                  <div
                    key={w}
                    data-day={`${w}-${d}`}
                    className="usage-calendar-day"
                    role="img"
                    aria-label={say(cell)}
                    tabIndex={at.w === w && at.d === d ? 0 : -1}
                    style={{ background: fill(cell) }}
                    onPointerEnter={() => setPointed(cell)}
                    onFocus={() => {
                      setPointed(cell);
                      setCursor({ w, d });
                    }}
                    onBlur={() => setPointed(null)}
                  />
                );
              })}
            </div>
          ))}
        </div>
      </div>
      <p className="muted usage-calendar-readout" aria-hidden="true">
        {pointed ? say(pointed) : "Point at a day to see it. Its colour is the model used most, its strength how much was used."}
      </p>
      <div className="usage-calendar-legend">
        {overview.models.map((m) => (
          <span key={m} className="usage-calendar-model">
            <i style={{ background: colorOf(m) }} />
            {m}
          </span>
        ))}
        {folded && (
          <span className="usage-calendar-model">
            <i style={{ background: "var(--series-other)" }} />
            {OTHER_MODEL}
          </span>
        )}
      </div>
      {!inPane && activeDays.length > 0 && (
        <details className="usage-calendar-table">
          <summary className="muted">The days as a table</summary>
          <table className="usage">
            <thead>
              <tr>
                <th>Day</th>
                <th>Tokens</th>
                <th>Models</th>
              </tr>
            </thead>
            <tbody>
              {activeDays.map((d) => (
                <tr key={d.day}>
                  <td>{dateLabel(d.day)}</td>
                  <td>{d.tokens.toLocaleString()}</td>
                  <td>{d.models.map((m) => `${m.model} ${compact(m.tokens)}`).join(", ")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      )}
    </div>
  );
}
