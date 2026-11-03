// Rules of the session import (SES-020), apart from its view.

export interface ImportRow {
  id: string;
  title: string;
  folder: string;
  sessionId: string;
  missing: boolean;
}

// canImport reports a session that can be picked: not here yet, and its folder is there.
export function canImport(row: ImportRow): boolean {
  return !row.sessionId && !row.missing;
}

// matchesImport finds rows by words of their title or folder.
export function matchesImport(row: ImportRow, query: string): boolean {
  const hay = `${row.title} ${row.folder}`.toLowerCase();
  return query
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .every((word) => hay.includes(word));
}

// folderName is the last part of a path, for a list that has little room.
export function folderName(folder: string): string {
  return folder.split("/").filter(Boolean).pop() ?? folder;
}

// importResult says how an import went, in one line.
export function importResult(done: number, failed: number): string {
  const sessions = (n: number) => `${n} ${n === 1 ? "session" : "sessions"}`;
  if (failed === 0) return `Imported ${sessions(done)}.`;
  return `Imported ${sessions(done)}; ${failed} could not be imported.`;
}
