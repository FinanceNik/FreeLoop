import { test } from "node:test";
import assert from "node:assert/strict";
import { bySession, createBatcher, foldSessions, FRAME_MS, type Clock } from "./eventBatch.ts";
import type { Session } from "./api.ts";
import type { TranscriptEvent } from "./transcript.ts";

const ev = (seq: number, type = "agent_message_chunk", sid = "a", data: unknown = {}): TranscriptEvent => ({ v: 1, seq, ts: "t" + seq, sid, type, data });

function fakeClock() {
  let next = 1;
  const timers = new Map<number, { fn: () => void; at: number }>();
  let now = 0;
  const clock: Clock = {
    set: (fn, ms) => {
      timers.set(next, { fn, at: now + ms });
      return next++;
    },
    clear: (h) => void timers.delete(h as number),
  };
  const advance = (ms: number) => {
    now += ms;
    for (const [id, t] of [...timers]) {
      if (t.at <= now) {
        timers.delete(id);
        t.fn();
      }
    }
  };
  return { clock, advance, pending: () => timers.size };
}

test("a burst of 1,000 chunks is delivered in one batch per frame, in order", () => {
  const f = fakeClock();
  const batches: TranscriptEvent[][] = [];
  const b = createBatcher<TranscriptEvent>((x) => batches.push(x), { clock: f.clock });
  for (let i = 1; i <= 1000; i++) b.push(ev(i));
  assert.equal(batches.length, 0);
  assert.equal(f.pending(), 1);
  f.advance(FRAME_MS);
  assert.equal(batches.length, 1);
  assert.deepEqual(
    batches[0].map((e) => e.seq),
    Array.from({ length: 1000 }, (_, i) => i + 1),
  );
  f.advance(1000);
  assert.equal(batches.length, 1);
});

test("a permission request, a decision, a turn's end and a status change are delivered at once with what came before", () => {
  for (const type of ["permission_requested", "permission_decided", "turn_ended", "session_status"]) {
    const f = fakeClock();
    const batches: TranscriptEvent[][] = [];
    const b = createBatcher<TranscriptEvent>((x) => batches.push(x), { clock: f.clock });
    b.push(ev(1));
    b.push(ev(2, type));
    assert.equal(batches.length, 1, type);
    assert.deepEqual(
      batches[0].map((e) => e.seq),
      [1, 2],
    );
    assert.equal(f.pending(), 0);
  }
});

test("cancel drops what is held", () => {
  const f = fakeClock();
  const batches: TranscriptEvent[][] = [];
  const b = createBatcher<TranscriptEvent>((x) => batches.push(x), { clock: f.clock });
  b.push(ev(1));
  b.cancel();
  f.advance(100);
  assert.equal(batches.length, 0);
});

test("bySession keeps each session's order", () => {
  const g = bySession([ev(1, "x", "a"), ev(1, "x", "b"), ev(2, "x", "a")]);
  assert.deepEqual(
    g.get("a")!.map((e) => e.seq),
    [1, 2],
  );
  assert.deepEqual(
    g.get("b")!.map((e) => e.seq),
    [1],
  );
});

const session = (id: string): Session => ({ id, title: id, lastSeq: 0, status: "idle", updatedAt: "" }) as unknown as Session;

test("foldSessions moves touched sessions to the front, applies changes and names unknown ones without side effects", () => {
  const list = [session("a"), session("b"), session("c")];
  const r = foldSessions(list, [ev(1, "session_status", "c", { status: "running" }), ev(1, "x", "zzz"), ev(2, "session_info_update", "b", { title: "B!" }), ev(2, "x", "zzz")]);
  assert.deepEqual(
    r.sessions.map((s) => s.id),
    ["b", "c", "a"],
  );
  assert.equal(r.sessions[1].status, "running");
  assert.equal(r.sessions[0].title, "B!");
  assert.equal(r.sessions[0].lastSeq, 2);
  assert.deepEqual(r.unknown, ["zzz"]);
  assert.equal(list[0].id, "a");
  assert.equal(list[1].title, "b");
});
