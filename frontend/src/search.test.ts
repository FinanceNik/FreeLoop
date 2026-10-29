import { test } from "node:test";
import assert from "node:assert/strict";
import { itemForSeq, snippetParts } from "./search.ts";

test("snippetParts marks matches and folds whitespace", () => {
  assert.deepEqual(snippetParts("…panic: \x02send on closed\x03 channel\n\nin \x02reaper\x03"), [
    { text: "…panic: ", mark: false },
    { text: "send on closed", mark: true },
    { text: " channel in ", mark: false },
    { text: "reaper", mark: true },
  ]);
  assert.deepEqual(snippetParts("<b>not markup</b>"), [{ text: "<b>not markup</b>", mark: false }]);
  assert.deepEqual(snippetParts(""), []);
});

test("itemForSeq finds the item a hit starts at, or the one before", () => {
  const items = [{ key: "3" }, { key: "7" }, { key: "12" }];
  assert.equal(itemForSeq(items, 7), "7");
  assert.equal(itemForSeq(items, 9), "7");
  assert.equal(itemForSeq(items, 40), "12");
  assert.equal(itemForSeq(items, 1), null);
});
