import assert from "node:assert/strict";
import { test } from "node:test";
import { isDemo } from "./demo.ts";

test("a session tagged demo is the demo", () => {
  assert.equal(isDemo({ tags: ["demo"] }), true);
  assert.equal(isDemo({ tags: ["work", "demo"] }), true);
});

test("any other session is not", () => {
  assert.equal(isDemo({ tags: [] }), false);
  assert.equal(isDemo({ tags: ["demos", "Demo"] }), false);
  assert.equal(isDemo({}), false);
  assert.equal(isDemo({ tags: null }), false);
  assert.equal(isDemo(undefined), false);
});
