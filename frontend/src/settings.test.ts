import { test } from "node:test";
import assert from "node:assert/strict";
import { SETTINGS_GROUPS, inSettings, isSettingsShortcut } from "./settings.ts";

test("every configuration page is listed once", () => {
  const ids = SETTINGS_GROUPS.flatMap((g) => g.pages.map((p) => p.id));
  assert.deepEqual([...ids].sort(), ["environments", "mcp", "profiles", "providers", "rules", "schedules", "settings", "skills"]);
  assert.equal(new Set(ids).size, ids.length);
});

test("the settings area covers its pages and the import page", () => {
  assert.equal(inSettings("providers"), true);
  assert.equal(inSettings("settings"), true);
  assert.equal(inSettings("import"), true);
  assert.equal(inSettings("session"), false);
  assert.equal(inSettings("attention"), false);
  assert.equal(inSettings("new"), false);
});

test("command-comma opens the settings", () => {
  const k = { key: ",", metaKey: true, ctrlKey: false, altKey: false, shiftKey: false };
  assert.equal(isSettingsShortcut(k), true);
  assert.equal(isSettingsShortcut({ ...k, metaKey: false }), false);
  assert.equal(isSettingsShortcut({ ...k, shiftKey: true }), false);
  assert.equal(isSettingsShortcut({ ...k, key: "." }), false);
});
