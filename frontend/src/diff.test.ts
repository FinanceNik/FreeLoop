import { test } from "node:test";
import assert from "node:assert/strict";
import { diffStats, parseDiff } from "./diff.ts";

const sample = `diff --git a/a.txt b/a.txt
index 2e65efe..5ea2ed4 100644
--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
@@ -10 +10,2 @@ func x() {
 ten
+eleven
\\ No newline at end of file
`;

test("parseDiff numbers lines per hunk and drops file headers", () => {
  const rows = parseDiff(sample);
  assert.deepEqual(
    rows.map((r) => [r.kind, r.text, r.oldNo ?? null, r.newNo ?? null]),
    [
      ["hunk", "@@ -1,3 +1,3 @@", null, null],
      ["ctx", "one", 1, 1],
      ["del", "two", 2, null],
      ["add", "TWO", null, 2],
      ["ctx", "three", 3, 3],
      ["hunk", "@@ -10 +10,2 @@ func x() {", null, null],
      ["ctx", "ten", 10, 10],
      ["add", "eleven", null, 11],
      ["meta", "\\ No newline at end of file", null, null],
    ],
  );
  assert.deepEqual(diffStats(rows), { added: 2, deleted: 1 });
});

test("parseDiff keeps binary notices and handles an empty diff", () => {
  const rows = parseDiff("diff --git a/x.png b/x.png\nindex 1..2 100644\nBinary files a/x.png and b/x.png differ\n");
  assert.deepEqual(rows, [{ kind: "meta", text: "Binary files a/x.png and b/x.png differ" }]);
  assert.deepEqual(parseDiff(""), []);
});

test("a new file from /dev/null is all additions", () => {
  const rows = parseDiff("diff --git a/b b/b\nnew file mode 100644\n--- /dev/null\n+++ b/b\n@@ -0,0 +1 @@\n+b\n");
  assert.deepEqual(
    rows.map((r) => r.kind),
    ["meta", "hunk", "add"],
  );
  assert.deepEqual(rows[2], { kind: "add", text: "b", newNo: 1 });
});
