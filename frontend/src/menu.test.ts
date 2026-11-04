import { test } from "node:test";
import assert from "node:assert/strict";
import { menuPosition, nextMenuItem } from "./menu.ts";

test("a menu opens at the pointer, moved in to fit", () => {
  assert.deepEqual(menuPosition(100, 100, 200, 150, 1000, 800), { left: 100, top: 100 });
  assert.deepEqual(menuPosition(900, 750, 200, 150, 1000, 800), { left: 792, top: 642 });
  assert.deepEqual(menuPosition(0, 0, 200, 150, 1000, 800), { left: 8, top: 8 });
  // Bigger than the window, its start stays in sight.
  assert.deepEqual(menuPosition(50, 50, 200, 900, 1000, 800), { left: 50, top: 8 });
});

test("the arrow keys move through a menu and wrap around", () => {
  assert.equal(nextMenuItem("ArrowDown", 0, 3), 1);
  assert.equal(nextMenuItem("ArrowDown", 2, 3), 0);
  assert.equal(nextMenuItem("ArrowUp", 0, 3), 2);
  assert.equal(nextMenuItem("Home", 2, 3), 0);
  assert.equal(nextMenuItem("End", 0, 3), 2);
  assert.equal(nextMenuItem("a", 0, 3), -1);
  assert.equal(nextMenuItem("ArrowDown", -1, 3), 0);
  assert.equal(nextMenuItem("ArrowDown", 0, 0), -1);
});
