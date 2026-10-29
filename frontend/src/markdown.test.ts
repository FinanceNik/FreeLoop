import { test } from "node:test";
import assert from "node:assert/strict";
import { parseInline, parseMarkdown, safeHref } from "./markdown.ts";

test("blocks", () => {
  const b = parseMarkdown("# Title\n\nSome *text* here.\nSecond line.\n\n- one\n- two `x`\n\n1. a\n2. b\n\n```go\nfunc main() {}\n```\n\n> quoted\n\n---\n");
  assert.deepEqual(
    b.map((x) => x.type),
    ["heading", "para", "list", "list", "code", "quote", "hr"],
  );
  assert.deepEqual(b[4], { type: "code", lang: "go", text: "func main() {}" });
  const list = b[2] as Extract<(typeof b)[number], { type: "list" }>;
  assert.equal(list.items.length, 2);
  assert.equal((b[3] as any).ordered, true);
});

test("an unterminated fence streams as code", () => {
  const b = parseMarkdown("Here:\n```ts\nconst a = 1;\nconst b");
  assert.equal(b[1].type, "code");
  assert.equal((b[1] as any).text, "const a = 1;\nconst b");
});

test("tables", () => {
  const b = parseMarkdown("| File | Change |\n|---|:---:|\n| a.go | added |\n| b.go | **fixed** |\n");
  assert.equal(b[0].type, "table");
  const t = b[0] as any;
  assert.equal(t.header.length, 2);
  assert.equal(t.rows.length, 2);
  assert.equal(t.rows[1][1][0].type, "strong");
});

test("inline", () => {
  assert.deepEqual(parseInline("use `go test ./...` and **run** it"), [
    { type: "text", text: "use " },
    { type: "code", text: "go test ./..." },
    { type: "text", text: " and " },
    { type: "strong", children: [{ type: "text", text: "run" }] },
    { type: "text", text: " it" },
  ]);
  assert.deepEqual(parseInline("a snake_case_name stays"), [{ type: "text", text: "a snake_case_name stays" }]);
  assert.deepEqual(parseInline("2 * 3 * 4"), [{ type: "text", text: "2 * 3 * 4" }]);
  assert.deepEqual(parseInline("see [docs](https://x.dev)")[1], {
    type: "link",
    href: "https://x.dev",
    children: [{ type: "text", text: "docs" }],
  });
  assert.deepEqual(parseInline("\\*not em\\*"), [{ type: "text", text: "*not em*" }]);
});

test("markup in model output stays text", () => {
  const b = parseMarkdown('<img src=x onerror="alert(1)"> <script>bad()</script>');
  assert.equal(b[0].type, "para");
  assert.deepEqual((b[0] as any).children, [{ type: "text", text: '<img src=x onerror="alert(1)"> <script>bad()</script>' }]);
});

test("only http links are followed", () => {
  assert.equal(safeHref("https://example.com"), "https://example.com");
  assert.equal(safeHref("javascript:alert(1)"), null);
  assert.equal(safeHref("file:///etc/passwd"), null);
  assert.equal(safeHref("/wails/runtime"), null);
});
