import { test } from "node:test";
import assert from "node:assert/strict";
import { rank, score, type PaletteItem } from "./palette.ts";

const item = (title: string, group = "Action", keywords?: string): PaletteItem => ({ id: title, title, group, keywords, run: () => {} });

test("score prefers whole, prefix, word and substring matches over fuzzy", () => {
  assert.ok(score("settings", "Settings") > score("set", "Settings"));
  assert.ok(score("set", "Settings") > score("set", "Reset"));
  assert.ok(score("diff", "Show diff") > score("iff", "Show diff"));
  assert.ok(score("iff", "Show diff") > score("sdf", "Show diff"));
  assert.ok(score("sdf", "Show diff") > 0);
  assert.equal(score("xyz", "Show diff"), 0);
  assert.ok(score("diff show", "Show the diff") > 0);
});

test("rank filters, orders by match and keeps the given order on ties", () => {
  const items = [item("Fix the login bug", "Session"), item("Providers", "Page"), item("Profiles", "Page"), item("Archive session", "Action", "hide")];
  assert.deepEqual(rank(items, "").map((i) => i.title), items.map((i) => i.title));
  assert.deepEqual(rank(items, "pro").map((i) => i.title), ["Providers", "Profiles"]);
  assert.deepEqual(rank(items, "hide").map((i) => i.title), ["Archive session"]);
  assert.deepEqual(rank(items, "page").map((i) => i.title), ["Providers", "Profiles"]);
  assert.equal(rank(items, "login")[0].title, "Fix the login bug");
  assert.equal(rank(items, "qqq").length, 0);
});
