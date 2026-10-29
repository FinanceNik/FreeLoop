import { test } from "node:test";
import assert from "node:assert/strict";
import { orderModels } from "./models.ts";

test("favorites first, hidden left out", () => {
  const ids = ["a", "b", "c", "d", "b"];
  assert.deepEqual(orderModels(ids, ["c", "manual"], ["a"], false), ["c", "manual", "b", "d"]);
  assert.deepEqual(orderModels(ids, [], ["a"], true), ["a", "b", "c", "d"]);
  assert.deepEqual(orderModels([], [], [], false), []);
});
