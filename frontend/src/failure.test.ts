import { test } from "node:test";
import assert from "node:assert/strict";
import { failureReport } from "./failure.ts";

test("a failure's report is the problem report with the window's error under it", () => {
  const text = failureReport("Freeloop 1.0\n", new TypeError("x is not a function"), "\n    at Item\n    at Pane");
  assert.ok(text.startsWith("Freeloop 1.0\n"));
  assert.ok(text.includes("TypeError: x is not a function"));
  assert.ok(text.includes("at Pane"));
});

test("a thrown value that is not an error is told as it is, and the report may be missing", () => {
  assert.ok(failureReport("", "plain words", "").includes("plain words"));
  assert.ok(failureReport("", { a: 1 }, "").length > 0);
});

test("a long stack is cut", () => {
  assert.ok(failureReport("", new Error("e"), "x".repeat(50_000)).length < 12_000);
});
