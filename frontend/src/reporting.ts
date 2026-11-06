// How agents reported their usage (PRV-008), in words for the usage page and the shared
// summary: turns an agent gave no tokens for, which no cost can include, and agents
// counted by each turn's total rather than per model call. Pure, so reporting.test.ts
// covers it.

export interface ReportedRow {
  sessionId: string;
  turnTotals?: number;
  unreported?: number;
}

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

function list(names: string[]): string {
  return names.length <= 1 ? (names[0] ?? "") : `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
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
    notes.push(`${name} reported no tokens for ${plural(a.turns, "turn")} in ${plural(a.sessions.size, "session")}, so the cost leaves ${a.turns === 1 ? "it" : "them"} out.`);
  }
  if (totals.size > 0) {
    const names = [...totals].sort();
    notes.push(`${list(names)} ${names.length === 1 ? "is" : "are"} counted from each turn's total, as ${names.length === 1 ? "it reports" : "they report"}, not per model call.`);
  }
  return notes;
}

// heldNote says what the policy held in the summary's time, beside what it cost (SEC-014).
export function heldNote(held: number): string {
  return held > 0 ? `The permission policy held ${plural(held, "tool call")} in this time.` : "";
}
