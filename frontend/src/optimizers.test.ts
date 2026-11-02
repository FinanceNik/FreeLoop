import assert from "node:assert/strict";
import { test } from "node:test";
import { inheritedLabel, inheritedOptimizer } from "./optimizers.ts";

const list = [{ id: "rtk", name: "RTK" }];

test("a new session takes the profile's optimizer, then the one in Settings (HRN-019)", () => {
  assert.equal(inheritedOptimizer("", ""), "");
  assert.equal(inheritedOptimizer("", "rtk"), "rtk");
  assert.equal(inheritedOptimizer("other", "rtk"), "other");
  assert.equal(inheritedOptimizer("off", "rtk"), "");
});

test("the form says where the inherited optimizer comes from", () => {
  assert.equal(inheritedLabel("", "", list), "From Settings (Off)");
  assert.equal(inheritedLabel("", "rtk", list), "From Settings (RTK)");
  assert.equal(inheritedLabel("off", "rtk", list), "From the profile (Off)");
  assert.equal(inheritedLabel("gone", "", list), "From the profile (gone)");
});
