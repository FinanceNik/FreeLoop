import { test } from "node:test";
import assert from "node:assert/strict";
import { currentOf, editPreview, moved, oldestFirst } from "./attentionview.ts";

const r = (id: string, at: string) => ({ requestId: id, createdAt: at });

test("requests across sessions, the longest wait first", () => {
  const list = oldestFirst([[r("b", "2026-11-03T10:00:00Z")], null, [r("a", "2026-11-03T09:00:00Z"), r("c", "2026-11-03T11:00:00Z")]]);
  assert.deepEqual(list.map((x) => x.requestId), ["a", "b", "c"]);
});

test("the keys act on the chosen request while it waits, then on the longest wait", () => {
  const list = [r("a", ""), r("b", "")];
  assert.equal(currentOf(list, "b"), "b");
  assert.equal(currentOf(list, "gone"), "a");
  assert.equal(currentOf(list, null), "a");
  assert.equal(currentOf([], "a"), null);
});

test("moving stops at the ends", () => {
  const list = [r("a", ""), r("b", "")];
  assert.equal(moved(list, "a", 1), "b");
  assert.equal(moved(list, "b", 1), "b");
  assert.equal(moved(list, "a", -1), "a");
  assert.equal(moved([], "a", 1), null);
});

test("an edit's change is found in the spellings in use", () => {
  assert.deepEqual(editPreview({ path: "a", old_string: "x", new_string: "y" }), { oldText: "x", newText: "y" });
  assert.deepEqual(editPreview({ oldText: "x", newText: "y" }), { oldText: "x", newText: "y" });
  assert.deepEqual(editPreview({ path: "a", content: "new" }), { oldText: "", newText: "new" });
  assert.equal(editPreview({ command: "ls" }), null);
  assert.equal(editPreview(null), null);
  assert.equal(editPreview("text"), null);
});
