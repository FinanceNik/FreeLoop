// @ mentions in the composer (GUI-004). The engine embeds the files a
// message mentions; this module finds the mention being typed and ranks the
// project's files for the picker.

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

// rankFiles returns the best limit matches for query.
export function rankFiles(query: string, files: readonly string[], limit = 8): string[] {
  const scored: { path: string; score: number }[] = [];
  for (const path of files) {
    const score = fuzzyScore(query, path);
    if (score !== null) scored.push({ path, score });
  }
  scored.sort((a, b) => b.score - a.score || a.path.localeCompare(b.path));
  return scored.slice(0, limit).map((s) => s.path);
}
