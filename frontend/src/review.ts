// Review comments on the Diff tab (GUI-019): where a comment sits, how it
// is found again when the diff changes, and the one message a review
// becomes. Plain text, so every harness reads it the same way (HRN-001).
// No runtime imports, so `node --test` runs it directly.

import type { DiffRow } from "./diff.ts";

export interface ReviewComment {
  id: string;
  path: string;
  // The line the comment is on: its number in the new file, or in the old
  // one for a deleted line, and its text, to find it again.
  side: "new" | "old";
  line: number;
  text: string;
  // The diff around it, as shown when it was made: up to two rows each side.
  context: { kind: DiffRow["kind"]; text: string }[];
  // The index of the commented row within context.
  at: number;
  body: string;
}

const AROUND = 2;

// anchor describes the row at index i of a file's diff for a comment, or
// null for a row that is not a line of code.
export function anchor(rows: readonly DiffRow[], i: number): Pick<ReviewComment, "side" | "line" | "text" | "context" | "at"> | null {
  const r = rows[i];
  if (!r || (r.kind !== "add" && r.kind !== "del" && r.kind !== "ctx")) return null;
  const side = r.kind === "del" ? "old" : "new";
  const line = side === "old" ? r.oldNo! : r.newNo!;
  const code = (k: DiffRow["kind"]) => k === "add" || k === "del" || k === "ctx";
  let start = i;
  while (start > 0 && i - start < AROUND && code(rows[start - 1].kind)) start--;
  let end = i;
  while (end < rows.length - 1 && end - i < AROUND && code(rows[end + 1].kind)) end++;
  const context = rows.slice(start, end + 1).map((x) => ({ kind: x.kind, text: x.text }));
  return { side, line, text: r.text, context, at: i - start };
}

// locate finds a comment's row in a file's current diff: the row with its
// line number and text, else the row with its text nearest that number,
// else -1 (the line is gone).
export function locate(rows: readonly DiffRow[], c: Pick<ReviewComment, "side" | "line" | "text">): number {
  const no = (r: DiffRow) => (c.side === "old" ? (r.kind === "del" || r.kind === "ctx" ? r.oldNo : undefined) : r.kind === "add" || r.kind === "ctx" ? r.newNo : undefined);
  let best = -1;
  let dist = Infinity;
  rows.forEach((r, i) => {
    const n = no(r);
    if (n === undefined || r.text !== c.text) return;
    const d = Math.abs(n - c.line);
    if (d < dist) {
      best = i;
      dist = d;
    }
  });
  return best;
}

// fence returns a backtick fence longer than any run of backticks in text.
function fence(text: string): string {
  const longest = Math.max(2, ...[...text.matchAll(/`+/g)].map((m) => m[0].length));
  return "`".repeat(longest + 1);
}

// reviewMessage writes the comments as one message, grouped by file in the
// order the files were first commented on, by line within a file, each with
// its place and the lines around it.
export function reviewMessage(input: readonly ReviewComment[]): string {
  if (input.length === 0) return "";
  const files = [...new Set(input.map((c) => c.path))];
  const comments = [...input].sort((a, b) => files.indexOf(a.path) - files.indexOf(b.path) || a.line - b.line);
  const out = [comments.length === 1 ? "A review comment on your changes:" : `${comments.length} review comments on your changes:`];
  comments.forEach((c, i) => {
    const mark = (k: DiffRow["kind"]) => (k === "add" ? "+" : k === "del" ? "-" : " ");
    const block = c.context.map((x) => mark(x.kind) + x.text).join("\n");
    const f = fence(block);
    const where = `${c.path}:${c.line}${c.side === "old" ? " (a removed line)" : ""}`;
    out.push(`${i + 1}. ${where}\n${f}diff\n${block}\n${f}\n${c.body.trim()}`);
  });
  return out.join("\n\n") + "\n";
}
