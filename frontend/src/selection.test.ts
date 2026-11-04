import { test } from "node:test";
import assert from "node:assert/strict";
import { clearsSelection } from "./selection.ts";

const click = { had: true, clicks: 1, moved: 0, shift: false, ownSelection: false };

test("a plain click clears a selection", () => {
  assert.equal(clearsSelection(click), true);
  assert.equal(clearsSelection({ ...click, moved: 3 }), true);
});

test("what selects by hand keeps working", () => {
  assert.equal(clearsSelection({ ...click, moved: 40 }), false); // a drag
  assert.equal(clearsSelection({ ...click, clicks: 2 }), false); // a word
  assert.equal(clearsSelection({ ...click, clicks: 3 }), false); // a line
  assert.equal(clearsSelection({ ...click, shift: true }), false); // extending it
});

test("nothing to clear, or a selection of the field's own, is left alone", () => {
  assert.equal(clearsSelection({ ...click, had: false }), false);
  assert.equal(clearsSelection({ ...click, ownSelection: true }), false);
});
