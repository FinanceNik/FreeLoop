import type { Session } from "./api.ts";
import type { TranscriptEvent } from "./transcript.ts";

// Events reach the window in batches, one per frame (NFR-PERF-001). The engine emits
// one event per token; applying each on its own redraws the whole app for every token
// of every session. The batcher gathers events and hands them over once per frame, in
// the order they came, and the window applies a batch in one update per piece of state.

// FRAME_MS is about one frame at 60 Hz.
export const FRAME_MS = 16;

// urgent says an event is not held back: a request for a decision, the decision, a
// turn's end and a change of status (rare, and what a row says it is waiting for comes
// from it). It carries the events gathered before it, so order is kept.
export function urgent(ev: { type: string }): boolean {
  return ev.type === "permission_requested" || ev.type === "permission_decided" || ev.type === "turn_ended" || ev.type === "session_status";
}

export interface Batcher<T> {
  push(ev: T): void;
  // flush hands over what is held now.
  flush(): void;
  // cancel drops the timer and what is held (the window is going away).
  cancel(): void;
}

export interface Clock {
  set(fn: () => void, ms: number): unknown;
  clear(handle: unknown): void;
}

const realClock: Clock = { set: (fn, ms) => setTimeout(fn, ms), clear: (h) => clearTimeout(h as ReturnType<typeof setTimeout>) };

export function createBatcher<T extends { type: string }>(
  deliver: (batch: T[]) => void,
  opts: { clock?: Clock; ms?: number; urgent?: (ev: T) => boolean } = {},
): Batcher<T> {
  const clock = opts.clock ?? realClock;
  const ms = opts.ms ?? FRAME_MS;
  const isUrgent = opts.urgent ?? urgent;
  let held: T[] = [];
  let timer: unknown = null;
  const flush = () => {
    if (timer !== null) {
      clock.clear(timer);
      timer = null;
    }
    if (held.length === 0) return;
    const batch = held;
    held = [];
    deliver(batch);
  };
  return {
    push(ev) {
      held.push(ev);
      if (isUrgent(ev)) flush();
      else if (timer === null) timer = clock.set(flush, ms);
    },
    flush,
    cancel() {
      if (timer !== null) clock.clear(timer);
      timer = null;
      held = [];
    },
  };
}

// bySession groups a batch by session, each group in arrival order.
export function bySession<T extends { sid: string }>(batch: readonly T[]): Map<string, T[]> {
  const groups = new Map<string, T[]>();
  for (const ev of batch) {
    const g = groups.get(ev.sid);
    if (g) g.push(ev);
    else groups.set(ev.sid, [ev]);
  }
  return groups;
}

function touch(s: Session, ev: TranscriptEvent): Session {
  const d = (ev.data ?? {}) as Record<string, string>;
  const next = { ...s, lastSeq: Math.max(s.lastSeq, ev.seq), updatedAt: ev.ts };
  if (ev.type === "session_status" && d.status) next.status = d.status as Session["status"];
  if (ev.type === "current_mode_update" && d.currentModeId) next.mode = d.currentModeId;
  if (ev.type === "git_state") next.branch = d.branch ?? "";
  if (ev.type === "model_changed" && d.model) {
    next.model = d.model;
    next.providerId = d.providerId || next.providerId;
  }
  if (ev.type === "session_info_update" && d.title) next.title = d.title;
  if (ev.type === "optimizer_changed") next.optimizer = d.optimizer ?? "";
  if (ev.type === "workflow_changed") next.workflow = (ev.data as { workflow?: string }).workflow ?? "";
  if (ev.type === "tags_changed") next.tags = (ev.data as { tags?: string[] }).tags ?? [];
  return next;
}

// foldSessions moves each session a batch touches to the front of the list, the one
// touched last first, with what its events changed. Sessions the list does not hold are
// named in unknown, for the caller to refresh outside any state updater. It does not
// change its input.
export function foldSessions(list: readonly Session[], batch: readonly TranscriptEvent[]): { sessions: Session[]; unknown: string[] } {
  let sessions = [...list];
  const unknown: string[] = [];
  for (const ev of batch) {
    const i = sessions.findIndex((s) => s.id === ev.sid);
    if (i < 0) {
      if (!unknown.includes(ev.sid)) unknown.push(ev.sid);
      continue;
    }
    const s = touch(sessions[i], ev);
    sessions = [s, ...sessions.slice(0, i), ...sessions.slice(i + 1)];
  }
  return { sessions, unknown };
}
