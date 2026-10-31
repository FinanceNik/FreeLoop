// Sidebar filters and session tags (SES-004, SES-013). Pure, so
// filters.test.ts covers them.

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
    let t = raw.trim().replace(/^#+/, "").toLowerCase().split(/\s+/).filter(Boolean).join("-");
    t = Array.from(t).slice(0, MAX_TAG_LEN).join("");
    if (t && !out.includes(t)) out.push(t);
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
  if (![...f.tags, ...tags].every((t) => have.includes(t))) return false;
  const q = text.toLowerCase();
  return !q || s.title.toLowerCase().includes(q) || s.projectRoot.toLowerCase().includes(q) || (s.branch ?? "").toLowerCase().includes(q);
}

// allTags lists the tags in use, the most used first.
export function allTags(sessions: { tags?: string[] | null }[]): string[] {
  const count = new Map<string, number>();
  for (const s of sessions) for (const t of s.tags ?? []) count.set(t, (count.get(t) ?? 0) + 1);
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
