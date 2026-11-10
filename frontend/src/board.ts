// The board view's columns (GUI-016). Pure, so board.test.ts covers it.
import { t, type PlainKey } from "./i18n.ts";

type Card = { status: string; updatedAt: string | Date };

export type Column<T> = { key: string; title: string; sessions: T[] };

// Each column takes these statuses; a session in error, or waiting for an
// answer, needs the user as much as one waiting for an approval.
const COLUMNS: { key: string; title: PlainKey; statuses: string[] }[] = [
  { key: "needs", title: "board.column.needs", statuses: ["waiting_permission", "waiting_input", "error"] },
  { key: "running", title: "status.running", statuses: ["running", "queued"] },
  { key: "idle", title: "status.idle", statuses: ["idle"] },
];

// columns sorts sessions into the board's columns, the most recently updated
// first, except that what needs the user has waited longest first, as in the
// attention view; archived sessions are not on the board. Titles are in the language
// in force.
export function columns<T extends Card>(sessions: T[]): Column<T>[] {
  const time = (s: Card) => new Date(s.updatedAt).getTime() || 0;
  return COLUMNS.map((c) => ({
    key: c.key,
    title: t(c.title),
    sessions: sessions.filter((s) => c.statuses.includes(s.status)).sort((a, b) => (c.key === "needs" ? time(a) - time(b) : time(b) - time(a))),
  }));
}

// since says how long ago a time was, in the largest whole unit.
export function since(at: string | Date, now: Date = new Date()): string {
  const s = Math.max(0, Math.round((now.getTime() - new Date(at).getTime()) / 1000));
  if (s < 60) return t("board.since.now");
  const m = Math.floor(s / 60);
  if (m < 60) return t("board.since.minutes", { minutes: m });
  const h = Math.floor(m / 60);
  if (h < 24) return t("board.since.hours", { hours: h });
  const d = Math.floor(h / 24);
  return d === 1 ? t("board.since.yesterday") : t("board.since.days", { days: d });
}
