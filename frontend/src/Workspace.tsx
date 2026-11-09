import { useEffect, useRef, useState, type CSSProperties, type DragEvent, type ReactNode } from "react";
import type { Session } from "./api.ts";
import { tabAfterKey } from "./a11y.ts";
import { t } from "./i18n.ts";
import { basename } from "./paths.ts";
import Splitter from "./Splitter.tsx";
import SourceMark from "./SourceMark.tsx";
import UsagePane from "./UsagePane.tsx";
import { sourceColor, type Source } from "./source.ts";
import { clampSplit, rememberSplit, storedSplit } from "./sizes.ts";
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
  // What a session runs on: a mark on its tab, a line along its pane (GUI-034).
  sourceOf: (s: Session) => Source;
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

export default function Workspace({ work, sessions, renderSession, onActivate, onPlace, onSwap, onClose, onNew, onUsagePage, onError, unread, sourceOf, activityOf, statusLabel, onTabbed, projects, providers, harnesses }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const panesBox = useRef<HTMLDivElement>(null);
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
  // How much of the width the left column of panes takes (GUI-032).
  const [storedShare, setShare] = useState<number | null>(storedSplit);
  const columnsApart = split && !asTabs;
  const share = clampSplit(storedShare ?? 0.5, width);
  const dragSplit = (next: number) => {
    const clamped = clampSplit(next, width);
    setShare(clamped);
    rememberSplit(clamped);
  };
  // A hidden pane still needs to be able to say it wants the user.
  const statusOf = (sid: string | null) => {
    const s = session(sid);
    if (!s) return null;
    return (
      <>
        {unread.includes(s.id) && <span className="unread-dot" role="img" aria-label={t("status.unread")} />}
        {s.status !== "idle" && <span className={`badge badge-${s.status}`}>{statusLabel[s.status] ?? s.status}</span>}
      </>
    );
  };
  const session = (sid: string | null) => (sid ? sessions.find((s) => s.id === sid) : undefined);
  const titleOf = (sid: string | null) => (sid === USAGE_PANE ? t("app.usage") : (session(sid)?.title ?? t("workspace.empty")));
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
        <div className="pane-tabs" role="tablist" aria-label={t("workspace.panes")}>
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
              {session(sid) && <SourceMark source={sourceOf(session(sid)!)} detail={session(sid)!.model || undefined} />} {titleOf(sid)} {statusOf(sid)}
            </button>
          ))}
        </div>
      )}
      <div
        ref={panesBox}
        className="panes"
        data-layout={asTabs ? 1 : work.layout}
        style={columnsApart ? { gridTemplateColumns: `minmax(0, ${share}fr) minmax(0, ${1 - share}fr)` } : undefined}
      >
        {columnsApart && (
          <Splitter
            className="splitter-panes"
            label={t("workspace.width")}
            style={{ left: `${share * 100}%` }}
            value={share * 100}
            min={clampSplit(0, width) * 100}
            max={clampSplit(1, width) * 100}
            step={2}
            fromPointer={(x) => {
              const box = panesBox.current?.getBoundingClientRect();
              return box && box.width > 0 ? ((x - box.left) / box.width) * 100 : share * 100;
            }}
            onChange={(percent) => dragSplit(percent / 100)}
            onReset={() => {
              setShare(null);
              rememberSplit(null);
            }}
          />
        )}
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
              style={{ order: i, ...(s ? { "--pane-source": sourceColor(sourceOf(s).slot) } : {}) } as CSSProperties}
              onDragEnd={endPaneDrag}
              aria-label={split ? t("workspace.pane", { number: i + 1, title: titleOf(sid) }) : undefined}
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
                <UsagePane
                  draggable={split && !asTabs}
                  onDragStart={(e) => startPaneDrag(e, i)}
                  onUsagePage={onUsagePage}
                  onClose={() => onClose(i)}
                  projects={projects}
                  providers={providers}
                  harnesses={harnesses}
                  onError={onError}
                />
              ) : s ? (
                renderSession(s, active, split ? () => onClose(i) : undefined, split && !asTabs ? (e) => startPaneDrag(e, i) : undefined)
              ) : (
                <div className="empty pane-empty">
                  <p>{offered.length > 0 ? t("workspace.choose") : t("workspace.create")}</p>
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
                    <button onClick={onNew}>{t("app.newSession")}</button>
                    {!work.panes.includes(USAGE_PANE) && <button onClick={() => onPlace(USAGE_PANE, i)}>{t("workspace.showUsage")}</button>}
                    {split && <button onClick={() => onClose(i)}>{t("workspace.close")}</button>}
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
