import { USAGE_PANE } from "./workspace.ts";
import { apply, emptyTranscript, type Transcript, type TranscriptEvent } from "./transcript.ts";

// A session that is not on screen holds no transcript (NFR-PERF-001, NFR-PERF-004). The
// window fetches a session's events when a pane or the screen shows it and lets them go
// a while after none does. What the sidebar, the Board and the attention view need comes
// from the session's row, the activity fold and the engine's attention list, none of
// which read the transcript. These are the rules; App.tsx applies them.

// LET_GO_MS is how long a session may be off screen before its events are dropped: long
// enough that going back and forth between two sessions costs no fetch.
export const LET_GO_MS = 30_000;

// heldSessions are the sessions whose transcripts the window keeps: those in a pane and
// the one on the screen. A standalone window holds its own session and no other.
export function heldSessions(w: { panes: readonly (string | null)[]; screenSession: string; standalone?: string }): Set<string> {
  if (w.standalone) return new Set([w.standalone]);
  const held = new Set<string>();
  for (const p of w.panes) if (p !== null && p !== USAGE_PANE) held.add(p);
  if (w.screenSession) held.add(w.screenSession);
  return held;
}

// ingestHeld applies events to the transcripts the window keeps: a held session's, and
// that of a session let go of soon but not yet (which must not fall behind, or showing
// it again would show a stale end). Events of any other session are dropped, not stored
// and not fetched for. A held session's gap is named in gaps; a gap in one that is not
// held drops its transcript, since a fetch would be for nothing. It returns trs itself
// when nothing changed.
export function ingestHeld(
  trs: Record<string, Transcript>,
  held: ReadonlySet<string>,
  groups: ReadonlyMap<string, readonly TranscriptEvent[]>,
): { transcripts: Record<string, Transcript>; gaps: string[] } {
  let next = trs;
  const gaps: string[] = [];
  for (const [sid, events] of groups) {
    const isHeld = held.has(sid);
    if (!isHeld && !next[sid]) continue;
    const current = next[sid] ?? emptyTranscript;
    const { transcript, missingAfter } = apply(current, events);
    if (missingAfter !== null && !isHeld) {
      if (next === trs) next = { ...trs };
      delete next[sid];
      continue;
    }
    if (transcript !== current) next = { ...next, [sid]: transcript };
    if (missingAfter !== null) gaps.push(sid);
  }
  return { transcripts: next, gaps };
}

// letGo drops the transcripts of sessions that have been off screen for grace ms. since
// holds when each session was first seen off screen; it comes back updated (a session
// held again has no entry). wait says when to look again, or null when nothing waits.
export function letGo(
  trs: Record<string, Transcript>,
  held: ReadonlySet<string>,
  since: ReadonlyMap<string, number>,
  now: number,
  grace: number,
): { transcripts: Record<string, Transcript>; since: Map<string, number>; wait: number | null } {
  const nextSince = new Map<string, number>();
  let transcripts = trs;
  let wait: number | null = null;
  for (const sid of Object.keys(trs)) {
    if (held.has(sid)) continue;
    const from = since.get(sid) ?? now;
    if (now - from >= grace) {
      if (transcripts === trs) transcripts = { ...trs };
      delete transcripts[sid];
      continue;
    }
    nextSince.set(sid, from);
    wait = Math.min(wait ?? grace, grace - (now - from));
  }
  return { transcripts, since: nextSince, wait };
}
