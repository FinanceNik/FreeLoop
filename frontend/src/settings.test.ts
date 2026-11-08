import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_FONT_SIZE, MIN_FONT_SIZE, SETTINGS_GROUPS, inSettings, isGeneral, settingsStart, isSettingsShortcut, textSizeAfter, autoSaved, settingsKey } from "./settings.ts";

test("every configuration page is listed once", () => {
  const ids = SETTINGS_GROUPS.flatMap((g) => g.pages.map((p) => p.id));
  assert.deepEqual([...ids].sort(), ["appearance", "costs", "data", "environments", "keyboard", "mcp", "network", "optimizers", "permissions", "profiles", "providers", "rules", "schedules", "sessions", "skills", "workflows"]);
  assert.equal(new Set(ids).size, ids.length);
});

test("the settings area covers its pages and the import page", () => {
  assert.equal(inSettings("providers"), true);
  assert.equal(inSettings("costs"), true);
  assert.equal(inSettings("settings"), false);
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

test("the General page saves by itself, but not hooks, prices, budgets or the search settings", () => {
  const saved = { permissions: { allowBypass: false }, hooks: ["old"], prices: [1], budgets: [10], web: { searchURL: "a" }, commandApprovals: ["x"] };
  const form = { ...saved, permissions: { allowBypass: true }, hooks: ["half typed"], prices: [2], budgets: [0], web: { searchURL: "https://ha" } };
  const next = autoSaved(form, saved);
  assert.equal(next.permissions.allowBypass, true);
  assert.deepEqual(next.hooks, ["old"]);
  assert.deepEqual(next.prices, [1]);
  assert.deepEqual(next.budgets, [10]);
  assert.deepEqual(next.web, { searchURL: "a" });
});

test("approvals are not an edit of the settings", () => {
  const a = { permissions: { allowBypass: true }, commandApprovals: ["x"] };
  assert.equal(settingsKey(a), settingsKey({ ...a, commandApprovals: ["x", "y"] }));
  assert.notEqual(settingsKey(a), settingsKey({ ...a, permissions: { allowBypass: false } }));
});

test("the settings open on the page last shown, or Appearance", () => {
  assert.equal(settingsStart(null), "appearance");
  assert.equal(settingsStart("costs"), "costs");
  assert.equal(settingsStart("rules"), "rules");
  assert.equal(settingsStart("import"), "appearance");
  assert.equal(settingsStart("session"), "appearance");
  assert.equal(isGeneral("network"), true);
  assert.equal(isGeneral("rules"), false);
});
