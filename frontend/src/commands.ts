// Slash commands in the composer (GUI-004). Mode commands are the app's own
// and work for every harness; the others come from the session's harness
// (available_commands_update) and are sent as typed, so the harness expands
// them. Its only runtime import is the translation module, so `node --test` can run it.

import { t, type PlainKey } from "./i18n.ts";

export interface SlashCommand {
  name: string;
  description: string;
  // Set for the app's mode commands.
  mode?: string;
  // Set for an MCP server's prompt (MCP-007), which runs through the
  // engine with a form for its arguments.
  prompt?: { server: string; name: string; arguments: PromptArgument[] };
  // Set for a stage of the session's workflow (WFL-002), which the engine runs.
  stage?: boolean;
}

export interface PromptArgument {
  name: string;
  title?: string;
  description?: string;
  required: boolean;
}

// The permission modes (SEC-001), the app's own and the same for every harness, so
// named and explained in the language in force (GUI-048). A mode an agent advertises
// is shown as it names it.
export const MODES = [
  { id: "ask", name: "mode.ask", hint: "mode.ask.hint", command: "ask", description: "mode.ask.command" },
  { id: "accept_edits", name: "mode.acceptEdits", hint: "mode.acceptEdits.hint", command: "accept-edits", description: "mode.acceptEdits.command" },
  { id: "plan", name: "mode.plan", hint: "mode.plan.hint", command: "plan", description: "mode.plan.command" },
  { id: "auto", name: "mode.auto", hint: "mode.auto.hint", command: "auto", description: "mode.auto.command" },
  { id: "bypass", name: "mode.bypass", hint: "mode.bypass.hint", command: "bypass", description: "mode.bypass.command" },
] as const satisfies readonly { id: string; name: PlainKey; hint: PlainKey; command: string; description: PlainKey }[];

// A session never starts in these: the user chooses them in the session (SEC-001).
export const STARTING_MODES = MODES.filter((m) => m.id !== "bypass" && m.id !== "auto");

// modeName is a mode's name in the language in force, or the id of one the app does
// not know.
export function modeName(id: string): string {
  const m = MODES.find((m) => m.id === id);
  return m ? t(m.name) : id;
}

// modeHint says what a mode does, for one the app knows.
export function modeHint(id: string): string | undefined {
  const m = MODES.find((m) => m.id === id);
  return m && t(m.hint);
}

// modeCommands are the slash commands that choose a mode, described in the language
// in force; the command is typed, so it stays as it is.
export function modeCommands(): SlashCommand[] {
  return MODES.map((m) => ({ name: m.command, description: t(m.description), mode: m.id }));
}

// slashQuery returns what follows the / while the first word of the text is
// being typed, or null.
export function slashQuery(text: string, caret: number): string | null {
  if (!text.startsWith("/")) return null;
  const word = text.slice(1, caret);
  if (/\s/.test(word) || caret < 1) return null;
  return word;
}

// matchCommands lists the commands whose name starts with query first, then
// those that contain it.
export function matchCommands(query: string, list: readonly SlashCommand[]): SlashCommand[] {
  const q = query.toLowerCase();
  const starts = list.filter((c) => c.name.startsWith(q));
  const contains = list.filter((c) => !c.name.startsWith(q) && c.name.includes(q));
  return [...starts, ...contains];
}

// modeCommand returns the mode a message selects when it is exactly a mode
// command, such as "/plan".
export function modeCommand(text: string): string | null {
  const typed = text.trim();
  if (!typed.startsWith("/") || /\s/.test(typed)) return null;
  return MODES.find((m) => m.command === typed.slice(1))?.id ?? null;
}
