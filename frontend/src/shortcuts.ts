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

export const APPROVAL_HINTS: Record<ApprovalKind, string> = {
  allow_once: "Y or ⌘↩",
  allow_always: "A or ⌘⇧↩",
  reject: "N or ⌘⇧⌫",
};

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
  switch (k.key.toLowerCase()) {
    case "y":
      return "allow_once";
    case "a":
      return "allow_always";
    case "n":
      return "reject";
  }
  return null;
}

// optionKind maps a permission option's kind onto a shortcut.
export function optionKind(kind: string): ApprovalKind | null {
  if (kind === "allow_once" || kind === "allow_always") return kind;
  return kind.startsWith("reject") ? "reject" : null;
}

export function isTextField(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return el.isContentEditable || el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT";
}
