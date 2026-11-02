import { test } from "node:test";
import assert from "node:assert/strict";
import { chooseLayout, closePane, cycle, focusPane, forget, openIn, placeIn, restore, setLayout, SINGLE, tabbed, type WorkspaceState } from "./workspace.ts";

const w = (layout: 1 | 2 | 3 | 4, panes: (string | null)[], active = 0): WorkspaceState => ({ layout, panes, active });

test("a session opens in the active pane, or where it already is", () => {
  assert.deepEqual(openIn(SINGLE, "a", "active"), w(1, ["a"]));
  assert.deepEqual(openIn(w(2, ["a", "b"], 0), "b", "active"), w(2, ["a", "b"], 1));
  assert.deepEqual(openIn(w(2, ["a", "b"], 1), "c", "active"), w(2, ["a", "c"], 1));
});

test("a new pane is the first empty one, or one more, up to four", () => {
  assert.deepEqual(openIn(w(2, ["a", null]), "b", "new"), w(2, ["a", "b"], 1));
  assert.deepEqual(openIn(w(1, ["a"]), "b", "new"), w(2, ["a", "b"], 1));
  assert.deepEqual(openIn(w(3, ["a", "b", "c"], 0), "d", "new"), w(4, ["a", "b", "c", "d"], 3));
  // All four full: the active pane takes it.
  assert.deepEqual(openIn(w(4, ["a", "b", "c", "d"], 2), "e", "new"), w(4, ["a", "b", "e", "d"], 2));
});

test("shrinking keeps the active session first and drops the rest in order", () => {
  assert.deepEqual(setLayout(w(4, ["a", "b", "c", "d"], 2), 2), w(2, ["c", "a"], 0));
  assert.deepEqual(setLayout(w(3, [null, "b", null], 0), 1), w(1, ["b"], 0));
  assert.deepEqual(setLayout(w(1, ["a"]), 3), w(3, ["a", null, null], 0));
});

test("dropping a session on a pane moves it there", () => {
  assert.deepEqual(placeIn(w(2, ["a", "b"], 0), "a", 1), w(2, [null, "a"], 1));
  assert.deepEqual(placeIn(w(2, ["a", null], 0), "c", 1), w(2, ["a", "c"], 1));
  assert.deepEqual(placeIn(w(2, ["a", null]), "c", 5), w(2, ["a", null]));
});

test("closing a pane removes it; the last one is emptied", () => {
  assert.deepEqual(closePane(w(3, ["a", "b", "c"], 2), 0), w(2, ["b", "c"], 1));
  assert.deepEqual(closePane(w(2, ["a", "b"], 1), 1), w(1, ["a"], 0));
  assert.deepEqual(closePane(w(1, ["a"]), 0), w(1, [null]));
});

test("moving between panes wraps around", () => {
  assert.equal(cycle(w(4, ["a", "b", "c", "d"], 3), 1).active, 0);
  assert.equal(cycle(w(4, ["a", "b", "c", "d"], 0), -1).active, 3);
  assert.equal(focusPane(w(2, ["a", "b"]), 1).active, 1);
  assert.equal(focusPane(w(2, ["a", "b"]), 7).active, 0);
});

test("deleted sessions leave their panes empty", () => {
  assert.deepEqual(forget(w(2, ["a", "b"]), (s) => s !== "b"), w(2, ["a", null]));
});

test("a saved workspace is checked before use", () => {
  assert.deepEqual(restore({ layout: 2, panes: ["a", "a", "b"], active: 9 }), w(2, ["a", null], 1));
  assert.deepEqual(restore({ layout: 7, panes: [] }), SINGLE);
  assert.deepEqual(restore(null), SINGLE);
});

test("narrow windows turn the panes into tabs", () => {
  assert.equal(tabbed(w(2, ["a", "b"]), 700), true);
  assert.equal(tabbed(w(2, ["a", "b"]), 1000), false);
  assert.equal(tabbed(w(1, ["a"]), 300), false);
});

test("a pane added with the layout control is the one the next session goes into", () => {
  assert.deepEqual(chooseLayout(w(1, ["a"]), 2), w(2, ["a", null], 1));
  assert.deepEqual(chooseLayout(w(2, ["a", null], 0), 3), w(3, ["a", null, null], 1));
  assert.deepEqual(chooseLayout(w(3, ["a", "b", "c"], 2), 2), w(2, ["c", "a"], 0));
  assert.deepEqual(openIn(chooseLayout(w(1, ["a"]), 2), "b", "active"), w(2, ["a", "b"], 1));
});
