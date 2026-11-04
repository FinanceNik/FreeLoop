import { useEffect, useRef, useState, type DragEvent, type ReactNode } from "react";
import type { Session } from "./api.ts";
import { tabAfterKey } from "./a11y.ts";
import Limits from "./Limits.tsx";
import LiveUsage from "./LiveUsage.tsx";
import { basename } from "./paths.ts";
import { stableOrder, tabbed, USAGE_PANE, type WorkspaceState } from "./workspace.ts";

// The panes of the main window (GUI-017). The rules are in workspace.ts; this
// draws them. Every pane stays mounted, tabbed or not, so a hidden pane keeps
// its draft and scroll position and goes on streaming.

// What a dragged sidebar row carries.
export const SESSION_DRAG_TYPE = "application/x-freeloop-session";
// What a pane dragged by its header or tab carries: what it shows.
const PANE_DRAG_TYPE = "application/x-freeloop-pane";

interface Props {
  work: WorkspaceState;
  sessions: Session[];
  renderSession: (s: Session, active: boolean, onClose?: () => void, onPaneDrag?: (e: DragEvent) => void) => ReactNode;
  onActivate: (i: number) => void;
  onPlace: (sid: string, i: number) => void;
  // A pane was dragged onto another.
  onSwap: (from: number, to: number) => void;
  onClose: (i: number) => void;
  onNew: () => void;
  // Opens the full usage page.
  onUsagePage: () => void;
  onError: (err: unknown) => void;
  // Sessions that finished work the user has not seen (SES-022).
  unread: readonly string[];
  // What a session is doing, for a tab's tooltip (GUI-030).
  activityOf: (s: Session) => string;
  // How a session's status reads, and whether the panes are tabs just now.
  statusLabel: Record<string, string>;
  onTabbed: (tabbed: boolean) => void;
  // Names for the usage pane's filters.
  projects: readonly { id: string; name: string }[];
  providers: readonly { id: string; name: string }[];
  harnesses: readonly { id: string; name: string }[];
}

const MAX_OFFERED = 6;

export default function Workspace({ work, sessions, renderSession, onActivate, onPlace, onSwap, onClose, onNew, onUsagePage, onError, unread, activityOf, statusLabel, onTabbed, projects, providers, harnesses }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    if (!box.current) return;
    const seen = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    seen.observe(box.current);
    return () => seen.disconnect();
  }, []);
  const [dropOn, setDropOn] = useState<number | null>(null);
  const keys = work.panes.map((sid, i) => sid ?? `empty-${i}`);
  const rendered = useRef<string[]>([]);
  const order = (rendered.current = stableOrder(rendered.current, keys));

  const split = work.layout > 1;
  const asTabs = tabbed(work, width);
  useEffect(() => onTabbed(asTabs), [asTabs]);
  // A hidden pane still needs to be able to say it wants the user.
  const statusOf = (sid: string | null) => {
    const s = session(sid);
    if (!s) return null;
    return (
      <>
        {unread.includes(s.id) && <span className="unread-dot" role="img" aria-label="Finished, not read yet" />}
        {s.status !== "idle" && <span className={`badge badge-${s.status}`}>{statusLabel[s.status] ?? s.status}</span>}
      </>
    );
  };
  const session = (sid: string | null) => (sid ? sessions.find((s) => s.id === sid) : undefined);
  const titleOf = (sid: string | null) => (sid === USAGE_PANE ? "Usage" : (session(sid)?.title ?? "Empty"));
  // Sessions an empty pane offers: not shown yet, most recent first.
  const offered = sessions.filter((s) => !work.panes.includes(s.id) && s.status !== "archived" && !s.parentId).slice(0, MAX_OFFERED);

  // The pane being dragged: what is dragged can't be read until it is dropped.
  const draggedPane = useRef<number | null>(null);
  const startPaneDrag = (e: DragEvent, i: number) => {
    const from = e.target instanceof Element ? e.target : null;
    if (from?.closest("button:not(.pane-tab), summary, input, a, .export-options")) {
      e.preventDefault();
      return;
    }
    e.dataTransfer.setData(PANE_DRAG_TYPE, work.panes[i] ?? "");
    e.dataTransfer.effectAllowed = "move";
    draggedPane.current = i;
  };
  const endPaneDrag = () => {
    draggedPane.current = null;
    setDropOn(null);
  };

  const dragOver = (e: DragEvent, i: number) => {
    const types = e.dataTransfer.types;
    if (!types.includes(SESSION_DRAG_TYPE) && !types.includes(PANE_DRAG_TYPE)) return;
    e.preventDefault();
    if (draggedPane.current === i) {
      setDropOn(null);
      return;
    }
    e.dataTransfer.dropEffect = "move";
    setDropOn(i);
  };
  const drop = (e: DragEvent, i: number) => {
    setDropOn(null);
    if (e.dataTransfer.types.includes(PANE_DRAG_TYPE)) {
      e.preventDefault();
      onSwap(work.panes.indexOf(e.dataTransfer.getData(PANE_DRAG_TYPE)), i);
      return;
    }
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
              title={session(sid) ? activityOf(session(sid)!) || undefined : undefined}
              draggable={sid !== null}
              onDragStart={(e) => startPaneDrag(e, i)}
              onDragEnd={endPaneDrag}
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
              {titleOf(sid)} {statusOf(sid)}
            </button>
          ))}
        </div>
      )}
      <div className="panes" data-layout={asTabs ? 1 : work.layout}>
        {order.map((key) => {
          const i = keys.indexOf(key);
          const sid = work.panes[i];
          const active = i === work.active;
          const s = session(sid);
          const classes = ["pane", active && "pane-active", split && !asTabs && "pane-split", dropOn === i && "pane-drop"];
          return (
            <section
              key={key}
              className={classes.filter(Boolean).join(" ")}
              data-position={i}
              style={{ order: i }}
              onDragEnd={endPaneDrag}
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
              {sid === USAGE_PANE ? (
                <div className="pane-usage">
                  <div className="pane-usage-head" draggable={split && !asTabs} onDragStart={(e) => startPaneDrag(e, i)}>
                    <h2>Usage</h2>
                    <span className="spacer" />
                    <button className="link" onClick={onUsagePage}>
                      Full usage page
                    </button>
                    <button onClick={() => onClose(i)} aria-label="Close this pane" title="Close this pane">
                      ×
                    </button>
                  </div>
                  <Limits compact onError={onError} />
                  <LiveUsage compact projects={projects} providers={providers} harnesses={harnesses} onError={onError} />
                </div>
              ) : s ? (
                renderSession(s, active, split ? () => onClose(i) : undefined, split && !asTabs ? (e) => startPaneDrag(e, i) : undefined)
              ) : (
                <div className="empty pane-empty">
                  <p>{offered.length > 0 ? "Choose a session for this pane" : "Create a session for this pane"}</p>
                  {offered.length > 0 && (
                    <ul>
                      {offered.map((o) => (
                        <li key={o.id}>
                          <button className="pane-offer" onClick={() => onPlace(o.id, i)}>
                            {o.title} <span className="muted">{basename(o.projectRoot)}</span> {statusOf(o.id)}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                  <div className="row">
                    <button onClick={onNew}>New session</button>
                    {!work.panes.includes(USAGE_PANE) && <button onClick={() => onPlace(USAGE_PANE, i)}>Show usage</button>}
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
