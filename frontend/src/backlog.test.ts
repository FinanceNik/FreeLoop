import assert from "node:assert/strict";
import { test } from "node:test";
import { counts, relations, stateLabel, typeLabel, type BacklogItem } from "./backlog.ts";

const item = (over: Partial<BacklogItem>): BacklogItem => ({ id: "001", title: "One", state: "ready", type: "afk", waitsOn: [], blocks: [], ...over });

test("an issue says what it waits on and holds up", () => {
  assert.equal(relations(item({ waitsOn: ["001-store"], blocks: ["003-a", "004-b"] })), "after 001-store · then 003-a, 004-b");
  assert.equal(relations(item({ cycle: true, waitsOn: ["002"] })), "waits on itself through others");
  assert.equal(relations(item({ waitsOn: null, blocks: null })), "");
  assert.match(relations(item({ problem: "no frontmatter" })), /can't be read/);
  assert.equal(stateLabel(item({ state: "blocked" })), "Blocked");
  assert.equal(typeLabel(item({ type: "human" })), "Human");
});

test("a backlog counts what can run", () => {
  const c = counts([item({}), item({ state: "blocked" }), item({ type: "human" }), item({ state: "done" }), item({ problem: "x" })]);
  assert.deepEqual(c, { open: 4, afkReady: 1, done: 1 });
  assert.deepEqual(counts(null), { open: 0, afkReady: 0, done: 0 });
});
