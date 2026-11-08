import assert from "node:assert/strict";
import { test } from "node:test";
import { isDemo } from "./demo.ts";

test("the session on the tour provider is the demo", () => {
  assert.equal(isDemo({ providerId: "freeloop-tour", harnessId: "builtin" }), true);
});

test("a session the user tagged demo is not", () => {
  assert.equal(isDemo({ providerId: "anthropic", harnessId: "builtin" }), false);
  assert.equal(isDemo({ providerId: "", harnessId: "opencode" }), false);
  assert.equal(isDemo({ providerId: "freeloop-tour", harnessId: "opencode" }), false);
  assert.equal(isDemo({}), false);
  assert.equal(isDemo(undefined), false);
});
