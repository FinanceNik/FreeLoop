// parseDiff turns git's unified diff for one file into rows to render, with
// old and new line numbers (GUI-005). This module has no runtime imports so
// `node --test` can run it directly.

export interface DiffRow {
  kind: "meta" | "hunk" | "add" | "del" | "ctx";
  text: string;
  oldNo?: number;
  newNo?: number;
}

const HUNK = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/;

export function parseDiff(text: string): DiffRow[] {
  const rows: DiffRow[] = [];
  let oldNo = 0;
  let newNo = 0;
  let inHunk = false;
  const lines = text === "" ? [] : text.replace(/\n$/, "").split("\n");
  for (const line of lines) {
    const h = HUNK.exec(line);
    if (h) {
      oldNo = Number(h[1]);
      newNo = Number(h[2]);
      inHunk = true;
      rows.push({ kind: "hunk", text: line });
      continue;
    }
    if (!inHunk || line.startsWith("\\")) {
      // Headers before the first hunk, and "\ No newline at end of file".
      if (line.startsWith("diff --git") && rows.length > 0) inHunk = false;
      rows.push({ kind: "meta", text: line });
      continue;
    }
    if (line.startsWith("+")) rows.push({ kind: "add", text: line.slice(1), newNo: newNo++ });
    else if (line.startsWith("-")) rows.push({ kind: "del", text: line.slice(1), oldNo: oldNo++ });
    else rows.push({ kind: "ctx", text: line.slice(1), oldNo: oldNo++, newNo: newNo++ });
  }
  return rows.filter((r) => r.kind !== "meta" || !/^(diff --git|index |--- |\+\+\+ )/.test(r.text));
}

// diffStats counts added and deleted lines.
export function diffStats(rows: readonly DiffRow[]): { added: number; deleted: number } {
  let added = 0;
  let deleted = 0;
  for (const r of rows) {
    if (r.kind === "add") added++;
    else if (r.kind === "del") deleted++;
  }
  return { added, deleted };
}
