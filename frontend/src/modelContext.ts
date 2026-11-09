// What the model sees besides the conversation (NFR-PRIV-003): the records a built-in
// session writes of each kind of model call, made readable. Pure, so
// modelContext.test.ts covers it. Everything here is shown as plain text: the project's
// instructions come from the repository and must never reach the page as markup.
import { locale } from "./i18n.ts";

export interface ToolDef {
  name: string;
  description: string;
  inputSchema?: unknown;
}

export interface ModelContext {
  purpose: string;
  providerId?: string;
  model: string;
  effort?: string;
  system: string;
  tools: ToolDef[];
  sent?: string;
}

export interface PromptSection {
  title: string;
  text: string;
  chars: number;
  // From the repository: the project's AGENTS.md or CLAUDE.md, nested or not.
  fromRepository: boolean;
}

export const PURPOSE_LABEL: Record<string, string> = {
  turn: "Each turn",
  compaction: "Compacting the conversation",
  commit_message: "Drafting a commit message",
  pull_request: "Drafting a pull request",
  handoff: "Summarising for a handoff",
};

// The order the kinds of call are shown in.
export const PURPOSE_ORDER = ["turn", "compaction", "commit_message", "pull_request", "handoff"];

// sections splits a system prompt at its top-level headings ("# Safety"); what comes
// before the first heading is the introduction.
export function sections(system: string): PromptSection[] {
  const out: PromptSection[] = [];
  let title = "Introduction";
  let lines: string[] = [];
  const flush = () => {
    const text = lines.join("\n").trim();
    if (text) out.push({ title, text, chars: text.length, fromRepository: /^Project instructions|^Instructions for /.test(title) });
  };
  for (const line of system.split("\n")) {
    const heading = /^# (.+)$/.exec(line);
    if (heading) {
      flush();
      title = heading[1].trim();
      lines = [];
    } else {
      lines.push(line);
    }
  }
  flush();
  return out;
}

// tokens is a rough count for a size: about four characters a token.
export function tokens(chars: number): number {
  return Math.round(chars / 4);
}

// sizeText says how big something is, in words.
export function sizeText(chars: number): string {
  const t = tokens(chars);
  return `${chars.toLocaleString(locale())} characters, about ${t.toLocaleString(locale())} tokens`;
}

// latestByPurpose keeps the newest record of each kind of call, in display order, with
// how many records each had.
export function latestByPurpose(records: readonly ModelContext[]): { context: ModelContext; changes: number }[] {
  const latest = new Map<string, { context: ModelContext; changes: number }>();
  for (const r of records) latest.set(r.purpose, { context: r, changes: (latest.get(r.purpose)?.changes ?? 0) + 1 });
  return [...latest.values()].sort((a, b) => order(a.context.purpose) - order(b.context.purpose));
}

function order(purpose: string): number {
  const i = PURPOSE_ORDER.indexOf(purpose);
  return i < 0 ? PURPOSE_ORDER.length : i;
}
