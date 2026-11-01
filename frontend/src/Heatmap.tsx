import { useState, type KeyboardEvent } from "react";
import { heatLevel, WEEKDAYS, type Heatmap as Data } from "./activity.ts";

// Heatmap shows turns by weekday and hour (GUI-021): one hue, darker for more.
// One stop in the Tab order; the arrow keys walk the cells, and each says what it
// holds (NFR-A11Y-001).

const HOURS = Array.from({ length: 24 }, (_, h) => h);
const ROW = 18; // a row with its gap, as in app.css
const hour = (h: number) => `${String(h).padStart(2, "0")}:00`;

export default function Heatmap({ data }: { data: Data }) {
  const [cursor, setCursor] = useState({ d: 0, h: 9 });
  const [on, setOn] = useState<{ d: number; h: number } | null>(null);
  const turns = (n: number) => `${n} ${n === 1 ? "turn" : "turns"}`;
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const moves: Record<string, [number, number]> = { ArrowLeft: [0, -1], ArrowRight: [0, 1], ArrowUp: [-1, 0], ArrowDown: [1, 0] };
    const move = moves[e.key];
    if (!move) return;
    e.preventDefault();
    const d = Math.min(6, Math.max(0, cursor.d + move[0]));
    const h = Math.min(23, Math.max(0, cursor.h + move[1]));
    setCursor({ d, h });
    e.currentTarget.querySelector<HTMLElement>(`[data-cell="${d}-${h}"]`)?.focus();
  };
  return (
    <div className="usage-heat" role="group" aria-label="Turns by weekday and hour" onKeyDown={onKey} onPointerLeave={() => setOn(null)}>
      <span />
      {HOURS.map((h) => (
        <span key={h} className="usage-heat-hour">
          {h % 6 === 0 ? String(h).padStart(2, "0") : ""}
        </span>
      ))}
      {data.cells.map((row, d) => (
        <div key={d} style={{ display: "contents" }}>
          <span className="usage-heat-day">{WEEKDAYS[d]}</span>
          {row.map((n, h) => (
            <div
              key={h}
              data-cell={`${d}-${h}`}
              className={`usage-heat-cell heat-${heatLevel(n, data.max)}`}
              role="img"
              tabIndex={cursor.d === d && cursor.h === h ? 0 : -1}
              aria-label={`${WEEKDAYS[d]} ${hour(h)}: ${turns(n)}`}
              onPointerEnter={() => setOn({ d, h })}
              onFocus={() => {
                setOn({ d, h });
                setCursor({ d, h });
              }}
              onBlur={() => setOn(null)}
            />
          ))}
        </div>
      ))}
      {on && (
        <div
          className="usage-tip"
          aria-hidden="true"
          style={{
            left: `clamp(90px, calc(2.6rem + (100% - 2.6rem) * ${(on.h + 0.5) / 24}), calc(100% - 90px))`,
            top: on.d < 4 ? ROW * (on.d + 2) + 4 : ROW * on.d - 40,
          }}
        >
          <strong>{turns(data.cells[on.d][on.h])}</strong> <span className="muted">{WEEKDAYS[on.d]} {hour(on.h)}</span>
        </div>
      )}
    </div>
  );
}
