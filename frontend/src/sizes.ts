// The widths the user dragged the window's parts to (GUI-032). They belong to this
// viewer and this window's shape, so they stay in the page's storage, and every access
// is guarded: without it the parts simply keep their usual widths.

const SIDEBAR_KEY = "freeloop.sidebarWidth";
const PANEL_KEY = "freeloop.panelWidth";
const SPLIT_KEY = "freeloop.paneSplit";

// The conversation beside a panel keeps this much, as does a pane beside another.
const ROOM = 280;
const PANE_ROOM = 300;

export const SIDEBAR_MIN = 180;
export const PANEL_MIN = 260;

// clampSidebar keeps the sidebar readable, and from taking most of the window.
export function clampSidebar(px: number, windowWidth: number): number {
  return Math.round(Math.max(SIDEBAR_MIN, Math.min(px, Math.max(SIDEBAR_MIN, windowWidth / 2))));
}

// sidebarMax is the widest clampSidebar lets it be.
export function sidebarMax(windowWidth: number): number {
  return clampSidebar(Infinity, windowWidth);
}

// clampPanel leaves the conversation room beside the panel.
export function clampPanel(px: number, available: number): number {
  return Math.round(Math.max(PANEL_MIN, Math.min(px, Math.max(PANEL_MIN, available - ROOM))));
}

export function panelMax(available: number): number {
  return clampPanel(Infinity, available);
}

// clampSplit keeps both columns of panes usable; a split is the left one's share.
export function clampSplit(share: number, width: number): number {
  const least = width > 0 ? Math.min(0.5, PANE_ROOM / width) : 0.5;
  return Math.max(least, Math.min(share, 1 - least));
}

function stored(key: string): number | null {
  try {
    const raw = localStorage.getItem(key);
    const n = raw === null ? NaN : Number(raw);
    return Number.isFinite(n) && n > 0 ? n : null;
  } catch {
    return null;
  }
}

function remember(key: string, value: number | null) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, String(value));
  } catch {
    // Only a convenience: it is back to the usual width next time.
  }
}

export const storedSidebar = () => stored(SIDEBAR_KEY);
export const rememberSidebar = (px: number | null) => remember(SIDEBAR_KEY, px);
export const storedPanel = () => stored(PANEL_KEY);
export const storedSplit = () => {
  const share = stored(SPLIT_KEY);
  return share !== null && share < 1 ? share : null;
};
export const rememberSplit = (share: number | null) => remember(SPLIT_KEY, share);

// setPanelWidth applies a panel width to the whole window, since whichever pane is active
// shows the panel; null goes back to its usual width, which depends on the tab.
export function setPanelWidth(px: number | null) {
  if (px === null) document.documentElement.style.removeProperty("--panel-width");
  else document.documentElement.style.setProperty("--panel-width", `${px}px`);
  remember(PANEL_KEY, px);
}
