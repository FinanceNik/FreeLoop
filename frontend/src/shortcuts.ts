import { isMac, keyLabel, chord, systemName } from "./platform.ts";

// Keyboard shortcuts for the approval card (GUI-006). Single letters work
// when no text field has focus; the ⌘ forms (Ctrl on Windows and Linux) work
// everywhere, the composer included, so an approval can be answered without
// leaving it.

export { ariaKeys, keyLabel } from "./platform.ts";

export type ApprovalKind = "allow_once" | "allow_always" | "reject";

export interface Keys {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}

// The approval card's letters and the app's own shortcuts can be changed
// (SET-006); these are what they start as. A global one is written like
// "Meta+K", modifiers in the order Meta, Ctrl, Alt, Shift. Meta is the primary
// key: ⌘ on a Mac, Ctrl on Windows and Linux, so a config.yaml means the same
// on each. Outside a Mac, Ctrl is the primary key and is written Meta, and the
// Windows or Super key is written Super.
export interface ShortcutInfo {
  id: string;
  label: string;
  key: string;
  // A letter works only outside text fields, and on the approval card.
  letter?: boolean;
}

function pane(id: string, label: string, arrow: string): ShortcutInfo {
  return {
    id,
    label,
    get key() {
      return `${isMac ? "Ctrl" : "Alt"}+Shift+${arrow}`;
    },
  };
}

export const SHORTCUTS: ShortcutInfo[] = [
  { id: "palette", label: "Open the command palette", key: "Meta+K" },
  { id: "settings", label: "Open Settings", key: "Meta+," },
  { id: "attention", label: "Open Needs attention", key: "Meta+0" },
  { id: "textLarger", label: "Larger text", key: "Meta+=" },
  { id: "textSmaller", label: "Smaller text", key: "Meta+-" },
  // ⌃⇧ and an arrow on a Mac. Elsewhere Ctrl+Shift and an arrow selects a word in a
  // text field, so Alt+Shift and an arrow.
  pane("paneLeft", "Move to the pane on the left", "ArrowLeft"),
  pane("paneRight", "Move to the pane on the right", "ArrowRight"),
  pane("paneUp", "Move to the pane above", "ArrowUp"),
  pane("paneDown", "Move to the pane below", "ArrowDown"),
  { id: "allow_once", label: "Approval: allow once", key: "y", letter: true },
  { id: "allow_always", label: "Approval: allow always", key: "a", letter: true },
  { id: "reject", label: "Approval: deny", key: "n", letter: true },
];

// Keys the system or the app's fixed shortcuts need: edit commands, closing and
// hiding, ⌘1 to ⌘9 for the sessions listed first. Ctrl+Y is redo outside a Mac.
const RESERVED_KEYS = ["C", "V", "X", "A", "Z", "Shift+Z", "Q", "W", "H", "M", "F", "R", "1", "2", "3", "4", "5", "6", "7", "8", "9"];
function reserved(combo: string): boolean {
  return [...RESERVED_KEYS, ...(isMac ? [] : ["Y"])].some((k) => combo === `Meta+${k}`);
}

// normal writes a binding the way a key press is written on this system: outside a
// Mac, Ctrl is the primary key, Meta.
function normal(binding: string): string {
  if (isMac || !binding.includes("Ctrl")) return binding;
  const parts = binding.endsWith("++") ? [...binding.slice(0, -2).split("+"), "+"] : binding.split("+");
  const key = parts.pop()!;
  const mods = new Set(parts.map((p) => (p === "Ctrl" ? "Meta" : p)));
  return [...["Meta", "Super", "Alt", "Shift"].filter((m) => mods.has(m)), key].join("+");
}

export type Bindings = Record<string, string | undefined>;

let bindings: Bindings = {};

// setBindings takes the user's changes from config.yaml.
// One edited by hand into something unusable, such as ⌘C, is ignored.
export function setBindings(b: Bindings | undefined | null) {
  const given = b ?? {};
  bindings = {};
  for (const [id, key] of Object.entries(given)) {
    const info = SHORTCUTS.find((s) => s.id === id);
    if (info && key && !bindingProblem(id, info.letter ? key.toUpperCase() : normal(key), given)) bindings[id] = info.letter ? key : normal(key);
  }
}

export function bindingOf(id: string): string {
  return bindings[id] || SHORTCUTS.find((s) => s.id === id)?.key || "";
}

// defaultKey is what a shortcut starts as, on this system.
export function defaultKey(id: string): string {
  return SHORTCUTS.find((s) => s.id === id)?.key ?? "";
}

// comboOf writes a key press in the form bindings use.
export function comboOf(k: Keys): string {
  if (["Meta", "Control", "Alt", "Shift", "OS"].includes(k.key)) return "";
  const parts = [];
  if (isMac) {
    if (k.metaKey) parts.push("Meta");
    if (k.ctrlKey) parts.push("Ctrl");
  } else {
    if (k.ctrlKey) parts.push("Meta");
    if (k.metaKey) parts.push("Super");
  }
  if (k.altKey) parts.push("Alt");
  if (k.shiftKey) parts.push("Shift");
  parts.push(k.key.length === 1 ? k.key.toUpperCase() : k.key);
  return parts.join("+");
}

// pressed reports a key press matching a global shortcut.
export function pressed(k: Keys, id: string): boolean {
  const want = normal(bindingOf(id));
  const combo = comboOf(k);
  if (combo === want) return true;
  // ⌘+ is ⌘= with Shift on some keyboards, and its own key on others.
  return want === "Meta+=" && (combo === "Meta+Shift++" || combo === "Meta++");
}

// bindingProblem says why a key cannot be used for a shortcut, or "".
export function bindingProblem(id: string, combo: string, current: Bindings): string {
  const info = SHORTCUTS.find((s) => s.id === id);
  if (!info || !combo) return "Press a key.";
  if (info.letter) {
    if (!/^[A-Z]$/.test(combo)) return "An approval shortcut is one letter, with no modifier.";
  } else {
    // Alt with a letter types a character on a Mac and opens a menu elsewhere, so
    // Alt goes only with a named key such as an arrow.
    const key = combo.endsWith("++") ? "+" : combo.slice(combo.lastIndexOf("+") + 1);
    if (!/(^|\+)(Meta|Ctrl)\+/.test(combo) && !(/(^|\+)Alt\+/.test(combo) && key.length > 1)) {
      return isMac ? "Use ⌘ or ⌃ with it, or ⌥ with an arrow, so it works while you type." : "Use Ctrl with it, or Alt with an arrow, so it works while you type.";
    }
    if (reserved(normal(combo))) return `${keyLabel(combo)} is kept for ${systemName()} or the app.`;
  }
  const wanted = info.letter ? combo.toLowerCase() : normal(combo);
  const taken = SHORTCUTS.find((s) => s.id !== id && (info.letter ? current[s.id] || s.key : normal(current[s.id] || s.key)) === wanted);
  return taken ? `${keyLabel(wanted)} is already "${taken.label}".` : "";
}

export function approvalHint(kind: ApprovalKind): string {
  return `${bindingOf(kind).toUpperCase()} or ${approvalChord(kind)}`;
}

// approvalChord is the fixed form of an approval shortcut: ⌘↩, ⌘⇧↩ and ⌘⇧⌫ on a
// Mac, Ctrl+Enter, Ctrl+Shift+Enter and Ctrl+Shift+Backspace elsewhere.
export function approvalChord(kind: ApprovalKind): string {
  return kind === "allow_once" ? chord("Enter") : kind === "allow_always" ? chord("Enter", true) : chord("Backspace", true);
}

// A card ignores shortcuts this long after it appears, so a key pressed
// for something else cannot approve a call nobody has read.
export const APPROVAL_GRACE_MS = 400;

export function approvalShortcut(k: Keys, inField: boolean): ApprovalKind | null {
  const primary = isMac ? k.metaKey : k.ctrlKey;
  const other = isMac ? k.ctrlKey : k.metaKey;
  if (other || k.altKey) return null;
  if (primary) {
    if (k.key === "Enter") return k.shiftKey ? "allow_always" : "allow_once";
    if (k.key === "Backspace" && k.shiftKey) return "reject";
    return null;
  }
  if (inField || k.shiftKey) return null;
  const letter = k.key.toLowerCase();
  for (const kind of ["allow_once", "allow_always", "reject"] as const) {
    if (bindingOf(kind) === letter) return kind;
  }
  return null;
}

// optionKind maps a permission option's kind onto a shortcut.
export function optionKind(kind: string): ApprovalKind | null {
  if (kind === "allow_once" || kind === "allow_always") return kind;
  return kind.startsWith("reject") ? "reject" : null;
}

// optionLabel names an approval button after what it does. An external agent
// chooses its options' names as well as their kinds, and only the kind counts: a
// button it named "Deny" with the kind allow_always allowed the call.
export function optionLabel(kind: string, name: string): string {
  switch (kind) {
    case "allow_once":
      return "Allow once";
    case "allow_always":
      return "Allow always";
    case "reject_once":
      return "Deny";
    case "reject_always":
      return "Always deny";
  }
  return name;
}

export function isTextField(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return el.isContentEditable || el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT";
}

// formLabel names a form's buttons (MCP-008, HRN-021): it answers, or it is skipped.
// The kind decides, as for any approval.
export function formLabel(kind: string): string {
  return kind.startsWith("allow") ? "Send" : "Skip";
}

