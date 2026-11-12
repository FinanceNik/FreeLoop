import { test } from "node:test";
import assert from "node:assert/strict";
import { withoutFailed, chooseLayout, closePane, cycle, focusPane, forget, movePane, openIn, placeIn, restore, setLayout, SINGLE, stableOrder, swapPanes, tabbed, type WorkspaceState, USAGE_PANE, isSession } from "./workspace.ts";

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

test("a pane can hold live usage, which no session list clears (GUI-023)", () => {
  const one = openIn({ layout: 1, panes: ["a"], active: 0 }, USAGE_PANE, "new");
  assert.deepEqual(one.panes, ["a", USAGE_PANE]);
  assert.equal(one.active, 1);
  assert.deepEqual(forget(one, () => false).panes, [null, USAGE_PANE]);
  assert.deepEqual(restore({ layout: 2, panes: ["a", USAGE_PANE], active: 1 }).panes, ["a", USAGE_PANE]);
  // Opened again, it goes where it already is.
  assert.equal(openIn({ ...one, active: 0 }, USAGE_PANE, "new").active, 1);
  assert.equal(isSession(USAGE_PANE), false);
  assert.equal(isSession(null), false);
  assert.equal(isSession("ses_1"), true);
});

test("dragging a pane onto another swaps them, and the dragged one stays active", () => {
  assert.deepEqual(swapPanes(w(2, ["a", "b"], 0), 0, 1), w(2, ["b", "a"], 1));
  assert.deepEqual(swapPanes(w(4, ["a", "b", "c", "d"], 3), 3, 0), w(4, ["d", "b", "c", "a"], 0));
  assert.deepEqual(swapPanes(w(3, ["a", null, USAGE_PANE], 2), 2, 1), w(3, ["a", USAGE_PANE, null], 1));
  // Onto itself, or from a pane that is gone, nothing changes.
  const same = w(2, ["a", "b"], 0);
  assert.equal(swapPanes(same, 1, 1), same);
  assert.equal(swapPanes(same, -1, 1), same);
  assert.equal(swapPanes(same, 0, 2), same);
});

test("panes keep the order they appeared in, whatever they show on screen", () => {
  assert.deepEqual(stableOrder([], ["a", "b"]), ["a", "b"]);
  assert.deepEqual(stableOrder(["a", "b"], ["b", "a"]), ["a", "b"]);
  assert.deepEqual(stableOrder(["a", "b", "c"], ["c", "d", "a"]), ["a", "c", "d"]);
});

test("moving between panes follows the layout on screen and wraps around", () => {
  const at = (layout: 1 | 2 | 3 | 4, active: number, d: "left" | "right" | "up" | "down", tabs = false) =>
    movePane(w(layout, Array.from({ length: layout }, (_, i) => `s${i}`), active), d, tabs).active;
  // Two side by side.
  assert.equal(at(2, 0, "right"), 1);
  assert.equal(at(2, 1, "right"), 0);
  assert.equal(at(2, 0, "left"), 1);
  assert.equal(at(2, 0, "up"), 0);
  assert.equal(at(2, 1, "down"), 1);
  // One full height on the left, two stacked on the right.
  assert.equal(at(3, 0, "right"), 1);
  assert.equal(at(3, 2, "left"), 0);
  assert.equal(at(3, 1, "down"), 2);
  assert.equal(at(3, 2, "down"), 1);
  assert.equal(at(3, 0, "down"), 0);
  // A grid.
  assert.equal(at(4, 0, "right"), 1);
  assert.equal(at(4, 3, "left"), 2);
  assert.equal(at(4, 1, "down"), 3);
  assert.equal(at(4, 3, "down"), 1);
  assert.equal(at(4, 2, "up"), 0);
  // One pane has nowhere to go, and tabs stand in a row.
  assert.equal(at(1, 0, "right"), 0);
  assert.equal(at(4, 3, "right", true), 0);
  assert.equal(at(4, 0, "left", true), 3);
  assert.equal(at(4, 1, "down", true), 1);
});

test("a pane that failed is left out of what is saved, the rest kept in place", () => {
  const work = w(2, ["a", "b"], 1);
  assert.deepEqual(withoutFailed(work, new Set(["b"])), w(2, ["a", null], 1));
  assert.deepEqual(withoutFailed(work, new Set(["x"])), work);
  assert.equal(withoutFailed(work, new Set()), work);
  assert.deepEqual(withoutFailed(w(2, [USAGE_PANE, "b"]), new Set([USAGE_PANE])), w(2, [null, "b"]));
});
