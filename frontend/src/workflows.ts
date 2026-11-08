// A session's workflow stages as slash commands (WFL-002). A stage's command,
// with whatever follows it, goes to the engine, which sends the stage's prompt,
// starts a run or a review. This module has no runtime imports so `node --test`
// can run it.

import type { SlashCommand } from "./commands.ts";

export interface Stage {
  command: string;
  title: string;
  description?: string;
  kind?: string;
}

export interface WorkflowLike {
  id: string;
  name: string;
  stages: Stage[] | null;
  reason?: string;
}

// stageCommands lists a workflow's stages as slash commands.
export function stageCommands(w: WorkflowLike | undefined): SlashCommand[] {
  if (!w) return [];
  return (w.stages ?? []).map((s) => ({ name: s.command, description: `${w.name}: ${s.description || s.title}`, stage: true }));
}

// stageCall reads a message that invokes a stage: "/grill add dark mode" is the
// grill stage with "add dark mode". Only the workflow's own commands count.
export function stageCall(text: string, w: WorkflowLike | undefined): { command: string; input: string } | null {
  const m = /^\/([a-z][a-z0-9-]*)(?:\s+([\s\S]*))?$/.exec(text.trim());
  if (!m || !w) return null;
  if (!(w.stages ?? []).some((s) => s.command === m[1])) return null;
  return { command: m[1], input: (m[2] ?? "").trim() };
}

// What each kind of stage does, for the palette and Settings.
export function stageAction(kind: string | undefined): string {
  switch (kind) {
    case "afk":
      return "Starts an AFK run";
    case "review":
      return "Starts a review";
    case "cleanup":
      return "Archives what is finished";
    default:
      return "Sends its prompt here";
  }
}
