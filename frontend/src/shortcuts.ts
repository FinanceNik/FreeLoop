// Keyboard shortcuts for the approval card (GUI-006). Single letters work
// when no text field has focus; the ⌘ forms work everywhere, the composer
// included, so an approval can be answered without leaving it.

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
// "Meta+K", modifiers in the order Meta, Ctrl, Alt, Shift.
export interface ShortcutInfo {
  id: string;
  label: string;
  key: string;
  // A letter works only outside text fields, and on the approval card.
  letter?: boolean;
}

export const SHORTCUTS: ShortcutInfo[] = [
  { id: "palette", label: "Open the command palette", key: "Meta+K" },
  { id: "settings", label: "Open Settings", key: "Meta+," },
  { id: "attention", label: "Open Needs attention", key: "Meta+0" },
  { id: "textLarger", label: "Larger text", key: "Meta+=" },
  { id: "textSmaller", label: "Smaller text", key: "Meta+-" },
  { id: "allow_once", label: "Approval: allow once", key: "y", letter: true },
  { id: "allow_always", label: "Approval: allow always", key: "a", letter: true },
  { id: "reject", label: "Approval: deny", key: "n", letter: true },
];

// Keys the system or the app's fixed shortcuts need: edit commands, closing and
// hiding, ⌘1 to ⌘9 for the sessions listed first.
const RESERVED = new Set(["C", "V", "X", "A", "Z", "Shift+Z", "Q", "W", "H", "M", "F", "R", "1", "2", "3", "4", "5", "6", "7", "8", "9"].map((k) => `Meta+${k}`));

export type Bindings = Record<string, string | undefined>;

let bindings: Bindings = {};

// setBindings takes the user's changes from config.yaml.
// One edited by hand into something unusable, such as ⌘C, is ignored.
export function setBindings(b: Bindings | undefined | null) {
  const given = b ?? {};
  bindings = {};
  for (const [id, key] of Object.entries(given)) {
    const info = SHORTCUTS.find((s) => s.id === id);
    if (info && key && !bindingProblem(id, info.letter ? key.toUpperCase() : key, given)) bindings[id] = key;
  }
}

export function bindingOf(id: string): string {
  return bindings[id] || SHORTCUTS.find((s) => s.id === id)?.key || "";
}

// comboOf writes a key press in the form bindings use.
export function comboOf(k: Keys): string {
  if (["Meta", "Control", "Alt", "Shift"].includes(k.key)) return "";
  const parts = [];
  if (k.metaKey) parts.push("Meta");
  if (k.ctrlKey) parts.push("Ctrl");
  if (k.altKey) parts.push("Alt");
  if (k.shiftKey) parts.push("Shift");
  parts.push(k.key.length === 1 ? k.key.toUpperCase() : k.key);
  return parts.join("+");
}

// pressed reports a key press matching a global shortcut.
export function pressed(k: Keys, id: string): boolean {
  const want = bindingOf(id);
  const combo = comboOf(k);
  if (combo === want) return true;
  // ⌘+ is ⌘= with Shift on some keyboards, and its own key on others.
  return want === "Meta+=" && (combo === "Meta+Shift++" || combo === "Meta++");
}

// keyLabel shows a binding the way macOS writes one.
export function keyLabel(binding: string): string {
  const signs: Record<string, string> = { Meta: "⌘", Ctrl: "⌃", Alt: "⌥", Shift: "⇧", Enter: "↩", Backspace: "⌫", Escape: "⎋", ArrowUp: "↑", ArrowDown: "↓", ArrowLeft: "←", ArrowRight: "→" };
  if (!binding.includes("+") || binding === "+") return binding.toUpperCase();
  // The key can itself be "+".
  const parts = binding.endsWith("++") ? [...binding.slice(0, -2).split("+"), "+"] : binding.split("+");
  return parts.map((p) => signs[p] ?? p).join("");
}

// ariaKeys writes a binding for aria-keyshortcuts.
export function ariaKeys(binding: string): string {
  return binding.replace("Ctrl", "Control");
}

// bindingProblem says why a key cannot be used for a shortcut, or "".
export function bindingProblem(id: string, combo: string, current: Bindings): string {
  const info = SHORTCUTS.find((s) => s.id === id);
  if (!info || !combo) return "Press a key.";
  if (info.letter) {
    if (!/^[A-Z]$/.test(combo)) return "An approval shortcut is one letter, with no modifier.";
  } else {
    if (!/(^|\+)(Meta|Ctrl)\+/.test(combo)) return "Use ⌘ or ⌃ with it, so it works while you type.";
    if (RESERVED.has(combo)) return `${keyLabel(combo)} is kept for macOS or the app.`;
  }
  const wanted = info.letter ? combo.toLowerCase() : combo;
  const taken = SHORTCUTS.find((s) => s.id !== id && (current[s.id] || s.key) === wanted);
  return taken ? `${keyLabel(wanted)} is already "${taken.label}".` : "";
}

export function approvalHint(kind: ApprovalKind): string {
  const chord = { allow_once: "⌘↩", allow_always: "⌘⇧↩", reject: "⌘⇧⌫" }[kind];
  return `${bindingOf(kind).toUpperCase()} or ${chord}`;
}

// A card ignores shortcuts this long after it appears, so a key pressed
// for something else cannot approve a call nobody has read.
export const APPROVAL_GRACE_MS = 400;

export function approvalShortcut(k: Keys, inField: boolean): ApprovalKind | null {
  if (k.ctrlKey || k.altKey) return null;
  if (k.metaKey) {
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
