import assert from "node:assert/strict";
import { test } from "node:test";
import { basename, isAbsolute, nameStart, relative, isMarkdownFile, isPdfFile } from "./paths.ts";

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

test("Windows paths: either separator, a drive letter in any case", () => {
  assert.equal(basename("C:\\Users\\x\\proj\\src\\a.ts"), "a.ts");
  assert.equal(basename("C:/Users/x/proj/"), "proj");
  assert.equal(basename("\\\\server\\share\\repo"), "repo");
  assert.equal(relative("C:\\Users\\x\\proj\\src\\a.ts", "C:\\Users\\x\\proj"), "src/a.ts");
  assert.equal(relative("c:/users/x/proj/src/a.ts", "C:\\Users\\x\\proj\\"), "src/a.ts");
  assert.equal(relative(".\\src\\a.ts", "C:\\proj"), "src/a.ts");
  assert.equal(relative("C:\\projector\\a.ts", "C:\\proj"), "C:\\projector\\a.ts");
  assert.equal(relative("D:\\other\\a.ts", "C:\\proj"), "D:\\other\\a.ts");
  // A backslash in a macOS or Linux name is part of the name.
  assert.equal(relative("/repo/a\\b.go", "/repo"), "a\\b.go");
});

test("isAbsolute knows each system's roots", () => {
  for (const p of ["/usr/bin", "C:\\Users", "c:/Users", "\\\\server\\share"]) assert.ok(isAbsolute(p), p);
  for (const p of ["src/a.ts", "src\\a.ts", "./a", "C:", "C:file"]) assert.ok(!isAbsolute(p), p);
});

test("nameStart finds the file name after either separator", () => {
  assert.equal(nameStart("src/a.ts"), 4);
  assert.equal(nameStart("src\\lib\\a.ts"), 8);
  assert.equal(nameStart("a.ts"), 0);
});

test("the viewer knows Markdown and PDF files by their names", () => {
  for (const md of ["README.md", "docs/a.MD", "notes.markdown"]) assert.ok(isMarkdownFile(md), md);
  for (const other of ["md", "a.md.txt", "a.mdx", "README"]) assert.ok(!isMarkdownFile(other), other);
  assert.ok(isPdfFile("paper.PDF"));
  assert.ok(!isPdfFile("paper.pdf.txt"));
});
