import { test } from "node:test";
import assert from "node:assert/strict";
import { describe as describeSchedule, when } from "./schedules.ts";

test("a schedule's times in words", () => {
  assert.equal(describeSchedule({ repeat: "daily", at: "09:00" }), "Every day at 09:00");
  assert.equal(describeSchedule({ repeat: "hourly", at: "00:15" }), "Every hour at :15");
  assert.equal(describeSchedule({ repeat: "weekly", at: "07:30", weekday: 3 }), "Wednesdays at 07:30");
  assert.equal(describeSchedule({ repeat: "weekdays", at: "18:00" }), "Weekdays at 18:00");
});

test("times relative to now", () => {
  const now = new Date(2026, 9, 30, 12, 0);
  assert.match(when(new Date(2026, 9, 30, 15, 0), now), /^today /);
  assert.match(when(new Date(2026, 9, 31, 9, 0), now), /^tomorrow /);
  assert.match(when(new Date(2026, 10, 2, 9, 0), now), /^Monday /);
  assert.equal(when(undefined, now), "");
  assert.equal(when("0001-01-01T00:00:00Z", now), "");
});
