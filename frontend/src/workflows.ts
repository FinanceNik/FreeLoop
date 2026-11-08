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
  skill?: string;
  // It needs what follows its command, such as a brief (WFL-008).
  needsInput?: boolean;
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

// ---- Guidance (WFL-008): where a session is in its workflow, and what is next ----

// What the stage strip needs of the conversation: the stages that started, in
// order, and the turns that ended, by position.
export interface ItemLike {
  kind: string;
  command?: string;
  toolKind?: string;
  status?: string;
  content?: { path?: string }[];
  input?: unknown;
}

export interface RunLike {
  state: string;
  steps: { state: string }[] | null;
}

export type StageState = "done" | "running" | "next" | "todo";

export interface StageStep {
  stage: Stage;
  state: StageState;
  // A word on how far it got, such as an AFK run's "2 of 8 done".
  detail?: string;
}

// guided are the stages on the workflow's path: those that send a prompt, and the
// AFK run. A review or a cleanup is done when wanted, so it stays a slash command.
export function guided(w: WorkflowLike | undefined): Stage[] {
  return (w?.stages ?? []).filter((s) => !s.kind || s.kind === "prompt" || s.kind === "afk");
}

// stageProgress says, for each stage on the path, whether it ran in this session,
// runs now, comes next or later. The AFK stage follows the project's run.
export function stageProgress(w: WorkflowLike | undefined, items: readonly ItemLike[], running: boolean, run?: RunLike | null): StageStep[] {
  const stages = guided(w);
  const ran = new Set<string>();
  let last = -1;
  let open = false; // the last stage's turn has not ended
  for (const it of items) {
    if (it.kind === "stage" && it.command) {
      ran.add(it.command);
      last = Math.max(last, stages.findIndex((s) => s.command === it.command));
      open = true;
    } else if (it.kind === "turn_end") {
      open = false;
    }
  }
  const steps: StageStep[] = stages.map((stage) => ({ stage, state: ran.has(stage.command) ? "done" : "todo" }));
  const current = items.filter((it) => it.kind === "stage").pop()?.command;
  if (running && open && current) {
    const i = stages.findIndex((s) => s.command === current);
    if (i >= 0) steps[i].state = "running";
  }
  const afk = steps.find((s) => s.stage.kind === "afk");
  if (afk && run) {
    const all = run.steps ?? [];
    const done = all.filter((s) => s.state === "qa" || s.state === "qa_done").length;
    const going = run.state === "running" || run.state === "paused";
    afk.state = going ? "running" : "done";
    afk.detail = `${done} of ${all.length} done`;
    last = Math.max(last, stages.indexOf(afk.stage));
  }
  if (!steps.some((s) => s.state === "running")) {
    const next = steps.findIndex((s, i) => i > last && s.state === "todo");
    if (next >= 0) steps[next].state = "next";
  }
  return steps;
}

// stageOutputs are the files the last stage's turns wrote, for the handoff line:
// the paths of the file edits that completed since the stage started.
export function stageOutputs(items: readonly ItemLike[]): string[] {
  const from = items.map((it) => it.kind).lastIndexOf("stage");
  if (from < 0) return [];
  const out: string[] = [];
  for (const it of items.slice(from + 1)) {
    if (it.kind !== "tool" || it.toolKind !== "edit" || it.status !== "completed") continue;
    const input = (it.input ?? {}) as Record<string, unknown>;
    const paths = (it.content ?? []).map((c) => c.path).filter((p): p is string => !!p);
    for (const p of paths.length ? paths : [input.path, input.file_path].filter((p): p is string => typeof p === "string")) {
      if (!out.includes(p)) out.push(p);
    }
  }
  return out;
}

// withoutStageSkills drops an agent's own command for a skill one of the stages
// uses ("freeloop:grill-the-brief" beside /grill), so the menu offers the stage.
export function withoutStageSkills<T extends { name: string }>(cmds: readonly T[], w: WorkflowLike | undefined): T[] {
  const skills = (w?.stages ?? []).map((s) => s.skill).filter((s): s is string => !!s);
  return cmds.filter((c) => !skills.some((s) => c.name === s || c.name.endsWith(`:${s}`)));
}
