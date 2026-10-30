// Slash commands in the composer (GUI-004). Mode commands are the app's own
// and work for every harness; the others come from the session's harness
// (available_commands_update) and are sent as typed, so the harness expands
// them. This module has no runtime imports so `node --test` can run it.

export interface SlashCommand {
  name: string;
  description: string;
  // Set for the app's mode commands.
  mode?: string;
  // Set for an MCP server's prompt (MCP-007), which runs through the
  // engine with a form for its arguments.
  prompt?: { server: string; name: string; arguments: PromptArgument[] };
}

export interface PromptArgument {
  name: string;
  title?: string;
  description?: string;
  required: boolean;
}

export const MODE_COMMANDS: SlashCommand[] = [
  { name: "ask", description: "Ask before edits and commands", mode: "ask" },
  { name: "accept-edits", description: "Allow edits inside the project without asking", mode: "accept_edits" },
  { name: "plan", description: "Read and plan only; no changes", mode: "plan" },
  { name: "bypass", description: "Run tool calls without asking", mode: "bypass" },
];

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
  const t = text.trim();
  if (!t.startsWith("/") || /\s/.test(t)) return null;
  return MODE_COMMANDS.find((c) => c.name === t.slice(1))?.mode ?? null;
}
