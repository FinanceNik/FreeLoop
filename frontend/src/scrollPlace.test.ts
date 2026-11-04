import { test } from "node:test";
import assert from "node:assert/strict";
import { heightAbove, holdsPlace, readerScrolling, scrollsByKey } from "./scrollPlace.ts";

test("the reader scrolling up stops following what arrives", () => {
  assert.equal(holdsPlace(false, 1000, 900, 500, true), true);
  // Even a small step, still near the end.
  assert.equal(holdsPlace(false, 1000, 980, 20, true), true);
});

test("back at the end it follows again", () => {
  assert.equal(holdsPlace(true, 900, 1400, 0, true), false);
  assert.equal(holdsPlace(true, 900, 1340, 60, true), false);
});

test("scrolling down but not to the end keeps the place", () => {
  assert.equal(holdsPlace(true, 900, 1000, 400, true), true);
});

test("output growing past the end does not stop it following", () => {
  // Following, and a scroll event arrives after a large tool result was added.
  assert.equal(holdsPlace(false, 1000, 1000, 800, false), false);
  assert.equal(holdsPlace(false, 1000, 1200, 800, false), false);
});

test("a rounding of a pixel is not scrolling up", () => {
  assert.equal(holdsPlace(false, 1000, 999.5, 0, true), false);
});

test("the view moving up by itself does not stop it following", () => {
  // Measured in WebKit: cards collapsing at the end moved the view from 1730 to 1154.
  assert.equal(holdsPlace(false, 1730, 1154, 0, false), false);
  // An item above drawn shorter than its placeholder, with output still to scroll to.
  assert.equal(holdsPlace(false, 1000, 960, 800, false), false);
  // Cards collapsing just after the reader's click land on the end too.
  assert.equal(holdsPlace(false, 1730, 1154, 0, true), false);
});

test("a place the reader chose stays when the view moves by itself", () => {
  assert.equal(holdsPlace(true, 1000, 960, 800, false), true);
});

test("a scroll is the reader's during and just after their input", () => {
  assert.equal(readerScrolling(1000, 900, false), true);
  assert.equal(readerScrolling(1000, 0, true), true);
  assert.equal(readerScrolling(1000, 0, false), false);
});

test("the keys that scroll count as the reader's input, others do not", () => {
  assert.equal(scrollsByKey("PageUp"), true);
  assert.equal(scrollsByKey("a"), false);
});

test("only items wholly above the view count towards how far to scroll", () => {
  const items = [
    { bottom: 40, before: 60, after: 400 }, // above
    { bottom: 100, before: 60, after: 200 }, // ending at the top of the view
    { bottom: 130, before: 60, after: 900 }, // across it
    { bottom: 500, before: 60, after: 900 }, // in it
  ];
  assert.equal(heightAbove(items, 100), 340 + 140);
  assert.equal(heightAbove([], 100), 0);
});
