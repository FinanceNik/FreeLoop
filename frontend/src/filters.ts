// Sidebar filters and session tags (SES-004, SES-013). Pure, so
// filters.test.ts covers them.
import { t, type PlainKey } from "./i18n.ts";

export type SessionFilter = { query: string; status: string; tags: string[] };

export type SavedFilter = SessionFilter & { name: string };

type Filterable = { title: string; projectRoot: string; branch?: string; status: string; tags?: string[] | null };

export const MAX_TAGS = 16;
const MAX_TAG_LEN = 32;

// normalizeTags cleans tags the way the engine does: lower case, no leading
// #, words joined with dashes, at most 32 characters, sorted, no repeats.
export function normalizeTags(tags: string[]): string[] {
  const out: string[] = [];
  for (const raw of tags) {
    let tag = raw.trim().replace(/^#+/, "").toLowerCase().split(/\s+/).filter(Boolean).join("-");
    tag = Array.from(tag).slice(0, MAX_TAG_LEN).join("");
    if (tag && !out.includes(tag)) out.push(tag);
  }
  return out.sort();
}

// parseQuery splits the search box into its text and the #tags in it, so
// typing "#bug login" shows sessions tagged bug whose title has login.
export function parseQuery(query: string): { text: string; tags: string[] } {
  const words = query.trim().split(/\s+/).filter(Boolean);
  const tags = normalizeTags(words.filter((w) => w.startsWith("#") && w.length > 1));
  const text = words.filter((w) => !(w.startsWith("#") && w.length > 1)).join(" ");
  return { text, tags };
}

// matches reports whether a session passes a filter: its status, every tag
// asked for, and the query's text in its title, folder or branch.
export function matches(s: Filterable, f: SessionFilter): boolean {
  if (f.status && s.status !== f.status) return false;
  const { text, tags } = parseQuery(f.query);
  const have = s.tags ?? [];
  if (![...f.tags, ...tags].every((tag) => have.includes(tag))) return false;
  const q = text.toLowerCase();
  return !q || s.title.toLowerCase().includes(q) || s.projectRoot.toLowerCase().includes(q) || (s.branch ?? "").toLowerCase().includes(q);
}

// allTags lists the tags in use, the most used first.
export function allTags(sessions: { tags?: string[] | null }[]): string[] {
  const count = new Map<string, number>();
  for (const s of sessions) for (const tag of s.tags ?? []) count.set(tag, (count.get(tag) ?? 0) + 1);
  return [...count.keys()].sort((a, b) => count.get(b)! - count.get(a)! || a.localeCompare(b));
}

// isEmpty reports whether a filter shows everything.
export function isEmpty(f: SessionFilter): boolean {
  return !f.query.trim() && !f.status && f.tags.length === 0;
}

// sameFilter reports whether a saved filter is the one in use.
export function sameFilter(a: SessionFilter, b: SessionFilter): boolean {
  return a.query.trim() === b.query.trim() && a.status === b.status && normalizeTags(a.tags).join(" ") === normalizeTags(b.tags).join(" ");
}

// saveFilter adds a named filter, replacing one of the same name.
export function saveFilter(list: SavedFilter[], name: string, f: SessionFilter): SavedFilter[] {
  const n = name.trim();
  const entry: SavedFilter = { name: n, query: f.query.trim(), status: f.status, tags: normalizeTags(f.tags) };
  const i = list.findIndex((x) => x.name.toLowerCase() === n.toLowerCase());
  return i < 0 ? [...list, entry] : list.map((x, j) => (j === i ? entry : x));
}

// statusSummary counts sessions by status, most urgent first: what a collapsed
// project still shows, so nothing waiting on the user is hidden (GUI-001).
export function statusSummary(sessions: { status: string }[]): { status: string; count: number }[] {
  const order = ["waiting_permission", "waiting_input", "error", "running", "queued"];
  const rank = (status: string) => (order.includes(status) ? order.indexOf(status) : order.length);
  const counts = new Map<string, number>();
  for (const s of sessions) counts.set(s.status, (counts.get(s.status) ?? 0) + 1);
  return [...counts].map(([status, count]) => ({ status, count })).sort((a, b) => rank(a.status) - rank(b.status));
}

// dateGroups sorts sessions into calendar days counted back from now, by when
// each was last used (SES-021), most recent first and empty groups left out.
// Within a group the order moves by the minute, so that sessions streaming at
// the same time do not keep swapping places under the pointer. Each group has an
// id that stays the same in every language, and a label in the language in force.
export function dateGroups<T extends { id: string; updatedAt: unknown }>(sessions: T[], now: Date): { id: string; label: string; sessions: T[] }[] {
  const dayStart = (back: number) => new Date(now.getFullYear(), now.getMonth(), now.getDate() - back).getTime();
  const edges: [id: string, label: PlainKey, from: number][] = [
    ["today", "sidebar.date.today", dayStart(0)],
    ["yesterday", "sidebar.date.yesterday", dayStart(1)],
    ["week", "sidebar.date.week", dayStart(7)],
    ["month", "sidebar.date.month", dayStart(30)],
    ["older", "sidebar.date.older", -Infinity],
  ];
  const used = (s: T) => {
    const at = new Date(s.updatedAt as string).getTime();
    return Number.isNaN(at) ? -Infinity : at;
  };
  const minute = (s: T) => Math.floor(used(s) / 60000);
  const sorted = [...sessions].sort((a, b) => minute(b) - minute(a) || a.id.localeCompare(b.id));
  return edges
    .map(([id, label, from], i) => ({
      id,
      label: t(label),
      sessions: sorted.filter((s) => used(s) >= from && (i === 0 || used(s) < edges[i - 1][2])),
    }))
    .filter((g) => g.sessions.length > 0);
}
