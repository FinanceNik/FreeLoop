import { test } from "node:test";
import assert from "node:assert/strict";
import { heldSessions, ingestHeld, letGo, LET_GO_MS } from "./heldTranscripts.ts";
import { emptyTranscript, type Transcript, type TranscriptEvent } from "./transcript.ts";

const ev = (sid: string, seq: number): TranscriptEvent => ({ sid, seq, ts: "2026-01-01T00:00:00Z", type: "agent_message_chunk", data: {} }) as TranscriptEvent;
const tr = (n: number): Transcript => ({ events: Array.from({ length: n }, (_, i) => ev("x", i + 1)), length: n, lastSeq: n });

test("the sessions held are those in a pane or on the screen; a standalone window holds its own", () => {
  assert.deepEqual([...heldSessions({ panes: ["a", null, "usage", "b"], screenSession: "c" })].sort(), ["a", "b", "c"]);
  assert.deepEqual([...heldSessions({ panes: ["a"], screenSession: "" })], ["a"]);
  assert.deepEqual([...heldSessions({ panes: ["a", "b"], screenSession: "a", standalone: "z" })], ["z"]);
});

test("an event for a session in no pane fetches nothing and stores no event", () => {
  const held = new Set(["a"]);
  const groups = new Map([
    ["a", [ev("a", 1)]],
    ["b", [ev("b", 1), ev("b", 5)]],
  ]);
  const { transcripts, gaps } = ingestHeld({}, held, groups);
  assert.deepEqual(Object.keys(transcripts), ["a"]);
  assert.equal(transcripts.a.events.length, 1);
  assert.deepEqual(gaps, []);
});

test("a held session's gap is reported for a catch-up", () => {
  const { gaps } = ingestHeld({}, new Set(["a"]), new Map([["a", [ev("a", 7)]]]));
  assert.deepEqual(gaps, ["a"]);
});

test("a session on its way out keeps up with its events, so showing it again shows its end", () => {
  const trs = { a: tr(2) };
  const { transcripts, gaps } = ingestHeld(trs, new Set(), new Map([["a", [ev("a", 3)]]]));
  assert.equal(transcripts.a.lastSeq, 3);
  assert.deepEqual(gaps, []);
});

test("a gap in a session on its way out drops its transcript instead of fetching", () => {
  const trs = { a: tr(2), b: tr(1) };
  const { transcripts, gaps } = ingestHeld(trs, new Set(), new Map([["a", [ev("a", 9)]]]));
  assert.deepEqual(Object.keys(transcripts), ["b"]);
  assert.deepEqual(gaps, []);
  assert.deepEqual(Object.keys(trs), ["a", "b"]);
});

test("nothing held, nothing changed: the same object comes back", () => {
  const trs = { a: tr(2) };
  const { transcripts } = ingestHeld(trs, new Set(["a"]), new Map([["b", [ev("b", 1)]]]));
  assert.equal(transcripts, trs);
});

test("a session no pane shows is let go after a while, and one shown again is kept", () => {
  const trs = { a: tr(3), b: tr(3), c: emptyTranscript };
  const held = new Set(["a"]);
  // First sight: the clock starts, nothing is dropped.
  let r = letGo(trs, held, new Map(), 1000, LET_GO_MS);
  assert.equal(r.transcripts, trs);
  assert.equal(r.wait, LET_GO_MS);
  // Shown again before the time is up: its clock is cleared.
  r = letGo(trs, new Set(["a", "b"]), r.since, 1000 + LET_GO_MS - 1, LET_GO_MS);
  assert.equal(r.transcripts, trs);
  assert.equal(r.since.has("b"), false);
  // Left again; once the time has passed it is gone, and what is held stays.
  r = letGo(trs, held, r.since, 5000, LET_GO_MS);
  r = letGo(trs, held, r.since, 5000 + LET_GO_MS, LET_GO_MS);
  assert.deepEqual(Object.keys(r.transcripts), ["a"]);
  assert.equal(r.wait, null);
  assert.equal(r.since.size, 0);
});
