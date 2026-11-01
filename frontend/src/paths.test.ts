import assert from "node:assert/strict";
import { test } from "node:test";
import { basename, relative } from "./paths.ts";

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
