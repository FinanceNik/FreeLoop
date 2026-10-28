import { test } from "node:test";
import assert from "node:assert/strict";
import { apply, awaitingDecision, buildView, emptyTranscript, lineDiff, summary, type TranscriptEvent } from "./transcript.ts";

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

const at = (seq: number, type: string, data: unknown): TranscriptEvent => ({ v: 1, seq, ts: "2026-10-28T00:00:00Z", sid: "s", type, data });

test("buildView folds a tool-calling turn", () => {
  const v = buildView([
    at(1, "turn_started", {}),
    at(2, "user_message_chunk", { content: { type: "text", text: "fix it" } }),
    at(3, "agent_thought_chunk", { content: { type: "text", text: "hm " } }),
    at(4, "agent_thought_chunk", { content: { type: "text", text: "ok" } }),
    at(5, "agent_message_chunk", { content: { type: "text", text: "Let me " } }),
    at(6, "agent_message_chunk", { content: { type: "text", text: "look." } }),
    at(7, "usage_update", { inputTokens: 10, outputTokens: 5 }),
    at(8, "tool_call", { toolCallId: "c1", title: "Write a.go", kind: "edit", status: "pending", tool: "write_file" }),
    at(9, "permission_requested", { requestId: "r1", toolCallId: "c1", options: [{ optionId: "allow_once", name: "Allow once", kind: "allow_once" }] }),
  ]);
  assert.deepEqual(v.items.map((i) => i.kind), ["user", "thought", "agent", "tool"]);
  assert.equal((v.items[1] as any).text, "hm ok");
  assert.equal((v.items[2] as any).text, "Let me look.");
  assert.equal(awaitingDecision(v.items[3]), true);
  assert.equal(v.running, true);

  const v2 = buildView([
    at(8, "tool_call", { toolCallId: "c1", title: "Write a.go", kind: "edit", status: "pending" }),
    at(9, "permission_requested", { requestId: "r1", toolCallId: "c1", options: [{ optionId: "allow_once" }] }),
    at(10, "permission_decided", { requestId: "r1", toolCallId: "c1", outcome: "allowed", actor: "user" }),
    at(11, "tool_call_update", { toolCallId: "c1", status: "completed", content: [{ type: "diff", path: "a.go", oldText: "", newText: "x" }] }),
    at(12, "usage_update", { inputTokens: 20, outputTokens: 1, cacheReadTokens: 7 }),
    at(13, "turn_ended", { stopReason: "end_turn" }),
  ]);
  const tool = v2.items[0] as any;
  assert.equal(tool.status, "completed");
  assert.equal(tool.permission.decided.outcome, "allowed");
  assert.equal(awaitingDecision(tool), false);
  assert.equal(v2.usage.inputTokens, 20);
  assert.equal(v2.usage.cacheReadTokens, 7);
  assert.equal(v2.running, false);
});

test("lineDiff trims common lines to context", () => {
  const d = lineDiff("a\nb\nc\nd\ne\nf", "a\nb\nc\nX\ne\nf", 1);
  assert.deepEqual(d.map((l) => l.sign + l.text), [" c", "-d", "+X", " e"]);
  assert.deepEqual(lineDiff("", "new\n").map((l) => l.sign + l.text), ["+new"]);
});
