import { test } from "node:test";
import assert from "node:assert/strict";
import { addError, errorLine } from "./errorlist.ts";

test("an error names its session", () => {
  assert.equal(errorLine("no key", "Fix login"), "Fix login: no key");
  assert.equal(errorLine("no key"), "no key");
});

test("a new error does not replace an unread one, and repeats do not pile up", () => {
  assert.deepEqual(addError(["a"], "b"), ["a", "b"]);
  assert.deepEqual(addError(["a", "b"], "a"), ["b", "a"]);
});

test("the list stays short, keeping the newest", () => {
  assert.deepEqual(addError(["a", "b", "c", "d"], "e"), ["b", "c", "d", "e"]);
});
