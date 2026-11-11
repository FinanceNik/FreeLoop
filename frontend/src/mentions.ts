// @ mentions in the composer (GUI-004). The engine embeds the files a
// message mentions; this module finds the mention being typed and ranks the
// project's files for the picker.

import { relative } from "./paths.ts";

export interface Mention {
  start: number; // index of the @
  query: string;
}

// mentionAt returns the mention the caret is in: an @ at the start of the
// text or after whitespace, followed by no whitespace up to the caret.
export function mentionAt(text: string, caret: number): Mention | null {
  for (let i = caret - 1; i >= 0; i--) {
    const c = text[i];
    if (c === "@") {
      if (i > 0 && !/\s/.test(text[i - 1])) return null;
      return { start: i, query: text.slice(i + 1, caret) };
    }
    if (/\s/.test(c) || c === '"') return null;
  }
  return null;
}

// insertMention replaces the mention being typed with path, quoted when it
// holds spaces, and puts the caret after it.
export function insertMention(text: string, m: Mention, caret: number, path: string): { text: string; caret: number } {
  const token = /\s/.test(path) ? `@"${path}"` : `@${path}`;
  const rest = text.slice(caret);
  const sep = rest.length > 0 && /^\s/.test(rest) ? "" : " ";
  const before = text.slice(0, m.start) + token + sep;
  return { text: before + rest, caret: before.length };
}

// fuzzyScore scores path for query, or returns null when query's characters
// do not all appear in order. Matches in the file name, at word starts and
// in runs score higher; shorter paths win ties.
export function fuzzyScore(query: string, path: string): number | null {
  const q = query.toLowerCase();
  const p = path.toLowerCase();
  if (!q) return -path.length * 0.05;
  const base = Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\")) + 1;
  let score = 0;
  let from = 0;
  let prev = -2;
  for (const ch of q) {
    const i = p.indexOf(ch, from);
    if (i < 0) return null;
    if (i === prev + 1) score += 5;
    if (i === 0 || "/\\._- ".includes(p[i - 1])) score += 3;
    if (i >= base) score += 2;
    prev = i;
    from = i + 1;
  }
  if (p.slice(base).startsWith(q)) score += 15;
  else if (p.slice(base).includes(q)) score += 10;
  else if (p.includes(q)) score += 6;
  return score - path.length * 0.05;
}

// rankFiles returns the best limit matches for query. A boost (GUI-049, from
// sessionFiles) is added to a file's score, so the session's own files win a close
// call but not a clearly better match; with less than two characters typed, the
// boosted files come first, in the boost's order.
export function rankFiles(query: string, files: readonly string[], limit = 8, boost?: ReadonlyMap<string, number>): string[] {
  const scored: { path: string; score: number }[] = [];
  for (const path of files) {
    const score = fuzzyScore(query, path);
    if (score === null) continue;
    const extra = boost?.get(path) ?? 0;
    scored.push({ path, score: query.length < 2 && extra > 0 ? 1000 + extra : score + extra });
  }
  scored.sort((a, b) => b.score - a.score || a.path.localeCompare(b.path));
  return scored.slice(0, limit).map((s) => s.path);
}

// The bonus for a file the session touched (the most recent a little more) and for
// one git reports changed.
export const TOUCHED_BOOST = 4;
export const CHANGED_BOOST = 2;

// sessionFiles is what the @ picker ranks (GUI-049): the project's files, plus files
// git reports that the list does not have yet (one just created), and the boost for
// the session's own. touched are paths the session's tools and messages named, the
// most recent first, as written: absolute, or relative to the root. One counts only
// if it is in the list or git's changes, which leaves out paths outside the project,
// deleted files and other spellings.
export function sessionFiles(
  files: readonly string[],
  touched: readonly string[],
  root: string,
  changed: readonly { path: string; status: string }[] = [],
): { files: string[]; boost: Map<string, number> } {
  const present = changed.filter((c) => c.status !== "deleted").map((c) => c.path);
  const all = new Set(files);
  const list = [...files, ...present.filter((p) => !all.has(p))];
  present.forEach((p) => all.add(p));
  const boost = new Map<string, number>();
  for (const p of present) boost.set(p, CHANGED_BOOST);
  const own = [...new Set(touched.map((p) => relative(p, root)).filter((p) => all.has(p)))];
  own.forEach((p, i) => boost.set(p, TOUCHED_BOOST + (own.length - i) / (own.length + 1)));
  return { files: list, boost };
}
