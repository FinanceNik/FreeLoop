import { useEffect, useRef, useState, type DragEvent, type ReactNode } from "react";
import type { Session } from "./api.ts";
import { tabAfterKey } from "./a11y.ts";
import { tabbed, type WorkspaceState } from "./workspace.ts";

// The panes of the main window (GUI-017). The rules are in workspace.ts; this
// draws them. Every pane stays mounted, tabbed or not, so a hidden pane keeps
// its draft and scroll position and goes on streaming.

// What a dragged sidebar row carries.
export const SESSION_DRAG_TYPE = "application/x-freeloop-session";

interface Props {
  work: WorkspaceState;
  sessions: Session[];
  renderSession: (s: Session, active: boolean, onClose?: () => void) => ReactNode;
  onActivate: (i: number) => void;
  onPlace: (sid: string, i: number) => void;
  onClose: (i: number) => void;
  onNew: () => void;
}

const MAX_OFFERED = 6;

export default function Workspace({ work, sessions, renderSession, onActivate, onPlace, onClose, onNew }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    if (!box.current) return;
    const seen = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    seen.observe(box.current);
    return () => seen.disconnect();
  }, []);
  const [dropOn, setDropOn] = useState<number | null>(null);

  const split = work.layout > 1;
  const asTabs = tabbed(work, width);
  const session = (sid: string | null) => (sid ? sessions.find((s) => s.id === sid) : undefined);
  const titleOf = (sid: string | null) => session(sid)?.title ?? "Empty";
  // Sessions an empty pane offers: not shown yet, most recent first.
  const offered = sessions.filter((s) => !work.panes.includes(s.id) && s.status !== "archived" && !s.parentId).slice(0, MAX_OFFERED);

  const dragOver = (e: DragEvent, i: number) => {
    if (!e.dataTransfer.types.includes(SESSION_DRAG_TYPE)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setDropOn(i);
  };
  const drop = (e: DragEvent, i: number) => {
    setDropOn(null);
    const sid = e.dataTransfer.getData(SESSION_DRAG_TYPE);
    if (!sid) return;
    e.preventDefault();
    onPlace(sid, i);
  };

  return (
    <div ref={box} className="workspace">
      {asTabs && (
        <div className="pane-tabs" role="tablist" aria-label="Panes">
          {work.panes.map((sid, i) => (
            <button
              key={sid ?? `empty-${i}`}
              role="tab"
              aria-selected={i === work.active}
              tabIndex={i === work.active ? 0 : -1}
              className={i === work.active ? "pane-tab pane-tab-on" : "pane-tab"}
              onClick={() => onActivate(i)}
              onKeyDown={(e) => {
                const next = tabAfterKey(e.key, i, work.panes.length);
                if (next < 0) return;
                e.preventDefault();
                onActivate(next);
              }}
              onDragOver={(e) => dragOver(e, i)}
              onDrop={(e) => drop(e, i)}
            >
              {titleOf(sid)}
            </button>
          ))}
        </div>
      )}
      <div className="panes" data-layout={asTabs ? 1 : work.layout}>
        {work.panes.map((sid, i) => {
          const active = i === work.active;
          const s = session(sid);
          const classes = ["pane", active && "pane-active", split && !asTabs && "pane-split", dropOn === i && "pane-drop"];
          return (
            <section
              key={sid ?? `empty-${i}`}
              className={classes.filter(Boolean).join(" ")}
              aria-label={split ? `Pane ${i + 1}: ${titleOf(sid)}` : undefined}
              hidden={asTabs && !active}
              onPointerDownCapture={() => onActivate(i)}
              onFocusCapture={() => onActivate(i)}
              onDragOver={(e) => dragOver(e, i)}
              onDragLeave={(e) => {
                if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDropOn(null);
              }}
              onDrop={(e) => drop(e, i)}
            >
              {s ? (
                renderSession(s, active, split ? () => onClose(i) : undefined)
              ) : (
                <div className="empty pane-empty">
                  <p>{offered.length > 0 ? "Choose a session for this pane" : "Create a session for this pane"}</p>
                  {offered.length > 0 && (
                    <ul>
                      {offered.map((o) => (
                        <li key={o.id}>
                          <button className="pane-offer" onClick={() => onPlace(o.id, i)}>
                            {o.title}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                  <div className="row">
                    <button onClick={onNew}>New session</button>
                    {split && <button onClick={() => onClose(i)}>Close this pane</button>}
                  </div>
                </div>
              )}
            </section>
          );
        })}
      </div>
    </div>
  );
}
