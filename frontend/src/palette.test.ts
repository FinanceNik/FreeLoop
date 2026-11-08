import { test } from "node:test";
import assert from "node:assert/strict";
import { PER_GROUP, rank, score, sections, type PaletteItem } from "./palette.ts";

const item = (title: string, group = "Action", keywords?: string, detail?: string): PaletteItem => ({ id: `${group}:${title}`, title, group, keywords, detail, run: () => {} });

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

test("a long description does not match by scattered letters", () => {
  const long = item("/dataviz", "Command", undefined, "Use this skill whenever you are about to create ANY chart, graph, plot, dashboard, or data visualization");
  const page = item("Settings", "Page");
  // "set" is in the description's letters in order, not as a word.
  assert.deepEqual(rank([long, page], "set").map((i) => i.title), ["Settings"]);
  // A word of the description still finds it.
  assert.deepEqual(rank([long, page], "chart").map((i) => i.title), ["/dataviz"]);
  // And the title matches loosely.
  assert.deepEqual(rank([long, page], "dtvz").map((i) => i.title), ["/dataviz"]);
});

const many = [
  item("Allow once: write notes.md", "Approval"),
  item("New session"),
  item("Try the demo"),
  ...Array.from({ length: 9 }, (_, n) => item(`Session ${n}`, "Session")),
  item("Settings", "Page"),
  item("Usage", "Page"),
  item("Mode: Plan", "This session"),
  item("Show sessions: Running", "Filter"),
  item("freeloop", "New session", "new session start create", "/Users/me/freeloop"),
  ...Array.from({ length: 20 }, (_, n) => item(`/skill-${n}`, "Command", undefined, "a long description of an agent skill")),
];

test("before typing, only the short sections, in order, with five recent sessions", () => {
  const got = sections(many, "");
  assert.deepEqual(
    got.map((s) => s.label),
    ["Waiting for you", "Actions", "Recent sessions", "Go to"],
  );
  assert.equal(got.find((s) => s.group === "Session")?.items.length, 5);
  assert.ok(!got.some((s) => s.group === "Command" || s.group === "Filter" || s.group === "This session"));
});

test("a slash searches the agent's commands only, all of them", () => {
  const got = sections(many, "/");
  assert.deepEqual(got.map((s) => s.label), ["Agent commands"]);
  assert.equal(got[0].items.length, 20);
  assert.deepEqual(sections(many, "/skill-1").flatMap((s) => s.items.map((i) => i.title)).slice(0, 1), ["/skill-1"]);
  assert.deepEqual(sections(many, "/nothing"), []);
});

test("a search groups its results, best group first, a few of each", () => {
  const got = sections(many, "session");
  assert.ok(got.every((s) => s.items.length <= PER_GROUP));
  assert.equal(got.find((s) => s.group === "Session")?.items.length, PER_GROUP);
  assert.equal(got[0].group, "Session", "titles starting with the query come first");
  assert.equal(sections(many, "new")[0].items[0].title, "New session");
  assert.ok(got.some((s) => s.label === "New session in" && s.items[0].title === "freeloop"));
  assert.ok(!got.some((s) => s.group === "Command"), "a description's letters do not bring in the agent's commands");
});
