// Which sessions finished work the user has not seen (SES-022). Pure, so
// unread.test.ts covers it. An idle row says nothing by design (GUI-026), so
// without this a session that just finished looks like one idle for days.

export const UNREAD_KEY = "freeloop.unread";

// A turn the user stopped, or one that failed and shows as an error, is not news.
const NOT_NEWS = new Set(["cancelled", "error"]);

export function finishedTurn(ev: { type: string; data?: unknown }): boolean {
  if (ev.type !== "turn_ended") return false;
  return !NOT_NEWS.has((ev.data as { stopReason?: string } | undefined)?.stopReason ?? "");
}

// onScreen is the sessions the user can see now: none when the window is not in
// front or another page covers the panes, one when the panes are tabs.
export function onScreen(opts: {
  panes: readonly (string | null)[];
  active: number;
  tabbed: boolean;
  showingPanes: boolean;
  focused: boolean;
}): string[] {
  if (!opts.focused || !opts.showingPanes) return [];
  const panes = opts.tabbed ? [opts.panes[opts.active]] : opts.panes;
  return panes.filter((p): p is string => !!p);
}

export function markFinished(unread: readonly string[], sid: string, visible: readonly string[]): readonly string[] {
  return visible.includes(sid) || unread.includes(sid) ? unread : [...unread, sid];
}

// markRead keeps the same array when nothing changes, so React need not render.
export function markRead(unread: readonly string[], seen: readonly string[]): readonly string[] {
  const left = unread.filter((sid) => !seen.includes(sid));
  return left.length === unread.length ? unread : left;
}

export function parseUnread(stored: string | null): string[] {
  try {
    const list = JSON.parse(stored ?? "[]");
    return Array.isArray(list) ? list.filter((x): x is string => typeof x === "string") : [];
  } catch {
    return [];
  }
}
