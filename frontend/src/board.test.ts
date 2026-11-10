import { test } from "node:test";
import assert from "node:assert/strict";
import { columns, since } from "./board.ts";
import { setLanguage } from "./i18n.ts";

test("sessions go to their status's column, newest first, and the longest wait first where they need the user", () => {
  const cols = columns([
    { id: "a", status: "idle", updatedAt: "2026-10-31T10:00:00Z" },
    { id: "b", status: "error", updatedAt: "2026-10-31T09:00:00Z" },
    { id: "c", status: "idle", updatedAt: "2026-10-31T11:00:00Z" },
    { id: "d", status: "waiting_permission", updatedAt: "2026-10-31T12:00:00Z" },
    { id: "e", status: "archived", updatedAt: "2026-10-31T12:00:00Z" },
    { id: "f", status: "running", updatedAt: "2026-10-31T08:00:00Z" },
  ]);
  assert.deepEqual(
    cols.map((c) => [c.key, c.sessions.map((s) => s.id)]),
    [
      ["needs", ["b", "d"]],
      ["running", ["f"]],
      ["idle", ["c", "a"]],
    ],
  );
});

test("how long ago, in words", () => {
  const now = new Date("2026-10-31T12:00:00Z");
  assert.equal(since("2026-10-31T11:59:30Z", now), "just now");
  assert.equal(since("2026-10-31T11:15:00Z", now), "45 min ago");
  assert.equal(since("2026-10-31T07:00:00Z", now), "5 h ago");
  assert.equal(since("2026-10-30T10:00:00Z", now), "yesterday");
  assert.equal(since("2026-10-27T12:00:00Z", now), "4 days ago");
});

// GUI-048: the columns and the times follow the language.
test("the columns and how long ago are in the language in force", () => {
  const now = new Date("2026-10-31T12:00:00Z");
  assert.deepEqual(columns([]).map((c) => c.title), ["Needs you", "Running", "Idle"]);
  setLanguage("de");
  assert.deepEqual(columns([]).map((c) => c.title), ["Handlungsbedarf", "Läuft", "Inaktiv"]);
  assert.equal(since("2026-10-31T11:15:00Z", now), "vor 45 min");
  assert.equal(since("2026-10-27T12:00:00Z", now), "vor 4 Tagen");
  setLanguage("nl");
  assert.deepEqual(columns([]).map((c) => c.title), ["Actie nodig", "Bezig", "Inactief"]);
  assert.equal(since("2026-10-31T07:00:00Z", now), "5 uur geleden");
  assert.equal(since("2026-10-30T10:00:00Z", now), "gisteren");
  setLanguage("en");
});
