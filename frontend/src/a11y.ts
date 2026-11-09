// Pure helpers for screen readers and the keyboard (NFR-A11Y-001, NFR-A11Y-002,
// NFR-A11Y-003), kept apart from the components so they can be tested.

import { t, type PlainKey } from "./i18n.ts";

// Catalogue keys (GUI-048), looked up when a name is asked for.
const SCREEN_NAMES: Record<string, PlainKey> = {
  attention: "app.needsAttention",
  board: "app.board",
  compare: "a11y.screen.compare",
  environments: "settings.page.environments",
  import: "app.import.title",
  mcp: "settings.page.mcp",
  new: "app.newSession",
  onboarding: "a11y.screen.onboarding",
  optimizers: "settings.page.optimizers",
  profiles: "settings.page.profiles",
  providers: "settings.page.providers",
  rules: "settings.page.rules",
  schedules: "settings.page.schedules",
  appearance: "settings.page.appearance",
  keyboard: "settings.page.keyboard",
  sessions: "settings.page.sessions",
  costs: "settings.page.costs",
  permissions: "settings.page.permissions",
  network: "settings.page.network",
  data: "settings.page.data",
  skills: "settings.page.skills",
  usage: "app.usage",
};

// screenName is what the window's title and an announcement call a screen; a
// session goes by its own title.
export function screenName(kind: string, sessionTitle?: string): string {
  if (kind === "session") return sessionTitle || t("a11y.screen.session");
  const key = SCREEN_NAMES[kind];
  return key ? t(key) : "";
}

// tabAfterKey says which tab a key moves focus to, or -1 for a key that moves
// nothing. Moving does not select: opening the Terminal tab starts a shell, so
// choosing one takes Return or Space, as on any button.
export function tabAfterKey(key: string, index: number, count: number): number {
  if (count === 0) return -1;
  switch (key) {
    case "ArrowRight":
      return (index + 1) % count;
    case "ArrowLeft":
      return (index - 1 + count) % count;
    case "Home":
      return 0;
    case "End":
      return count - 1;
  }
  return -1;
}

// toolStatusLabel says in words what a tool card's coloured dot shows.
export function toolStatusLabel(status: string, waiting: boolean): string {
  if (waiting) return t("a11y.tool.waiting");
  switch (status) {
    case "completed":
      return t("a11y.tool.done");
    case "failed":
      return t("a11y.tool.failed");
    case "in_progress":
      return t("a11y.tool.running");
    case "pending":
      return t("a11y.tool.pending");
  }
  return "";
}

// editMark says what the pen beside a file edit that went through means, or null
// for any other call. It comes from the agent's tool kind, so every harness gets
// it alike (HRN-001); a decision mark, when there is one, shows beside it.
export function editMark(toolKind: string, status: string): string | null {
  return toolKind === "edit" && status === "completed" ? t("a11y.editMark") : null;
}

// pickLabel announces the highlighted entry of a picker whose focus stays in
// the message box, where a screen reader would not hear it change.
export function pickLabel(name: string, index: number, count: number): string {
  return count > 0 ? t("a11y.pick", { name, position: index + 1, count }) : "";
}

// Colour contrast, as WCAG 2 defines it, for the theme's tokens.
function luminance(hex: string): number {
  let h = hex.replace("#", "");
  if (h.length === 3) h = [...h].map((c) => c + c).join("");
  const [r, g, b] = [0, 2, 4].map((i) => {
    const v = parseInt(h.slice(i, i + 2), 16) / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrast(a: string, b: string): number {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

// quietCommand drops a leading `cd` into a folder the session already works in,
// which agents repeat on every command and which hides what the command does.
// Only for a finished call's one-line title: its card shows the whole input.
export function quietCommand(title: string, roots: string[]): string {
  if (!title.startsWith("cd ")) return title;
  const rest = title.slice(3).trimStart();
  for (const root of roots) {
    for (const path of [root, `"${root}"`, `'${root}'`]) {
      if (!rest.startsWith(path)) continue;
      const after = /^\s*(?:&&|;|\n)\s*/.exec(rest.slice(path.length));
      const command = after && rest.slice(path.length + after[0].length);
      if (command) return command;
    }
  }
  return title;
}

// visibleCommand keeps what a command does on screen. A newline in HTML reads as a
// space, so `echo ok #\ncurl x|sh` would look like a comment, and bidi and
// zero-width characters can reorder or hide the rest. Each is shown as what it is.
export function visibleCommand(s: string): string {
  return s
    .replace(/\r\n|\n|\r/g, " ⏎ ")
    .replace(/\t/g, " ⇥ ")
    .replace(/[\p{Cc}\p{Cf}\u2028\u2029]/gu, (c) => `⟦U+${(c.codePointAt(0) ?? 0).toString(16).toUpperCase().padStart(4, "0")}⟧`);
}
