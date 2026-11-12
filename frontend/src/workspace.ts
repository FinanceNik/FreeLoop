// The workspace (GUI-017): up to four sessions side by side in the main
// window, one per pane, one of them active. These functions hold the rules;
// the view only renders them. A session is in at most one pane.

export type Layout = 1 | 2 | 3 | 4;

export interface WorkspaceState {
  layout: Layout;
  // One entry per pane: a session ID, or null for an empty pane.
  panes: (string | null)[];
  active: number;
}

// USAGE_PANE in a pane's place shows live usage there (GUI-023). Session IDs start
// with ses_, so it can't be mistaken for one.
export const USAGE_PANE = "usage";

// isSession reports a pane showing a session, not nothing or usage.
export function isSession(pane: string | null): pane is string {
  return pane !== null && pane !== USAGE_PANE;
}

export const SINGLE: WorkspaceState = { layout: 1, panes: [null], active: 0 };

// The narrowest a pane may get before the panes become tabs.
export const PANE_MIN_WIDTH = 420;

// columns is how many panes stand side by side in a layout.
export function columns(layout: Layout): number {
  return layout === 1 ? 1 : 2;
}

function clamp(n: number, max: number): number {
  return Math.max(0, Math.min(n, max - 1));
}

// setLayout changes the number of panes. Growing adds empty panes; shrinking
// keeps the active pane's session first, then the others in order.
export function setLayout(w: WorkspaceState, layout: Layout): WorkspaceState {
  if (layout === w.layout) return w;
  if (layout > w.layout) {
    return { layout, panes: [...w.panes, ...Array(layout - w.panes.length).fill(null)], active: w.active };
  }
  const active = w.panes[w.active];
  const others = w.panes.filter((s, i) => s !== null && i !== w.active);
  const kept = [...(active ? [active] : []), ...others].slice(0, layout);
  const panes = [...kept, ...Array(layout - kept.length).fill(null)];
  return { layout, panes, active: 0 };
}

// chooseLayout is the layout control: a pane added this way is empty and
// becomes active, so the next session opened goes into it.
export function chooseLayout(w: WorkspaceState, layout: Layout): WorkspaceState {
  const next = setLayout(w, layout);
  if (layout <= w.layout) return next;
  const empty = next.panes.indexOf(null);
  return empty >= 0 ? { ...next, active: empty } : next;
}

// openIn shows a session: where it already is, else in the active pane
// ("active"), or in the first empty pane, adding one when all are full and
// there are fewer than four ("new").
export function openIn(w: WorkspaceState, sid: string, where: "active" | "new"): WorkspaceState {
  const at = w.panes.indexOf(sid);
  if (at >= 0) return { ...w, active: at };
  if (where === "active") {
    const panes = [...w.panes];
    panes[w.active] = sid;
    return { ...w, panes };
  }
  const empty = w.panes.indexOf(null);
  if (empty >= 0) return placeIn(w, sid, empty);
  if (w.layout < 4) {
    const grown = setLayout(w, (w.layout + 1) as Layout);
    return placeIn(grown, sid, grown.panes.length - 1);
  }
  return openIn(w, sid, "active");
}

// placeIn puts a session in pane i (dropped there, or opened from it) and
// makes it active; a pane that showed the session before is emptied.
export function placeIn(w: WorkspaceState, sid: string, i: number): WorkspaceState {
  if (i < 0 || i >= w.panes.length) return w;
  const panes = w.panes.map((s) => (s === sid ? null : s));
  panes[i] = sid;
  return { ...w, panes, active: i };
}

// swapPanes trades what two panes show, a pane dragged onto another; what was
// dragged stays active. An empty pane can be swapped with too.
export function swapPanes(w: WorkspaceState, from: number, to: number): WorkspaceState {
  if (from === to || from < 0 || to < 0 || from >= w.panes.length || to >= w.panes.length) return w;
  const panes = [...w.panes];
  [panes[from], panes[to]] = [panes[to], panes[from]];
  return { ...w, panes, active: to };
}

// stableOrder keeps panes in the order they first appeared. A pane's place on
// screen comes from CSS, so a swap moves no element: moving one would lose its
// scroll position and reload a preview.
export function stableOrder(previous: readonly string[], keys: readonly string[]): string[] {
  return [...previous.filter((k) => keys.includes(k)), ...keys.filter((k) => !previous.includes(k))];
}

// closePane removes pane i; the last pane stays and is emptied.
export function closePane(w: WorkspaceState, i: number): WorkspaceState {
  if (i < 0 || i >= w.panes.length) return w;
  if (w.layout === 1) return { ...w, panes: [null] };
  const panes = w.panes.filter((_, j) => j !== i);
  const active = w.active > i ? w.active - 1 : w.active === i ? Math.min(i, panes.length - 1) : w.active;
  return { layout: (w.layout - 1) as Layout, panes, active };
}

export function focusPane(w: WorkspaceState, i: number): WorkspaceState {
  if (i < 0 || i >= w.panes.length || i === w.active) return w;
  return { ...w, active: i };
}

// cycle moves the active pane by step, wrapping around.
export function cycle(w: WorkspaceState, step: number): WorkspaceState {
  const n = w.panes.length;
  return { ...w, active: (((w.active + step) % n) + n) % n };
}

export type Direction = "left" | "right" | "up" | "down";

// Which pane is next in each direction, by pane, as the layouts stand on screen: two
// side by side, one full height and two stacked, or a grid. It wraps around, as tmux
// does.
const NEXT: Record<Exclude<Layout, 1>, Record<Direction, number[]>> = {
  2: { left: [1, 0], right: [1, 0], up: [0, 1], down: [0, 1] },
  3: { left: [1, 0, 0], right: [1, 0, 0], up: [0, 2, 1], down: [0, 2, 1] },
  4: { left: [1, 0, 3, 2], right: [1, 0, 3, 2], up: [2, 3, 0, 1], down: [2, 3, 0, 1] },
};

// movePane makes the pane in that direction active. Tabs stand in a row, so there
// only left and right move.
export function movePane(w: WorkspaceState, direction: Direction, tabs: boolean): WorkspaceState {
  if (w.layout === 1) return w;
  if (tabs) return direction === "left" || direction === "right" ? cycle(w, direction === "right" ? 1 : -1) : w;
  return focusPane(w, NEXT[w.layout][direction][w.active]);
}

// forget empties the panes of sessions that no longer exist or are
// archived, after a delete or at launch.
export function forget(w: WorkspaceState, exists: (sid: string) => boolean): WorkspaceState {
  const keep = (s: string) => s === USAGE_PANE || exists(s);
  if (w.panes.every((s) => s === null || keep(s))) return w;
  return { ...w, panes: w.panes.map((s) => (s !== null && keep(s) ? s : null)) };
}

// restore checks a saved workspace before it is used.
export function restore(saved: unknown): WorkspaceState {
  const v = saved as Partial<WorkspaceState> | null;
  if (!v || ![1, 2, 3, 4].includes(v.layout as number) || !Array.isArray(v.panes)) return SINGLE;
  const layout = v.layout as Layout;
  const seen = new Set<string>();
  const panes = Array.from({ length: layout }, (_, i) => {
    const s = v.panes![i];
    if (typeof s !== "string" || !s || seen.has(s)) return null;
    seen.add(s);
    return s;
  });
  return { layout, panes, active: clamp(Number(v.active) || 0, layout) };
}

// tabbed reports whether the panes must become tabs in a workspace this wide.
export function tabbed(w: WorkspaceState, width: number): boolean {
  return w.layout > 1 && width > 0 && width < columns(w.layout) * PANE_MIN_WIDTH;
}

// withoutFailed is the workspace as it is saved: a pane whose view failed to draw is
// left empty, so the window does not open on it again (GUI-017).
export function withoutFailed(w: WorkspaceState, failed: ReadonlySet<string>): WorkspaceState {
  if (!w.panes.some((s) => s !== null && failed.has(s))) return w;
  return { ...w, panes: w.panes.map((s) => (s !== null && failed.has(s) ? null : s)) };
}
