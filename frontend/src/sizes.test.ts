import { test } from "node:test";
import assert from "node:assert/strict";
import { clampPanel, clampSidebar, clampSplit, panelMax, sidebarMax } from "./sizes.ts";

test("the sidebar stays readable and leaves the window its room", () => {
  assert.equal(clampSidebar(300, 1400), 300);
  assert.equal(clampSidebar(50, 1400), 180);
  assert.equal(clampSidebar(1000, 1400), 700);
  assert.equal(sidebarMax(1400), 700);
  // A very narrow window still gets a readable sidebar.
  assert.equal(clampSidebar(500, 300), 180);
});

test("a panel leaves the conversation room", () => {
  assert.equal(clampPanel(400, 1200), 400);
  assert.equal(clampPanel(100, 1200), 260);
  assert.equal(clampPanel(1100, 1200), 920);
  assert.equal(panelMax(1200), 920);
  assert.equal(clampPanel(500, 400), 260);
});

test("both columns of panes stay usable", () => {
  assert.equal(clampSplit(0.6, 1000), 0.6);
  assert.equal(clampSplit(0.1, 1000), 0.3);
  assert.equal(clampSplit(0.9, 1000), 0.7);
  // Too narrow for any choice, they are even; the panes become tabs before this.
  assert.equal(clampSplit(0.2, 500), 0.5);
  assert.equal(clampSplit(0.2, 0), 0.5);
});
