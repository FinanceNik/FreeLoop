import { test } from "node:test";
import assert from "node:assert/strict";
import { allTags, isEmpty, matches, normalizeTags, parseQuery, sameFilter, saveFilter, statusSummary, dateGroups } from "./filters.ts";
import { setLanguage } from "./i18n.ts";

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

test("statusSummary counts statuses with what needs the user first", () => {
  const list = [{ status: "running" }, { status: "error" }, { status: "waiting_permission" }, { status: "running" }];
  assert.deepEqual(statusSummary(list), [
    { status: "waiting_permission", count: 1 },
    { status: "error", count: 1 },
    { status: "running", count: 2 },
  ]);
  assert.deepEqual(statusSummary([]), []);
});

test("dateGroups sorts sessions into calendar days by when they were last used", () => {
  const now = new Date(2026, 10, 3, 9, 30);
  const at = (id: string, d: Date | string) => ({ id, updatedAt: typeof d === "string" ? d : d.toISOString() });
  const list = [
    at("older", new Date(2026, 8, 1)),
    at("day31", new Date(2026, 9, 3, 23, 59)),
    at("day30", new Date(2026, 9, 4, 0, 0)),
    at("day2", new Date(2026, 10, 1, 23, 59)),
    at("yesterday-late", new Date(2026, 10, 2, 23, 59)),
    at("yesterday-early", new Date(2026, 10, 2, 0, 0)),
    at("today-early", new Date(2026, 10, 3, 0, 0)),
    at("today-late", new Date(2026, 10, 3, 9, 0)),
    at("future", new Date(2026, 10, 4, 12, 0)),
    at("broken", "not a date"),
  ];
  const shape = dateGroups(list, now).map((g) => [g.label, g.sessions.map((s) => s.id)]);
  assert.deepEqual(shape, [
    ["Today", ["future", "today-late", "today-early"]],
    ["Yesterday", ["yesterday-late", "yesterday-early"]],
    ["Previous 7 days", ["day2"]],
    ["Previous 30 days", ["day30"]],
    ["Older", ["day31", "older", "broken"]],
  ]);
  assert.deepEqual(dateGroups([], now), []);
});

test("dateGroups keeps its order among sessions used in the same minute", () => {
  const now = new Date(2026, 10, 3, 9, 30);
  const a = { id: "a", updatedAt: new Date(2026, 10, 3, 9, 0, 1).toISOString() };
  const b = { id: "b", updatedAt: new Date(2026, 10, 3, 9, 0, 40).toISOString() };
  assert.deepEqual(dateGroups([b, a], now)[0].sessions.map((s) => s.id), ["a", "b"]);
  assert.deepEqual(dateGroups([{ ...a, updatedAt: new Date(2026, 10, 3, 9, 0, 59).toISOString() }, b], now)[0].sessions.map((s) => s.id), ["a", "b"]);
});

test("dateGroups names its days in the language in force, and keeps an id that does not change with it (GUI-048)", () => {
  const now = new Date(2026, 10, 3, 9, 30);
  const list = [
    { id: "today", updatedAt: new Date(2026, 10, 3, 9, 0).toISOString() },
    { id: "yesterday", updatedAt: new Date(2026, 10, 2, 9, 0).toISOString() },
    { id: "week", updatedAt: new Date(2026, 9, 30, 9, 0).toISOString() },
    { id: "month", updatedAt: new Date(2026, 9, 10, 9, 0).toISOString() },
    { id: "older", updatedAt: new Date(2026, 5, 1).toISOString() },
  ];
  const english = dateGroups(list, now).map((g) => g.id);
  setLanguage("de");
  const german = dateGroups(list, now);
  assert.deepEqual(
    german.map((g) => g.label),
    ["Heute", "Gestern", "Letzte 7 Tage", "Letzte 30 Tage", "Älter"],
  );
  assert.deepEqual(german.map((g) => g.id), english);
  setLanguage("nl");
  assert.deepEqual(
    dateGroups(list, now).map((g) => g.label),
    ["Vandaag", "Gisteren", "Afgelopen 7 dagen", "Afgelopen 30 dagen", "Ouder"],
  );
  setLanguage("en");
});
