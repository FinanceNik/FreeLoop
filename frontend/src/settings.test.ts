import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_FONT_SIZE, MIN_FONT_SIZE, SETTINGS_GROUPS, inSettings, isSettingsShortcut, textSizeAfter, autoSaved, settingsKey } from "./settings.ts";

test("every configuration page is listed once", () => {
  const ids = SETTINGS_GROUPS.flatMap((g) => g.pages.map((p) => p.id));
  assert.deepEqual([...ids].sort(), ["environments", "mcp", "optimizers", "profiles", "providers", "rules", "schedules", "settings", "skills"]);
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


const keys = (key: string, more = {}) => ({ key, metaKey: true, ctrlKey: false, altKey: false, shiftKey: false, ...more });

test("the text size shortcuts", () => {
  assert.equal(textSizeAfter(keys("="), 14), 15);
  assert.equal(textSizeAfter(keys("+"), 14), 15);
  assert.equal(textSizeAfter(keys("-"), 14), 13);
  assert.equal(textSizeAfter(keys("="), MAX_FONT_SIZE), MAX_FONT_SIZE);
  assert.equal(textSizeAfter(keys("-"), MIN_FONT_SIZE), MIN_FONT_SIZE);
  assert.equal(textSizeAfter(keys("0"), 14), null);
  assert.equal(textSizeAfter(keys("=", { metaKey: false }), 14), null);
  assert.equal(textSizeAfter(keys("=", { altKey: true }), 14), null);
  assert.equal(textSizeAfter(keys("-", { ctrlKey: true }), 14), null);
});

test("the General page saves by itself, but not hooks, prices or the search settings", () => {
  const saved = { permissions: { allowBypass: false }, hooks: ["old"], prices: [1], web: { searchURL: "a" }, commandApprovals: ["x"] };
  const form = { ...saved, permissions: { allowBypass: true }, hooks: ["half typed"], prices: [2], web: { searchURL: "https://ha" } };
  const next = autoSaved(form, saved);
  assert.equal(next.permissions.allowBypass, true);
  assert.deepEqual(next.hooks, ["old"]);
  assert.deepEqual(next.prices, [1]);
  assert.deepEqual(next.web, { searchURL: "a" });
});

test("approvals are not an edit of the settings", () => {
  const a = { permissions: { allowBypass: true }, commandApprovals: ["x"] };
  assert.equal(settingsKey(a), settingsKey({ ...a, commandApprovals: ["x", "y"] }));
  assert.notEqual(settingsKey(a), settingsKey({ ...a, permissions: { allowBypass: false } }));
});
