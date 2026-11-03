import { test } from "node:test";
import assert from "node:assert/strict";
import { trustStep } from "./trust.ts";

test("a project whose hooks are unknown is not trusted yet", () => {
  assert.equal(trustStep(null, true), "lookup");
  assert.equal(trustStep(null, false), "lookup");
});

test("hooks not on screen are confirmed, and no hooks need no question", () => {
  assert.equal(trustStep([{ command: "x" }], true), "confirm");
  assert.equal(trustStep([], true), "trust");
});

test("hooks the banner already lists need no second question", () => {
  assert.equal(trustStep([{ command: "x" }], false), "trust");
});
