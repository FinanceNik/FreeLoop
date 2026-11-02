import { test } from "node:test";
import assert from "node:assert/strict";
import { approvalHint, approvalShortcut, optionLabel, bindingOf, bindingProblem, comboOf, isTextField, keyLabel, optionKind, pressed, setBindings } from "./shortcuts.ts";

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

const press = (key: string, more = {}) => ({ key, metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...more });

test("a key press is written the way bindings are", () => {
  assert.equal(comboOf(press("k", { metaKey: true })), "Meta+K");
  assert.equal(comboOf(press("K", { metaKey: true, shiftKey: true })), "Meta+Shift+K");
  assert.equal(comboOf(press("Meta", { metaKey: true })), "");
  assert.equal(keyLabel("Meta+Shift+K"), "⌘⇧K");
  assert.equal(keyLabel("Meta+Shift++"), "⌘⇧+");
  assert.equal(keyLabel("y"), "Y");
});

test("a shortcut can be changed, and goes back when the change is gone", () => {
  assert.ok(pressed(press("k", { metaKey: true }), "palette"));
  setBindings({ palette: "Meta+P" });
  assert.ok(pressed(press("p", { metaKey: true }), "palette"));
  assert.ok(!pressed(press("k", { metaKey: true }), "palette"));
  setBindings({});
  assert.ok(pressed(press("k", { metaKey: true }), "palette"));
});

test("approval letters can be changed", () => {
  setBindings({ allow_once: "j" });
  assert.equal(approvalShortcut(press("j"), false), "allow_once");
  assert.equal(approvalShortcut(press("y"), false), null);
  assert.equal(approvalShortcut(press("j"), true), null);
  assert.equal(approvalHint("allow_once"), "J or ⌘↩");
  setBindings({});
});

test("what cannot be a shortcut is refused and says why", () => {
  assert.match(bindingProblem("palette", "Meta+C", {}), /kept for macOS/);
  assert.match(bindingProblem("palette", "Meta+1", {}), /kept for macOS/);
  assert.match(bindingProblem("palette", "K", {}), /Use ⌘/);
  assert.match(bindingProblem("palette", "Meta+,", {}), /already/);
  assert.match(bindingProblem("allow_once", "N", {}), /already/);
  assert.match(bindingProblem("allow_once", "Meta+K", {}), /one letter/);
  assert.equal(bindingProblem("palette", "Meta+P", {}), "");
  assert.equal(bindingProblem("allow_once", "J", {}), "");
  assert.equal(bindingProblem("palette", "Meta+,", { settings: "Meta+S" }), "");
});

test("a binding edited by hand into something unusable is ignored", () => {
  setBindings({ palette: "Meta+C", nonsense: "Meta+J" });
  assert.equal(bindingOf("palette"), "Meta+K");
  setBindings({});
});

test("an approval button says what its kind does, whatever the agent named it", () => {
  assert.equal(optionLabel("allow_always", "Deny"), "Allow always");
  assert.equal(optionLabel("reject_once", "Allow"), "Deny");
  assert.equal(optionLabel("other", "Something"), "Something");
});
