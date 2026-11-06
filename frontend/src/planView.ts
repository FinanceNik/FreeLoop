import { awaitingDecision, type Item, type PermissionOption } from "./transcript.ts";

// A plan an agent asks the user to approve (GUI-043): the built-in agent's present_plan
// and an external agent's switch_mode request both carry it as `plan` in their input,
// so neither is special-cased.

type ToolItem = Extract<Item, { kind: "tool" }>;

export function planText(item: ToolItem): string | null {
  if (item.toolKind !== "switch_mode" && item.tool !== "present_plan") return null;
  const plan = (item.input as { plan?: unknown } | null)?.plan;
  return typeof plan === "string" && plan.trim() !== "" ? plan : null;
}

export type PlanState = "waiting" | "approved" | "sent back" | "stopped" | "shown";

export interface PlanVersion {
  toolCallId: string;
  text: string;
  state: PlanState;
  item: ToolItem;
}

function stateOf(item: ToolItem): PlanState {
  if (awaitingDecision(item)) return "waiting";
  const d = item.permission?.decided;
  if (d?.outcome === "allowed") return "approved";
  if (d?.outcome === "denied") return "sent back";
  if (d) return "stopped";
  return "shown";
}

// planVersions lists a session's plans, oldest first.
export function planVersions(items: readonly Item[]): PlanVersion[] {
  const out: PlanVersion[] = [];
  for (const item of items) {
    if (item.kind !== "tool") continue;
    const text = planText(item);
    if (text !== null) out.push({ toolCallId: item.toolCallId, text, state: stateOf(item), item });
  }
  return out;
}

// planBlocks splits a plan where a reader comments on it: at blank lines, keeping a
// fenced code block whole.
export function planBlocks(text: string): string[] {
  const blocks: string[] = [];
  let current: string[] = [];
  let fence = false;
  for (const line of text.split("\n")) {
    if (/^\s*(```|~~~)/.test(line)) fence = !fence;
    if (!fence && line.trim() === "") {
      if (current.length) blocks.push(current.join("\n"));
      current = [];
      continue;
    }
    current.push(line);
  }
  if (current.length) blocks.push(current.join("\n"));
  return blocks;
}

const QUOTE_MAX = 100;

// quoteOf is the line a comment is about, short enough to quote back.
export function quoteOf(block: string): string {
  const first = block.split("\n").find((l) => l.trim() !== "") ?? "";
  const plain = first.replace(/^\s*(#{1,6}\s+|[-*+]\s+|\d+[.)]\s+|>\s*)/, "").trim();
  return plain.length > QUOTE_MAX ? `${plain.slice(0, QUOTE_MAX - 1)}…` : plain;
}

export interface PlanNote {
  block: number;
  quote: string;
  note: string;
}

// planFeedback puts the user's comments together for the agent: each under the line it
// is about, then what they said about the plan as a whole.
export function planFeedback(notes: readonly PlanNote[], general: string): string {
  const parts = [...notes]
    .filter((n) => n.note.trim() !== "")
    .sort((a, b) => a.block - b.block)
    .map((n) => `> ${n.quote}\n\n${n.note.trim()}`);
  if (general.trim()) parts.push(general.trim());
  return parts.join("\n\n");
}

// planChoices splits a plan's options by what they do. The kind decides, never the
// agent's wording, which only says what follows: an option named "Keep planning" with
// an allow kind approves.
export function planChoices(options: readonly PermissionOption[]): { approve: PermissionOption[]; back: PermissionOption[] } {
  return {
    approve: options.filter((o) => o.kind === "allow_once" || o.kind === "allow_always"),
    back: options.filter((o) => o.kind.startsWith("reject")),
  };
}

// approveLabel names an approving option for the menu: the agent's words for what
// follows, without its own "Yes,".
export function approveLabel(o: PermissionOption): string {
  const words = o.name.replace(/^\s*(yes|approve)\s*[,:-]?\s*(and\s+)?/i, "").trim();
  return words ? words.charAt(0).toUpperCase() + words.slice(1) : "Approve";
}
