// The board view's columns (GUI-016). Pure, so board.test.ts covers it.

type Card = { status: string; updatedAt: string | Date };

export type Column<T> = { key: string; title: string; sessions: T[] };

// Each column takes these statuses; a session in error, or waiting for an
// answer, needs the user as much as one waiting for an approval.
const COLUMNS: { key: string; title: string; statuses: string[] }[] = [
  { key: "needs", title: "Needs you", statuses: ["waiting_permission", "waiting_input", "error"] },
  { key: "running", title: "Running", statuses: ["running", "queued"] },
  { key: "idle", title: "Idle", statuses: ["idle"] },
];

// columns sorts sessions into the board's columns, the most recently
// updated first in each; archived sessions are not on the board.
export function columns<T extends Card>(sessions: T[]): Column<T>[] {
  const time = (s: Card) => new Date(s.updatedAt).getTime() || 0;
  return COLUMNS.map((c) => ({
    key: c.key,
    title: c.title,
    sessions: sessions.filter((s) => c.statuses.includes(s.status)).sort((a, b) => time(b) - time(a)),
  }));
}

// since says how long ago a time was, in the largest whole unit.
export function since(t: string | Date, now: Date = new Date()): string {
  const s = Math.max(0, Math.round((now.getTime() - new Date(t).getTime()) / 1000));
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} h ago`;
  const d = Math.floor(h / 24);
  return d === 1 ? "yesterday" : `${d} days ago`;
}
