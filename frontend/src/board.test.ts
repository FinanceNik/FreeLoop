import { test } from "node:test";
import assert from "node:assert/strict";
import { columns, since } from "./board.ts";

test("sessions go to their status's column, newest first", () => {
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
      ["needs", ["d", "b"]],
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
