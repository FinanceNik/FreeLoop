import assert from "node:assert/strict";
import { test } from "node:test";
import { placeholder } from "./composer.ts";

test("a pane the user is not in gets the short placeholder", () => {
  assert.equal(placeholder(false, false), "Ask for a change…");
  assert.equal(placeholder(true, false), "Queue a message");
  assert.ok(placeholder(false, true).includes("Type @"));
});
