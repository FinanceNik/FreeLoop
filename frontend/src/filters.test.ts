import { test } from "node:test";
import assert from "node:assert/strict";
import { allTags, isEmpty, matches, normalizeTags, parseQuery, sameFilter, saveFilter } from "./filters.ts";

const s = (title: string, tags: string[], status = "idle") => ({ title, projectRoot: "/code/app", branch: "main", status, tags });

test("tags are cleaned like the engine cleans them", () => {
  assert.deepEqual(normalizeTags([" #Bug ", "needs review", "bug", ""]), ["bug", "needs-review"]);
  assert.equal(normalizeTags(["x".repeat(40)])[0].length, 32);
});

test("the query's #words are tags", () => {
  assert.deepEqual(parseQuery("#Bug login  #ui"), { text: "login", tags: ["bug", "ui"] });
  assert.deepEqual(parseQuery("# alone"), { text: "# alone", tags: [] });
});

test("a session matches every part of a filter", () => {
  const f = { query: "", status: "", tags: ["bug"] };
  assert.ok(matches(s("Fix login", ["bug", "ui"]), f));
  assert.ok(!matches(s("Fix login", ["ui"]), f));
  assert.ok(matches(s("Fix login", ["bug"]), { query: "#bug login", status: "", tags: [] }));
  assert.ok(!matches(s("Fix login", ["bug"]), { query: "#bug signup", status: "", tags: [] }));
  assert.ok(!matches(s("Fix login", ["bug"], "running"), { ...f, status: "idle" }));
  assert.ok(matches({ ...s("Old", []), tags: null }, { query: "", status: "", tags: [] }));
});

test("tags in use, the most used first", () => {
  assert.deepEqual(allTags([s("a", ["ui", "bug"]), s("b", ["bug"]), { tags: null }]), ["bug", "ui"]);
});

test("saved filters replace one of the same name", () => {
  const f = { query: " login ", status: "", tags: ["Bug"] };
  let list = saveFilter([], "Bugs", f);
  assert.deepEqual(list, [{ name: "Bugs", query: "login", status: "", tags: ["bug"] }]);
  list = saveFilter(list, "bugs", { query: "", status: "error", tags: [] });
  assert.equal(list.length, 1);
  assert.equal(list[0].status, "error");
  assert.ok(sameFilter(list[0], { query: "", status: "error", tags: [] }));
  assert.ok(isEmpty({ query: " ", status: "", tags: [] }));
});
