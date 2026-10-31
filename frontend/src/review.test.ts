import { test } from "node:test";
import assert from "node:assert/strict";
import { parseDiff } from "./diff.ts";
import { anchor, locate, reviewMessage, type ReviewComment } from "./review.ts";

const diff = `diff --git a/a.ts b/a.ts
--- a/a.ts
+++ b/a.ts
@@ -1,5 +1,5 @@
 one
 two
-three
+THREE
 four
 five
`;

test("a comment anchors to its line with two lines around it", () => {
  const rows = parseDiff(diff);
  const i = rows.findIndex((r) => r.text === "THREE");
  const a = anchor(rows, i)!;
  assert.equal(a.side, "new");
  assert.equal(a.line, 3);
  assert.deepEqual(
    a.context.map((x) => x.text),
    ["two", "three", "THREE", "four", "five"],
  );
  assert.equal(a.context[a.at].text, "THREE");
  const d = anchor(rows, rows.findIndex((r) => r.text === "three"))!;
  assert.equal(d.side, "old");
  assert.equal(d.line, 3);
  assert.equal(anchor(rows, 0), null, "the hunk header takes no comment");
});

test("a comment is found again after lines move, and lost when its line goes", () => {
  const rows = parseDiff(diff);
  const c = anchor(rows, rows.findIndex((r) => r.text === "THREE"))!;
  const moved = parseDiff(diff.replace("@@ -1,5 +1,5 @@\n one", "@@ -1,5 +1,6 @@\n zero\n one").replace(" zero", "+zero"));
  const i = locate(moved, c);
  assert.equal(moved[i].text, "THREE");
  assert.equal(moved[i].newNo, 4);
  assert.equal(locate(parseDiff(diff.replace("+THREE", "+3")), c), -1);
});

test("the review message names each place and quotes the diff", () => {
  const rows = parseDiff(diff);
  const c: ReviewComment = { id: "1", path: "a.ts", body: "  Keep it lower case. ", ...anchor(rows, rows.findIndex((r) => r.text === "THREE"))! };
  const d: ReviewComment = { id: "2", path: "b.ts", body: "Why ```this```?", side: "old", line: 9, text: "x", context: [{ kind: "del", text: "x ``` y" }], at: 0 };
  const m = reviewMessage([c, d]);
  assert.ok(m.startsWith("2 review comments on your changes:"));
  assert.ok(m.includes("1. a.ts:3\n```diff\n two\n-three\n+THREE\n four\n five\n```\nKeep it lower case."), m);
  assert.ok(m.includes("2. b.ts:9 (a removed line)\n````diff\n-x ``` y\n````\nWhy ```this```?"), m);
  assert.equal(reviewMessage([]), "");
  // By file, then line, whatever order they were written in.
  const later: ReviewComment = { ...c, id: "3", line: 1, body: "first line" };
  const order = reviewMessage([c, d, later]).split("\n").filter((l) => /^\d+\. /.test(l));
  assert.deepEqual(order, ["1. a.ts:1", "2. a.ts:3", "3. b.ts:9 (a removed line)"]);
});
