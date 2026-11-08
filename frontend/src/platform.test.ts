import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { ariaKeys, chord, detectOS, fileManager, forPlatform, keyLabel, primaryKey, primaryOnly, secretStore, inFileManager, setPlatform, showInFiles, systemName, terminalClipboard } from "./platform.ts";
import { approvalHint, approvalShortcut, bindingOf, bindingProblem, comboOf, defaultKey, pressed, setBindings } from "./shortcuts.ts";
import { isSettingsShortcut, textSizeAfter } from "./settings.ts";
import { setLanguage } from "./i18n.ts";

const k = (key: string, mods: Partial<{ metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean }> = {}) => ({
  key,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  ...mods,
});

const MAC = "MacIntel";
const WINDOWS = "Win32";
const LINUX = "Linux x86_64";

test("the system is read from the platform, and from the user agent when that is empty", () => {
  assert.equal(detectOS("MacIntel"), "mac");
  assert.equal(detectOS("darwin"), "mac");
  assert.equal(detectOS("Win32"), "windows");
  assert.equal(detectOS("Linux x86_64"), "linux");
  assert.equal(detectOS("", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Edg/120"), "windows");
  assert.equal(detectOS("", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15"), "linux");
  assert.equal(detectOS("", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15"), "mac");
  assert.equal(detectOS(""), "mac");
});

test("on a Mac the primary key is ⌘ and shortcuts are written with signs", () => {
  setPlatform(MAC);
  assert.equal(primaryKey(k("k", { metaKey: true })), true);
  assert.equal(primaryKey(k("k", { ctrlKey: true })), false);
  assert.equal(primaryOnly(k("1", { metaKey: true })), true);
  assert.equal(primaryOnly(k("1", { metaKey: true, ctrlKey: true })), false);
  assert.equal(keyLabel("Meta+K"), "⌘K");
  assert.equal(keyLabel("Meta+Shift+K"), "⌘⇧K");
  assert.equal(keyLabel("Ctrl+Shift+ArrowLeft"), "⌃⇧←");
  assert.equal(chord("Enter"), "⌘↩");
  assert.equal(chord("Backspace", true), "⌘⇧⌫");
  assert.equal(ariaKeys("Meta+K"), "Meta+K");
  assert.equal(ariaKeys("Ctrl+Shift+ArrowLeft"), "Control+Shift+ArrowLeft");
  assert.equal(fileManager(), "Finder");
  assert.equal(showInFiles(), "Show in Finder");
  assert.equal(secretStore(), "the macOS Keychain");
  assert.equal(systemName(), "macOS");
});

test("on Windows and Linux the primary key is Ctrl and shortcuts are spelled out", () => {
  for (const p of [WINDOWS, LINUX]) {
    setPlatform(p);
    assert.equal(primaryKey(k("k", { ctrlKey: true })), true, p);
    assert.equal(primaryKey(k("k", { metaKey: true })), false, p);
    assert.equal(primaryOnly(k("1", { ctrlKey: true })), true, p);
    assert.equal(primaryOnly(k("1", { ctrlKey: true, metaKey: true })), false, p);
    assert.equal(keyLabel("Meta+K"), "Ctrl+K", p);
    assert.equal(keyLabel("Meta+Shift+K"), "Ctrl+Shift+K", p);
    assert.equal(keyLabel("Meta+Shift++"), "Ctrl+Shift++", p);
    assert.equal(keyLabel("Alt+Shift+ArrowLeft"), "Alt+Shift+←", p);
    assert.equal(keyLabel("y"), "Y", p);
    assert.equal(chord("Enter"), "Ctrl+Enter", p);
    assert.equal(chord("Backspace", true), "Ctrl+Shift+Backspace", p);
    assert.equal(ariaKeys("Meta+K"), "Control+K", p);
  }
  setPlatform(WINDOWS);
  assert.equal(showInFiles(), "Show in File Explorer");
  assert.equal(secretStore(), "Windows Credential Manager");
  setPlatform(LINUX);
  assert.equal(showInFiles(), "Show in file manager");
  assert.equal(secretStore(), "the system keyring");
});

test("text written for a Mac reads right on each system", () => {
  const text = "Press ⌘K, then ⌘⇧↩. ⌘-click a row. ⌘, opens Settings. Data is in {{~/Library|%AppData%|~/.config}}; {{Finder|a file manager}}.";
  setPlatform(MAC);
  assert.equal(forPlatform(text), "Press ⌘K, then ⌘⇧↩. ⌘-click a row. ⌘, opens Settings. Data is in ~/Library; Finder.");
  setPlatform(WINDOWS);
  assert.equal(forPlatform(text), "Press Ctrl+K, then Ctrl+Shift+Enter. Ctrl-click a row. Ctrl+, opens Settings. Data is in %AppData%; a file manager.");
  setPlatform(LINUX);
  assert.equal(forPlatform(text), "Press Ctrl+K, then Ctrl+Shift+Enter. Ctrl-click a row. Ctrl+, opens Settings. Data is in ~/.config; a file manager.");
  assert.equal(forPlatform("⌥⌘1 and ⌘⇧⌫"), "Ctrl+Alt+1 and Ctrl+Shift+Backspace");
});

test("the app's shortcuts use Ctrl on Windows and Linux, and the same config means the same", () => {
  for (const p of [WINDOWS, LINUX]) {
    setPlatform(p);
    setBindings({});
    assert.equal(comboOf(k("k", { ctrlKey: true })), "Meta+K", p);
    assert.equal(comboOf(k("k", { metaKey: true })), "Super+K", p);
    assert.equal(pressed(k("k", { ctrlKey: true }), "palette"), true, p);
    assert.equal(pressed(k("k", { metaKey: true }), "palette"), false, p);
    assert.equal(isSettingsShortcut(k(",", { ctrlKey: true })), true, p);
    assert.equal(isSettingsShortcut(k(",", { metaKey: true })), false, p);
    assert.equal(textSizeAfter(k("=", { ctrlKey: true }), 14), 15, p);
    assert.equal(textSizeAfter(k("-", { ctrlKey: true }), 14), 13, p);
    assert.equal(textSizeAfter(k("=", { metaKey: true }), 14), null, p);
    // A binding written on a Mac with Control is Ctrl here too.
    setBindings({ palette: "Ctrl+J" });
    assert.equal(pressed(k("j", { ctrlKey: true }), "palette"), true, p);
    setBindings({});
  }
});

test("Ctrl+Shift and an arrow selects words outside a Mac, so the panes use Alt+Shift there", () => {
  setPlatform(MAC);
  setBindings({});
  assert.equal(defaultKey("paneLeft"), "Ctrl+Shift+ArrowLeft");
  assert.equal(pressed(k("ArrowLeft", { ctrlKey: true, shiftKey: true }), "paneLeft"), true);
  for (const p of [WINDOWS, LINUX]) {
    setPlatform(p);
    setBindings({});
    assert.equal(bindingOf("paneLeft"), "Alt+Shift+ArrowLeft", p);
    assert.equal(keyLabel(bindingOf("paneRight")), "Alt+Shift+→", p);
    assert.equal(pressed(k("ArrowLeft", { altKey: true, shiftKey: true }), "paneLeft"), true, p);
    assert.equal(pressed(k("ArrowLeft", { ctrlKey: true, shiftKey: true }), "paneLeft"), false, p);
    assert.equal(bindingProblem("paneUp", "Alt+Shift+ArrowDown", {}).includes("already"), true, p);
  }
});

test("outside a Mac, approvals answer to Ctrl, and the Windows key does nothing", () => {
  for (const p of [WINDOWS, LINUX]) {
    setPlatform(p);
    setBindings({});
    assert.equal(approvalShortcut(k("Enter", { ctrlKey: true }), true), "allow_once", p);
    assert.equal(approvalShortcut(k("Enter", { ctrlKey: true, shiftKey: true }), true), "allow_always", p);
    assert.equal(approvalShortcut(k("Backspace", { ctrlKey: true, shiftKey: true }), true), "reject", p);
    assert.equal(approvalShortcut(k("Enter", { metaKey: true }), true), null, p);
    assert.equal(approvalShortcut(k("Enter", { ctrlKey: true, metaKey: true }), true), null, p);
    assert.equal(approvalShortcut(k("y"), false), "allow_once", p);
    assert.equal(approvalHint("allow_once"), "Y or Ctrl+Enter", p);
  }
  setPlatform(MAC);
  assert.equal(approvalShortcut(k("Enter", { ctrlKey: true }), true), null);
  assert.equal(approvalHint("reject"), "N or ⌘⇧⌫");
});

test("outside a Mac a shortcut needs Ctrl, or Alt with a named key, and the system's keys are kept", () => {
  for (const p of [WINDOWS, LINUX]) {
    setPlatform(p);
    assert.match(bindingProblem("palette", "K", {}), /Use Ctrl/, p);
    assert.match(bindingProblem("palette", "Alt+K", {}), /Use Ctrl/, p);
    assert.match(bindingProblem("palette", "Meta+C", {}), new RegExp(`Ctrl\\+C is kept for ${p === WINDOWS ? "Windows" : "Linux"}`), p);
    assert.match(bindingProblem("palette", "Meta+Y", {}), /kept/, p);
    assert.match(bindingProblem("palette", "Ctrl+V", {}), /kept/, p);
    assert.equal(bindingProblem("palette", "Meta+P", {}), "", p);
    assert.equal(bindingProblem("palette", "Alt+ArrowUp", {}), "", p);
  }
  setPlatform(MAC);
  assert.equal(bindingProblem("palette", "Meta+Y", {}), "");
  assert.match(bindingProblem("palette", "K", {}), /Use ⌘/);
});

test("in a terminal, copy and paste follow the system without taking Ctrl+C from the shell", () => {
  setPlatform(MAC);
  assert.equal(terminalClipboard(k("c", { ctrlKey: true }), true), null);
  assert.equal(terminalClipboard(k("v", { metaKey: true }), false), null);
  setPlatform(WINDOWS);
  assert.equal(terminalClipboard(k("c", { ctrlKey: true }), false), null);
  assert.equal(terminalClipboard(k("c", { ctrlKey: true }), true), "copy");
  assert.equal(terminalClipboard(k("v", { ctrlKey: true }), false), "paste");
  assert.equal(terminalClipboard(k("C", { ctrlKey: true, shiftKey: true }), false), "copy");
  setPlatform(LINUX);
  assert.equal(terminalClipboard(k("c", { ctrlKey: true }), true), null);
  assert.equal(terminalClipboard(k("v", { ctrlKey: true }), false), null);
  assert.equal(terminalClipboard(k("C", { ctrlKey: true, shiftKey: true }), true), "copy");
  assert.equal(terminalClipboard(k("V", { ctrlKey: true, shiftKey: true }), false), "paste");
  assert.equal(terminalClipboard(k("d", { ctrlKey: true }), false), null);
});

test("every help page reads cleanly on each system", () => {
  const dir = new URL("./help/", import.meta.url);
  const pages = readdirSync(dir).filter((f) => f.endsWith(".md"));
  assert.ok(pages.length > 0);
  for (const p of [MAC, WINDOWS, LINUX]) {
    setPlatform(p);
    for (const f of pages) {
      const text = forPlatform(readFileSync(new URL(f, dir), "utf8"));
      assert.doesNotMatch(text, /\{\{|\}\}/, `${f} on ${p}`);
      if (p === MAC) continue;
      assert.doesNotMatch(text, /[⌘⌥⌃⇧]/, `${f} on ${p}`);
      assert.doesNotMatch(text, /\bthis Mac\b|\byour Mac\b|Finder|Keychain|~\/Library/, `${f} on ${p}`);
    }
  }
});

test("where secrets are kept and what shows a folder are said in the language in force (GUI-048)", () => {
  setLanguage("de");
  setPlatform("MacIntel");
  assert.equal(secretStore(), "dem macOS-Schlüsselbund");
  assert.equal(inFileManager(), "im Finder");
  setPlatform("Win32");
  assert.equal(secretStore(), "der Windows-Anmeldeinformationsverwaltung");
  setLanguage("nl");
  assert.equal(inFileManager(), "in Verkenner");
  setPlatform("Linux x86_64");
  assert.equal(secretStore(), "de sleutelbos van het systeem");
  setLanguage("en");
  setPlatform("MacIntel");
  assert.equal(inFileManager(), "in the Finder");
});
