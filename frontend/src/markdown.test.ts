import { test } from "node:test";
import assert from "node:assert/strict";
import { parseInline, parseMarkdown, parseMarkdownAfter, safeHref, type Parsed } from "./markdown.ts";

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

// Review findings: a long run of spaces in a heading backtracked for
// seconds, and thousands of > overflowed the stack and blanked the window.
test("hostile headings and quotes stay fast and shallow", () => {
  let start = Date.now();
  const [h] = parseMarkdown("# Title" + " ".repeat(20000) + "end");
  assert.equal(h.type, "heading");
  assert.ok(Date.now() - start < 500, `heading took ${Date.now() - start} ms`);
  start = Date.now();
  assert.equal(parseMarkdown(">".repeat(20000) + " hi")[0].type, "quote");
  assert.ok(Date.now() - start < 2000, `quotes took ${Date.now() - start} ms`);
  assert.deepEqual(parseMarkdown("## Title ##"), parseMarkdown("## Title"));
  const [csharp] = parseMarkdown("# C#");
  assert.equal(csharp.type === "heading" && csharp.children[0].type === "text" ? csharp.children[0].text : "", "C#");
});

// Model output is untrusted: each of these took seconds to minutes at
// 200,000 characters, or grew faster than the text. Parsing must stay
// roughly linear, measured against the same input at 10,000 characters so
// a slow machine does not fail it.
const rep = (s: string, n: number) => s.repeat(Math.max(1, Math.floor(n / s.length)));
const hostile: Record<string, (n: number) => string> = {
  "a run of backticks": (n) => rep("`", n),
  "backticks then text": (n) => rep("`", n / 2) + rep("a", n / 2),
  "unclosed double backticks": (n) => rep("``a", n),
  "unclosed brackets": (n) => rep("[", n),
  "brackets then one link": (n) => rep("[", n) + "a](x)",
  "unclosed link targets": (n) => rep("[a](", n),
  "unclosed emphasis": (n) => "*" + rep("a ", n),
  "alternating delimiters": (n) => rep("*_", n),
  "alternating strong delimiters": (n) => rep("**__", n),
  "escapes": (n) => rep("\\", n),
  "angle brackets": (n) => rep("<", n),
  "a table row then spaces": (n) => "a|b\n" + " ".repeat(n) + "x",
  "a table separator then spaces": (n) => "a|b\n|---" + " ".repeat(n) + "x",
  "a fence then spaces": (n) => "```" + " ".repeat(n) + "x y",
  "a heading with a line separator": (n) => "# " + " ".repeat(n) + "a\u2028b",
  "a bullet with a line separator": (n) => "- " + " ".repeat(n) + "a\u2028b",
  "a numbered item with a line separator": (n) => "1. " + " ".repeat(n) + "a\u2028b",
  "nested quotes": (n) => rep(">", n) + " hi",
  "quotes on every line": (n) => rep("> > > > > > > > > > > > > > > > > > > > x\n", n),
  "rule-like dashes": (n) => rep("- ", n) + "x",
  "pipes": (n) => rep("|", n),
  "a wide table": (n) => rep("|a", n / 3) + "\n" + rep("|---", n / 3) + "\n" + rep("|x", n / 3),
  "one long paragraph": (n) => rep("word *a* _b_ `c` [d](https://e) \n", n),
};

function timed(s: string): number {
  let best = Infinity;
  for (let k = 0; k < 3; k++) {
    const start = performance.now();
    parseMarkdown(s);
    best = Math.min(best, performance.now() - start);
  }
  return best;
}

for (const [name, gen] of Object.entries(hostile)) {
  test(`hostile Markdown parses in linear time: ${name}`, () => {
    timed(gen(10_000)); // warm up
    const small = timed(gen(10_000));
    const large = timed(gen(200_000));
    assert.ok(large < 20 * small + 50, `${name}: ${large.toFixed(1)} ms at 200k against ${small.toFixed(1)} ms at 10k`);
    assert.ok(large < 2000, `${name}: ${large.toFixed(1)} ms at 200k`);
  });
}

test("what the linear parser defines for odd input", () => {
  // A backtick run with no closer as long is text up to the longest closer
  // after it, then a code span.
  assert.deepEqual(parseInline("```a``"), [
    { type: "text", text: "`" },
    { type: "code", text: "a" },
  ]);
  assert.deepEqual(parseInline("`a``b"), [
    { type: "code", text: "a" },
    { type: "text", text: "`b" },
  ]);
  assert.deepEqual(parseInline("``x"), [{ type: "text", text: "``x" }]);
  assert.deepEqual(parseInline("[[[a](https://x.dev)"), [
    { type: "link", href: "https://x.dev", children: [{ type: "text", text: "[[a" }] },
  ]);
  assert.deepEqual(parseInline("[a](b"), [{ type: "text", text: "[a](b" }]);
  // A line separator inside a line no longer stops it being a list item.
  assert.equal(parseMarkdown("- a\u2028b")[0].type, "list");
  // Fences and table separators read as before, spaces and all.
  assert.deepEqual(parseMarkdown("  ```  ts  \nx\n  ```  ")[0], { type: "code", lang: "ts", text: "x" });
  assert.equal(parseMarkdown("```ts x\ny")[0].type, "para");
  assert.equal(parseMarkdown("| a | b |\n | --- | :---: | \n| 1 | 2 |")[0].type, "table");
  assert.equal(parseMarkdown("a | b\n---|---")[0].type, "table");
  assert.equal(parseMarkdown("a | b\n|| ---")[0].type, "para");
  assert.equal(parseMarkdown("a | b\n--- ---")[0].type, "para");
  // Quotes deeper than the limit keep their remaining > as text.
  let q = parseMarkdown(">".repeat(20) + " hi")[0];
  let depth = 0;
  while (q.type === "quote" && q.children[0].type === "quote") {
    q = q.children[0];
    depth++;
  }
  assert.equal(depth, 16);
  assert.deepEqual(q.type === "quote" ? q.children[0] : null, { type: "para", children: [{ type: "text", text: ">>> hi" }] });
});

test("a streamed reply parses only its unfinished end again", () => {
  const reply =
    "# Plan\r\n\nFirst *this*,\nthen that.\n\n```go\nfunc main() {\n\n}\n```\n\n- one\n- two\n\n> quoted\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\nDone.\r\n\r\nEnd";
  for (const size of [1, 2, 3, 7, 13]) {
    let prev: Parsed | undefined;
    for (let at = 0; at < reply.length; ) {
      at = Math.min(reply.length, at + size);
      const next = parseMarkdownAfter(reply.slice(0, at), prev);
      assert.deepEqual(next.blocks, parseMarkdown(reply.slice(0, at)), `after ${at} characters in chunks of ${size}`);
      // Blocks before the last blank line keep their identity.
      if (prev) for (let k = 0; k < prev.stableBlocks; k++) assert.equal(next.blocks[k], prev.blocks[k]);
      prev = next;
    }
    assert.ok(prev!.stableBlocks >= 7, `stable blocks ${prev!.stableBlocks}`);
  }
  // Text that does not extend the last is parsed afresh.
  const first = parseMarkdownAfter("a\n\nb\n\nc");
  assert.deepEqual(parseMarkdownAfter("x\n\ny", first).blocks, parseMarkdown("x\n\ny"));
  assert.equal(parseMarkdownAfter("a\n\nb\n\nc", first), first);
});

test("streaming a long reply costs about what parsing it once does", () => {
  const reply = rep("Some **text** with `code` and [a link](https://x.dev).\n\n", 200_000);
  let prev: Parsed | undefined;
  const start = performance.now();
  for (let at = 0; at < reply.length; ) {
    at = Math.min(reply.length, at + 40);
    prev = parseMarkdownAfter(reply.slice(0, at), prev);
  }
  const elapsed = performance.now() - start;
  assert.ok(elapsed < 3000, `streaming 200k characters in 40-character chunks took ${elapsed.toFixed(0)} ms`);
});
