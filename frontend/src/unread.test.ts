import { test } from "node:test";
import assert from "node:assert/strict";
import { finishedTurn, markFinished, markRead, onScreen, parseUnread } from "./unread.ts";

test("a finished turn is news; a stopped or failed one is not", () => {
  assert.equal(finishedTurn({ type: "turn_ended", data: { stopReason: "end_turn" } }), true);
  assert.equal(finishedTurn({ type: "turn_ended", data: { stopReason: "max_turns" } }), true);
  assert.equal(finishedTurn({ type: "turn_ended", data: { stopReason: "cancelled" } }), false);
  assert.equal(finishedTurn({ type: "turn_ended", data: { stopReason: "error" } }), false);
  assert.equal(finishedTurn({ type: "session_status", data: { status: "idle" } }), false);
});

const panes = { panes: ["a", "b", null], active: 1, tabbed: false, showingPanes: true, focused: true };

test("what is on screen", () => {
  assert.deepEqual(onScreen(panes), ["a", "b"]);
  assert.deepEqual(onScreen({ ...panes, tabbed: true }), ["b"]);
  assert.deepEqual(onScreen({ ...panes, focused: false }), []);
  assert.deepEqual(onScreen({ ...panes, showingPanes: false }), []);
});

test("a session finishing out of sight is marked once, and one in sight not at all", () => {
  assert.deepEqual(markFinished([], "a", ["b"]), ["a"]);
  assert.deepEqual(markFinished(["a"], "a", ["b"]), ["a"]);
  assert.deepEqual(markFinished([], "a", ["a"]), []);
});

test("seeing a session clears it, and nothing changing keeps the same array", () => {
  assert.deepEqual(markRead(["a", "b"], ["a"]), ["b"]);
  const same = ["a"];
  assert.equal(markRead(same, ["z"]), same);
});

test("what was stored may be anything", () => {
  assert.deepEqual(parseUnread('["a","b"]'), ["a", "b"]);
  assert.deepEqual(parseUnread(null), []);
  assert.deepEqual(parseUnread("{not json"), []);
  assert.deepEqual(parseUnread('{"a":1}'), []);
  assert.deepEqual(parseUnread('["a",1]'), ["a"]);
});
