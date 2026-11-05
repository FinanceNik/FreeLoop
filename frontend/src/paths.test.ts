import assert from "node:assert/strict";
import { test } from "node:test";
import { basename, relative, isMarkdownFile, isPdfFile } from "./paths.ts";

test("basename is the last part", () => {
  assert.equal(basename("/a/b/c"), "c");
  assert.equal(basename("/a/b/"), "b");
  assert.equal(basename("/"), "/");
});

test("relative strips the root and a leading ./", () => {
  assert.equal(relative("/repo/a/b.go", "/repo"), "a/b.go");
  assert.equal(relative("/repo/a/b.go", "/repo/"), "a/b.go");
  assert.equal(relative("./a.go", "/repo"), "a.go");
  assert.equal(relative("/elsewhere/a.go", "/repo"), "/elsewhere/a.go");
  assert.equal(relative("/repository/a.go", "/repo"), "/repository/a.go");
});

test("the viewer knows Markdown and PDF files by their names", () => {
  for (const md of ["README.md", "docs/a.MD", "notes.markdown"]) assert.ok(isMarkdownFile(md), md);
  for (const other of ["md", "a.md.txt", "a.mdx", "README"]) assert.ok(!isMarkdownFile(other), other);
  assert.ok(isPdfFile("paper.PDF"));
  assert.ok(!isPdfFile("paper.pdf.txt"));
});
