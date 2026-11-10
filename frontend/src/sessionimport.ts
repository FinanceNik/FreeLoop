// Rules of the session import (SES-020), apart from its view.
import { t } from "./i18n.ts";

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
  return folder.split(/[\\/]/).filter(Boolean).pop() ?? folder;
}

// importResult says how an import went, in one line.
export function importResult(done: number, failed: number): string {
  if (failed === 0) return t("sessionImport.result", { count: done });
  return t("sessionImport.resultFailed", { sessions: t("sessionImport.sessions", { count: done }), count: failed });
}
