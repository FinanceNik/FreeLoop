// A session's transcript in the UI is a pure function of its events. Replay
// from disk and the live stream both go through apply, so they cannot drift.
// This module has no runtime imports so `node --test` can run it directly.

export interface TranscriptEvent {
  v: number;
  seq: number;
  ts: string;
  sid: string;
  type: string;
  data: unknown;
}

export interface Transcript {
  events: TranscriptEvent[];
  lastSeq: number;
}

export const emptyTranscript: Transcript = { events: [], lastSeq: 0 };

export interface ApplyResult {
  transcript: Transcript;
  // When incoming events skip ahead (the bridge dropped some), the caller
  // fetches everything after this seq and applies it.
  missingAfter: number | null;
}

// apply appends the events that extend t contiguously, ignores ones already
// seen, and stops at the first gap.
export function apply(t: Transcript, incoming: readonly TranscriptEvent[]): ApplyResult {
  const sorted = [...incoming].sort((a, b) => a.seq - b.seq);
  const added: TranscriptEvent[] = [];
  let lastSeq = t.lastSeq;
  for (const ev of sorted) {
    if (ev.seq <= lastSeq) continue;
    if (ev.seq !== lastSeq + 1) {
      return { transcript: extend(t, added, lastSeq), missingAfter: lastSeq };
    }
    added.push(ev);
    lastSeq = ev.seq;
  }
  return { transcript: extend(t, added, lastSeq), missingAfter: null };
}

function extend(t: Transcript, added: TranscriptEvent[], lastSeq: number): Transcript {
  return added.length === 0 ? t : { events: [...t.events, ...added], lastSeq };
}

// summary renders an event's payload as one line of text for the M0 view.
export function summary(ev: TranscriptEvent): string {
  const d = ev.data as Record<string, unknown> | null;
  if (d && typeof d === "object") {
    const content = d.content as { type?: string; text?: string } | undefined;
    if (content?.type === "text" && typeof content.text === "string") return content.text;
    if (typeof d.status === "string") return d.status;
    if (typeof d.stopReason === "string") return d.stopReason;
    if (typeof d.message === "string") return d.message;
  }
  return JSON.stringify(ev.data);
}
