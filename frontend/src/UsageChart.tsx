import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { tabAfterKey } from "./a11y.ts";
import { niceTicks, type Day } from "./analytics.ts";

// UsageChart draws usage over time as stacked columns, one per day or week
// (PRV-008). Each column is its own hover and focus target, and its readout
// lists every series; the dashboard's tables carry the same numbers.

export interface Series {
  key: string;
  label: string;
  color: string; // a CSS colour, normally var(--series-n)
}

interface Props {
  data: Day[];
  series: Series[];
  format: (v: number) => string;
  bucketLabel: (day: string) => string;
  // A shorter label for the axis, when bucketLabel is long.
  axisLabel?: (day: string) => string;
  label: string; // what the chart shows, for screen readers
  // Take the height of the space it is given, in a pane, rather than its own.
  fill?: boolean;
}

const DEFAULT_HEIGHT = 220;
const MIN_HEIGHT = 120;
const PAD = { top: 10, right: 8, bottom: 26, left: 48 };
const GAP = 2; // the surface gap between stacked segments
const MAX_BAR = 24;

export default function UsageChart({ data, series, format, bucketLabel, axisLabel = bucketLabel, label, fill = false }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(640);
  const [height, setHeight] = useState(DEFAULT_HEIGHT);
  const [hover, setHover] = useState<number | null>(null);
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      setWidth(Math.max(280, el.clientWidth));
      if (fill) setHeight(Math.max(MIN_HEIGHT, el.clientHeight));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [fill]);

  const max = Math.max(0, ...data.map((d) => d.total));
  const ticks = niceTicks(max);
  const top = ticks[ticks.length - 1] || 1;
  const plotW = width - PAD.left - PAD.right;
  const plotH = height - PAD.top - PAD.bottom;
  const slot = plotW / Math.max(1, data.length);
  const bar = Math.max(2, Math.min(MAX_BAR, slot * 0.68));
  const y = (v: number) => PAD.top + plotH - (v / top) * plotH;
  // Show a date label every so many columns, so they never collide.
  const every = Math.max(1, Math.ceil(data.length / Math.max(1, Math.floor(plotW / 72))));

  const columns = data.map((d, i) => {
    const x = PAD.left + i * slot + (slot - bar) / 2;
    let base = PAD.top + plotH;
    const parts: { key: string; y: number; h: number; color: string }[] = [];
    d.values.forEach((v, s) => {
      if (v <= 0) return;
      const h = (v / top) * plotH;
      parts.push({ key: series[s]?.key ?? String(s), y: base - h, h, color: series[s]?.color ?? "var(--series-other)" });
      base -= h;
    });
    return { x, parts, d };
  });

  // One stop in the Tab order, however many days are shown; the arrow keys
  // walk the columns (NFR-A11Y-001).
  const [cursor, setCursor] = useState(Number.MAX_SAFE_INTEGER);
  const onKey = (e: KeyboardEvent<SVGSVGElement>) => {
    const at = data.findIndex((_, i) => i === Math.min(cursor, data.length - 1));
    const next = tabAfterKey(e.key, at, data.length);
    if (next < 0) return;
    e.preventDefault();
    e.currentTarget.querySelectorAll<SVGRectElement>(".usage-hit")[next]?.focus();
  };
  const tip = hover !== null ? data[hover] : null;
  const tipLeft = hover !== null ? Math.min(Math.max(PAD.left + hover * slot + slot / 2, 90), width - 90) : 0;

  return (
    <div className={fill ? "usage-chart usage-chart-fill" : "usage-chart"} ref={box}>
      <svg width={width} height={height} role="group" aria-label={label} onPointerLeave={() => setHover(null)} onKeyDown={onKey}>
        {ticks.map((t) => (
          <g key={t}>
            <line className="usage-grid" x1={PAD.left} x2={width - PAD.right} y1={y(t)} y2={y(t)} />
            <text className="usage-axis" x={PAD.left - 8} y={y(t)} dy="0.32em" textAnchor="end">
              {format(t)}
            </text>
          </g>
        ))}
        {columns.map(({ x, parts, d }, i) => (
          <g key={d.day} className={hover === i ? "usage-col usage-col-on" : "usage-col"}>
            {parts.map((p, j) => {
              // The top segment gets the rounded data-end; each segment
              // leaves a surface gap below the next.
              const isTop = j === parts.length - 1;
              const h = Math.max(1, p.h - (isTop ? 0 : GAP));
              const r = isTop ? Math.min(4, h / 2, bar / 2) : 0;
              return <path key={p.key} d={column(x, p.y + (isTop ? 0 : GAP), bar, h, r)} fill={p.color} />;
            })}
            {(data.length - 1 - i) % every === 0 && (
              <text className="usage-axis" x={x + bar / 2} y={height - 8} textAnchor="middle">
                {axisLabel(d.day)}
              </text>
            )}
            <rect
              className="usage-hit"
              x={PAD.left + i * slot}
              y={PAD.top}
              width={slot}
              height={plotH}
              role="img"
              tabIndex={i === Math.min(cursor, data.length - 1) ? 0 : -1}
              aria-label={`${bucketLabel(d.day)}: ${format(d.total)}${d.values.map((v, s) => (v > 0 ? `, ${series[s]?.label} ${format(v)}` : "")).join("")}`}
              onPointerMove={() => setHover(i)}
              onMouseMove={() => setHover(i)}
              onFocus={() => {
                setHover(i);
                setCursor(i);
              }}
              onBlur={() => setHover(null)}
            />
          </g>
        ))}
        <line className="usage-baseline" x1={PAD.left} x2={width - PAD.right} y1={PAD.top + plotH} y2={PAD.top + plotH} />
      </svg>
      {tip && (
        <div className="usage-tip" style={{ left: tipLeft }} aria-hidden="true">
          <div className="usage-tip-head">
            <strong>{format(tip.total)}</strong> <span className="muted">{bucketLabel(tip.day)}</span>
          </div>
          {tip.values.map((v, s) =>
            v > 0 ? (
              <div key={series[s]?.key ?? s} className="usage-tip-row">
                <span className="usage-key-line" style={{ background: series[s]?.color }} />
                <strong>{format(v)}</strong> <span className="muted">{series[s]?.label}</span>
              </div>
            ) : null,
          )}
          {tip.total === 0 && <div className="muted">No use</div>}
        </div>
      )}
    </div>
  );
}

// column is a bar from (x, y) of width w and height h whose top corners are
// rounded by r and whose base is square.
function column(x: number, y: number, w: number, h: number, r: number): string {
  if (r <= 0) return `M${x},${y}h${w}v${h}h${-w}Z`;
  return `M${x},${y + h}V${y + r}Q${x},${y} ${x + r},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + h}Z`;
}
