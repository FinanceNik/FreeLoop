// The command palette's items and matching (GUI-014). This module's only
// runtime import is the translation module, so `node --test` can run it.
import { t, type PlainKey } from "./i18n.ts";

export interface PaletteItem {
  id: string;
  title: string;
  // What kind of item it is: the palette lists items under their group's
  // heading (Session, Page, Action…).
  group: string;
  // More words that find it, and a detail shown dimmed.
  keywords?: string;
  detail?: string;
  // A keyboard shortcut that does the same, shown as a hint.
  shortcut?: string;
  run: () => void;
}

// score rates how well query matches text, 0 for no match: the whole text,
// then its start, then the start of a word, then anywhere, then the
// query's characters in order (fuzzy).
export function score(query: string, text: string): number {
  const q = query.trim().toLowerCase();
  const t = text.toLowerCase();
  if (!q) return 1;
  if (t === q) return 100;
  if (t.startsWith(q)) return 80;
  const at = t.indexOf(q);
  if (at > 0 && /[^a-z0-9]/.test(t[at - 1])) return 60;
  if (at > 0) return 40;
  // Every word of the query at the start of some word, in any order.
  const words = q.split(/\s+/);
  if (words.length > 1 && words.every((w) => new RegExp(`(^|[^a-z0-9])${escapeRe(w)}`).test(t))) return 50;
  // Fuzzy: characters in order, rewarded for running together.
  let i = 0;
  let run = 0;
  let best = 0;
  for (const c of t) {
    if (c === q[i]) {
      i++;
      run++;
      best = Math.max(best, run);
      if (i === q.length) return 10 + Math.min(best, 9);
    } else {
      run = 0;
    }
  }
  return 0;
}

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// rank keeps the items matching query, best first; ties keep the given
// order, so callers list what matters most first. A title matches loosely,
// its characters in order; the group (its id and its heading in the language
// in force), keywords and detail only by a word or
// part of one, since a long description holds nearly any query's characters
// in order, which once filled the list with an agent's commands.
export function rank(items: readonly PaletteItem[], query: string, limit = 60): PaletteItem[] {
  if (!query.trim()) return items.slice(0, limit);
  const scored: { item: PaletteItem; s: number; i: number }[] = [];
  items.forEach((item, i) => {
    const rest = score(query, `${item.group} ${labelOf(item.group)} ${item.title} ${item.keywords ?? ""} ${item.detail ?? ""}`);
    const s = Math.max(score(query, item.title) * 2, rest >= 40 ? rest : 0);
    if (s > 0) scored.push({ item, s, i });
  });
  scored.sort((a, b) => b.s - a.s || a.i - b.i);
  return scored.slice(0, limit).map((x) => x.item);
}

// A section of the palette's list: items of one group under its heading.
export interface PaletteSection {
  group: string;
  label: string;
  items: PaletteItem[];
}

// What each group is called above its items, by the id App.tsx and the
// views give their items; a group not listed shows its id.
const LABELS: readonly { group: string; label: PlainKey }[] = [
  { group: "Approval", label: "palette.group.approval" },
  { group: "This session", label: "palette.group.thisSession" },
  { group: "Action", label: "palette.group.action" },
  { group: "New session", label: "palette.group.newSession" },
  { group: "Session", label: "palette.group.session" },
  { group: "Page", label: "palette.group.page" },
  { group: "Filter", label: "sidebar.filters" },
  { group: "Command", label: "palette.group.command" },
  { group: "Resource", label: "palette.group.resource" },
  { group: "Changes", label: "palette.group.changes" },
  { group: "Help", label: "app.help" },
];

function labelOf(group: string): string {
  const label = LABELS.find((l) => l.group === group)?.label;
  return label ? t(label) : group;
}

// The agent's commands, which a query starting with "/" searches alone.
const COMMANDS = { group: "Command" };

// Before anything is typed the palette shows a short list: what waits for
// the user, the app's actions, the latest sessions and where to go. The rest
// (this session's controls, filters, the agent's commands) is a search away.
const AT_REST: readonly { group: string; most: number; label?: PlainKey }[] = [
  { group: "Approval", most: Infinity },
  { group: "Action", most: Infinity },
  { group: "Session", most: 5, label: "palette.recentSessions" },
  { group: "Page", most: Infinity },
];

// How many of one group a search shows; the agent's commands, after a "/",
// show all.
export const PER_GROUP = 6;

// sections arranges what query finds under headings. A query starting with
// "/" searches the agent's commands only. Otherwise each group shows its best
// few, and the groups come in the order of their best match. The headings
// are in the language in force when it is called.
export function sections(items: readonly PaletteItem[], query: string): PaletteSection[] {
  const q = query.trim();
  if (!q) {
    return AT_REST.map(({ group, most, label }) => ({ group, label: label ? t(label) : labelOf(group), items: items.filter((i) => i.group === group).slice(0, most) })).filter(
      (s) => s.items.length > 0,
    );
  }
  if (q.startsWith("/")) {
    const found = rank(
      items.filter((i) => i.group === COMMANDS.group),
      q,
      500,
    );
    return found.length ? [{ group: COMMANDS.group, label: labelOf(COMMANDS.group), items: found }] : [];
  }
  const out: PaletteSection[] = [];
  for (const item of rank(items, q, 500)) {
    let s = out.find((x) => x.group === item.group);
    if (!s) out.push((s = { group: item.group, label: labelOf(item.group), items: [] }));
    if (s.items.length < PER_GROUP) s.items.push(item);
  }
  return out;
}
