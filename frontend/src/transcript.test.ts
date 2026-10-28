import { test } from "node:test";
import assert from "node:assert/strict";
import { apply, emptyTranscript, summary, type TranscriptEvent } from "./transcript.ts";

const ev = (seq: number, data: unknown = {}): TranscriptEvent => ({
  v: 1,
  seq,
  ts: "2026-10-28T00:00:00Z",
  sid: "ses_1",
  type: "notice",
  data,
});

test("appends contiguous events", () => {
  const r = apply(emptyTranscript, [ev(1), ev(2)]);
  assert.equal(r.missingAfter, null);
  assert.equal(r.transcript.lastSeq, 2);
  assert.deepEqual(r.transcript.events.map((e) => e.seq), [1, 2]);
});

test("accepts out-of-order input within one batch", () => {
  const r = apply(emptyTranscript, [ev(2), ev(1), ev(3)]);
  assert.equal(r.transcript.lastSeq, 3);
  assert.equal(r.missingAfter, null);
});

test("ignores duplicates and returns the same object when nothing changes", () => {
  const t = apply(emptyTranscript, [ev(1), ev(2)]).transcript;
  const r = apply(t, [ev(1), ev(2)]);
  assert.equal(r.transcript, t);
  assert.equal(r.missingAfter, null);
});

test("stops at a gap and reports where to fetch from", () => {
  const t = apply(emptyTranscript, [ev(1)]).transcript;
  const r = apply(t, [ev(2), ev(4), ev(5)]);
  assert.equal(r.transcript.lastSeq, 2);
  assert.equal(r.missingAfter, 2);
});

test("replay then live is the same as live alone", () => {
  const replayed = apply(apply(emptyTranscript, [ev(1), ev(2)]).transcript, [ev(2), ev(3)]).transcript;
  const live = apply(emptyTranscript, [ev(1), ev(2), ev(3)]).transcript;
  assert.deepEqual(replayed, live);
});

test("summary prefers text content", () => {
  assert.equal(summary(ev(1, { content: { type: "text", text: "hi" } })), "hi");
  assert.equal(summary(ev(1, { status: "idle" })), "idle");
  assert.equal(summary(ev(1, { a: 1 })), '{"a":1}');
});
