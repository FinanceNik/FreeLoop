import { test } from "node:test";
import assert from "node:assert/strict";
import { heightAbove, holdsPlace } from "./scrollPlace.ts";

test("scrolling up stops following what arrives", () => {
  assert.equal(holdsPlace(false, 1000, 900, 500), true);
});

test("back at the end it follows again", () => {
  assert.equal(holdsPlace(true, 900, 1400, 0), false);
  assert.equal(holdsPlace(true, 900, 1340, 60), false);
});

test("scrolling down but not to the end keeps the place", () => {
  assert.equal(holdsPlace(true, 900, 1000, 400), true);
});

test("output growing past the end does not stop it following", () => {
  // Following, and a scroll event arrives after a large tool result was added.
  assert.equal(holdsPlace(false, 1000, 1000, 800), false);
  assert.equal(holdsPlace(false, 1000, 1200, 800), false);
});

test("a rounding of a pixel is not scrolling up", () => {
  assert.equal(holdsPlace(false, 1000, 999.5, 0), false);
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
