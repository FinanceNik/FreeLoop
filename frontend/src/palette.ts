// The command palette's items and matching (GUI-014). This module has no
// runtime imports so `node --test` can run it.

export interface PaletteItem {
  id: string;
  title: string;
  // What kind of item it is, shown beside it: Session, Page, Action…
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
// order, so callers list what matters most first. Matches on the title beat
// matches on the keywords and group.
export function rank(items: readonly PaletteItem[], query: string, limit = 60): PaletteItem[] {
  if (!query.trim()) return items.slice(0, limit);
  const scored: { item: PaletteItem; s: number; i: number }[] = [];
  items.forEach((item, i) => {
    const s = Math.max(score(query, item.title) * 2, score(query, `${item.group} ${item.title} ${item.keywords ?? ""} ${item.detail ?? ""}`));
    if (s > 0) scored.push({ item, s, i });
  });
  scored.sort((a, b) => b.s - a.s || a.i - b.i);
  return scored.slice(0, limit).map((x) => x.item);
}
