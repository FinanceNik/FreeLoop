import { test } from "node:test";
import assert from "node:assert/strict";
import { approvalShortcut, optionKind } from "./shortcuts.ts";

const k = (key: string, mods: Partial<{ metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean }> = {}) => ({
  key,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  ...mods,
});

test("letters answer only outside text fields", () => {
  assert.equal(approvalShortcut(k("y"), false), "allow_once");
  assert.equal(approvalShortcut(k("a"), false), "allow_always");
  assert.equal(approvalShortcut(k("n"), false), "reject");
  assert.equal(approvalShortcut(k("y"), true), null);
  assert.equal(approvalShortcut(k("Y", { shiftKey: true }), false), null);
  assert.equal(approvalShortcut(k("x"), false), null);
});

test("command forms work in text fields", () => {
  assert.equal(approvalShortcut(k("Enter", { metaKey: true }), true), "allow_once");
  assert.equal(approvalShortcut(k("Enter", { metaKey: true, shiftKey: true }), true), "allow_always");
  assert.equal(approvalShortcut(k("Backspace", { metaKey: true, shiftKey: true }), true), "reject");
  assert.equal(approvalShortcut(k("Backspace", { metaKey: true }), true), null);
  assert.equal(approvalShortcut(k("Enter", { metaKey: true, ctrlKey: true }), false), null);
  assert.equal(approvalShortcut(k("y", { metaKey: true }), false), null);
});

test("option kinds", () => {
  assert.equal(optionKind("allow_once"), "allow_once");
  assert.equal(optionKind("reject_once"), "reject");
  assert.equal(optionKind("reject_always"), "reject");
  assert.equal(optionKind("other"), null);
});
