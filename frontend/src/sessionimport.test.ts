import assert from "node:assert/strict";
import { test } from "node:test";
import { canImport, folderName, importResult, matchesImport, type ImportRow } from "./sessionimport.ts";

const row = (over: Partial<ImportRow> = {}): ImportRow => ({ id: "a", title: "Fix the login", folder: "/Users/me/shop", sessionId: "", missing: false, ...over });

test("a session here already, or with its folder gone, cannot be picked", () => {
  assert.equal(canImport(row()), true);
  assert.equal(canImport(row({ sessionId: "ses_1" })), false);
  assert.equal(canImport(row({ missing: true })), false);
});

test("rows are found by words of the title and folder, in any order", () => {
  assert.equal(matchesImport(row(), ""), true);
  assert.equal(matchesImport(row(), "LOGIN shop"), true);
  assert.equal(matchesImport(row(), "shop payment"), false);
});

test("a folder shows by its last part", () => {
  assert.equal(folderName("/Users/me/shop/"), "shop");
  assert.equal(folderName("/"), "/");
  assert.equal(folderName("C:\\Users\\me\\shop\\"), "shop");
});

test("the result counts sessions and failures", () => {
  assert.equal(importResult(1, 0), "Imported 1 session.");
  assert.equal(importResult(3, 1), "Imported 3 sessions; 1 could not be imported.");
});
