import { test } from "node:test";
import assert from "node:assert/strict";
import { fit, keepAsIs, MAX_BYTES, room } from "./images.ts";

test("pictures are scaled to fit, keeping their shape", () => {
  assert.deepEqual(fit(800, 600), { w: 800, h: 600 });
  assert.deepEqual(fit(4000, 3000), { w: 2000, h: 1500 });
  assert.deepEqual(fit(1000, 5000), { w: 400, h: 2000 });
  assert.deepEqual(fit(30000, 1), { w: 2000, h: 1 });
});

test("only small pictures of known types go unchanged", () => {
  assert.ok(keepAsIs("image/png", 100_000, 1200, 800));
  assert.ok(!keepAsIs("image/png", MAX_BYTES + 1, 1200, 800));
  assert.ok(!keepAsIs("image/png", 100_000, 2400, 800));
  assert.ok(!keepAsIs("image/svg+xml", 100, 10, 10));
});

test("at most ten pictures a message", () => {
  assert.equal(room(0, 3), 3);
  assert.equal(room(8, 5), 2);
  assert.equal(room(10, 1), 0);
});
