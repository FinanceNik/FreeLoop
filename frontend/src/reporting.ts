// How agents reported their usage (PRV-008), in words for the usage page and the shared
// summary: turns an agent gave no tokens for, which no cost can include, and agents
// counted by each turn's total rather than per model call. Pure, so reporting.test.ts
// covers it.

import { list, t } from "./i18n.ts";

export interface ReportedRow {
  sessionId: string;
  turnTotals?: number;
  unreported?: number;
}

export function reportingNotes<R extends ReportedRow>(rows: R[], agent: (r: R) => string): string[] {
  const silent = new Map<string, { turns: number; sessions: Set<string> }>();
  const totals = new Set<string>();
  for (const r of rows) {
    if ((r.unreported ?? 0) > 0) {
      const a = silent.get(agent(r)) ?? { turns: 0, sessions: new Set<string>() };
      a.turns += r.unreported ?? 0;
      a.sessions.add(r.sessionId);
      silent.set(agent(r), a);
    }
    if ((r.turnTotals ?? 0) > 0) totals.add(agent(r));
  }
  const notes: string[] = [];
  for (const [name, a] of [...silent].sort(([x], [y]) => x.localeCompare(y))) {
    notes.push(t("reporting.silent", { name, count: a.turns, turns: t("reporting.turns", { count: a.turns }), sessions: t("reporting.sessions", { count: a.sessions.size }) }));
  }
  if (totals.size > 0) {
    const names = [...totals].sort();
    notes.push(t("reporting.turnTotals", { names: list(names), count: names.length }));
  }
  return notes;
}

// heldNote says what the policy held in the summary's time, beside what it cost (SEC-014).
export function heldNote(held: number): string {
  return held > 0 ? t("reporting.held", { count: held }) : "";
}
