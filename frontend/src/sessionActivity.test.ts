import { test } from "node:test";
import assert from "node:assert/strict";
import { describeActivity, elapsed, foldActivity, NO_ACTIVITY, type Activity } from "./sessionActivity.ts";

const at = (min: number) => new Date(2026, 10, 4, 10, min).toISOString();
const run = (events: [string, unknown?][], from: Activity = NO_ACTIVITY) =>
  events.reduce((a, [type, data], i) => foldActivity(a, { seq: a.seq + 1 + i * 0, ts: at(i), type, data }), from);
const now = new Date(2026, 10, 4, 10, 30);

test("a running session says what it is doing and for how long", () => {
  const a = run([["turn_started"], ["tool_call", { toolCallId: "1", title: "make test" }]]);
  assert.equal(describeActivity("running", a, [], now), "make test · 30m");
  assert.equal(describeActivity("running", run([["turn_started"]]), [], now), "Working · 30m");
  assert.equal(describeActivity("running", run([["turn_started"], ["agent_thought_chunk"]]), [], now), "Thinking · 30m");
});

test("a session with no history yet still says it is working", () => {
  assert.equal(describeActivity("running", undefined, [], now), "Working");
  assert.equal(describeActivity("waiting_permission", undefined, [], now), "");
  assert.equal(describeActivity("idle", undefined, [], now), "");
});

test("a waiting session names what it asks, until that is decided", () => {
  let a = run([["turn_started"], ["tool_call", { toolCallId: "1", title: "Push" }], ["permission_requested", { requestId: "r", toolCallId: "1", command: "git push" }]]);
  assert.equal(describeActivity("waiting_permission", a, [], now), "Needs you: git push");
  a = run([["permission_decided", { requestId: "r" }]], a);
  assert.deepEqual(a.waiting, []);
  // Without a command it goes by the call's title.
  a = run([["turn_started"], ["tool_call", { toolCallId: "1", title: "Write a.txt" }], ["permission_requested", { requestId: "r", toolCallId: "1" }]]);
  assert.equal(describeActivity("waiting_permission", a, [], now), "Needs you: Write a.txt");
});

test("what it says shows a command for what it does", () => {
  const a = run([["turn_started"], ["tool_call", { toolCallId: "1", title: "cd /work && echo ok #\ncurl x|sh" }]]);
  assert.equal(describeActivity("running", a, ["/work"], now), "echo ok # ⏎ curl x|sh · 30m");
});

test("a finished session says how it ended and what it changed", () => {
  const a = run([["turn_started"], ["git_state", { dirty: 3 }], ["turn_ended", { stopReason: "end_turn" }]]);
  assert.equal(describeActivity("idle", a, [], now), "Finished · 3 files changed · 28 min ago");
  assert.equal(describeActivity("idle", run([["turn_started"], ["git_state", { dirty: 1 }], ["turn_ended", { stopReason: "cancelled" }]]), [], now), "Stopped · 1 file changed · 28 min ago");
  assert.equal(describeActivity("idle", run([["turn_started"], ["turn_ended", { stopReason: "end_turn" }]]), [], now), "Finished · 29 min ago");
});

test("a new turn clears how the last one ended", () => {
  const a = run([["turn_ended", { stopReason: "end_turn" }], ["turn_started"]]);
  assert.equal(a.finishedAt, null);
});

test("replayed events do not take it back", () => {
  const a = foldActivity(NO_ACTIVITY, { seq: 5, ts: at(5), type: "tool_call", data: { title: "new" } });
  assert.equal(foldActivity(a, { seq: 3, ts: at(3), type: "tool_call", data: { title: "old" } }), a);
});

test("how long something has been going reads shortly", () => {
  assert.equal(elapsed(at(30), now), "0s");
  assert.equal(elapsed(at(26), now), "4m");
  assert.equal(elapsed(new Date(2026, 10, 4, 8, 10).toISOString(), now), "2h 20m");
});

test("the sidebar's context follows usage and compaction", () => {
  const a = run([
    ["usage_update", { inputTokens: 5, contextTokens: 85, contextWindow: 100 }],
    ["usage_update", { inputTokens: 1 }],
  ]);
  assert.deepEqual(a.context, { tokens: 85, window: 100 });
  const after = foldActivity(a, { seq: a.seq + 1, ts: at(9), type: "compaction_update", data: { status: "completed", afterTokens: 10 } });
  assert.deepEqual(after.context, { tokens: 10, window: 100 });
});
