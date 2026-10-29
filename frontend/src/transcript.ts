// A session's transcript in the UI is a pure function of its events. Replay
// from disk and the live stream both go through apply, so they cannot drift.
// This module has no runtime imports so `node --test` can run it directly.

export interface TranscriptEvent {
  v: number;
  seq: number;
  ts: string;
  sid: string;
  type: string;
  data: unknown;
}

export interface Transcript {
  events: TranscriptEvent[];
  lastSeq: number;
}

export const emptyTranscript: Transcript = { events: [], lastSeq: 0 };

export interface ApplyResult {
  transcript: Transcript;
  // When incoming events skip ahead (the bridge dropped some), the caller
  // fetches everything after this seq and applies it.
  missingAfter: number | null;
}

// apply appends the events that extend t contiguously, ignores ones already
// seen, and stops at the first gap.
export function apply(t: Transcript, incoming: readonly TranscriptEvent[]): ApplyResult {
  const sorted = [...incoming].sort((a, b) => a.seq - b.seq);
  const added: TranscriptEvent[] = [];
  let lastSeq = t.lastSeq;
  for (const ev of sorted) {
    if (ev.seq <= lastSeq) continue;
    if (ev.seq !== lastSeq + 1) {
      return { transcript: extend(t, added, lastSeq), missingAfter: lastSeq };
    }
    added.push(ev);
    lastSeq = ev.seq;
  }
  return { transcript: extend(t, added, lastSeq), missingAfter: null };
}

function extend(t: Transcript, added: TranscriptEvent[], lastSeq: number): Transcript {
  return added.length === 0 ? t : { events: [...t.events, ...added], lastSeq };
}

// summary renders an event's payload as one line of text for the M0 view.
export function summary(ev: TranscriptEvent): string {
  const d = ev.data as Record<string, unknown> | null;
  if (d && typeof d === "object") {
    const content = d.content as { type?: string; text?: string } | undefined;
    if (content?.type === "text" && typeof content.text === "string") return content.text;
    if (typeof d.status === "string") return d.status;
    if (typeof d.stopReason === "string") return d.stopReason;
    if (typeof d.message === "string") return d.message;
  }
  return JSON.stringify(ev.data);
}

// ---- Conversation view: events folded into items to render ----

export interface PermissionOption {
  optionId: string;
  name: string;
  kind: string;
}

export interface PermissionState {
  requestId: string;
  options: PermissionOption[];
  command?: string;
  paths?: string[];
  decided?: {
    outcome: string;
    actor: string;
    optionId?: string;
    reason?: string;
    persisted?: { tool: string; command?: string; path?: string; scope: string };
  };
}

export interface ToolContent {
  type: string;
  content?: { type: string; text?: string };
  path?: string;
  oldText?: string;
  newText?: string;
  exitCode?: number;
  output?: string;
}

export type Item =
  | { kind: "user"; key: string; text: string; ts: string }
  | { kind: "agent"; key: string; text: string }
  | { kind: "thought"; key: string; text: string }
  | {
      kind: "tool";
      key: string;
      toolCallId: string;
      tool: string;
      title: string;
      toolKind: string;
      status: string;
      input: unknown;
      content: ToolContent[];
      permission?: PermissionState;
    }
  | { kind: "turn_end"; key: string; stopReason: string; ts: string }
  | { kind: "error"; key: string; message: string }
  | { kind: "mode"; key: string; mode: string };

export interface Usage {
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
}

export interface GitState {
  repo: boolean;
  branch: string;
  head: string;
  dirty: number;
  baseCommit: string;
}

export interface SessionView {
  items: Item[];
  usage: Usage;
  git: GitState | null;
  mode: string | null;
  status: string | null;
  running: boolean;
}

type Data = Record<string, any>;

// buildView folds a session's events into what the conversation shows.
export function buildView(events: readonly TranscriptEvent[]): SessionView {
  const items: Item[] = [];
  const tools = new Map<string, Extract<Item, { kind: "tool" }>>();
  const usage: Usage = { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0 };
  let mode: string | null = null;
  let status: string | null = null;
  let running = false;
  let git: GitState | null = null;

  const last = () => items[items.length - 1];
  for (const ev of events) {
    const d = (ev.data ?? {}) as Data;
    const key = String(ev.seq);
    switch (ev.type) {
      case "user_message_chunk":
        items.push({ kind: "user", key, text: d.content?.text ?? "", ts: ev.ts });
        break;
      case "agent_message_chunk":
      case "agent_thought_chunk": {
        const kind = ev.type === "agent_message_chunk" ? "agent" : "thought";
        const prev = last();
        const text = d.content?.text ?? "";
        if (prev && prev.kind === kind) {
          items[items.length - 1] = { ...prev, text: prev.text + text };
        } else {
          items.push({ kind, key, text });
        }
        break;
      }
      case "tool_call": {
        const item = {
          kind: "tool" as const,
          key,
          toolCallId: d.toolCallId,
          tool: d.tool ?? "",
          title: d.title ?? d.tool ?? "Tool call",
          toolKind: d.kind ?? "other",
          status: d.status ?? "pending",
          input: d.rawInput,
          content: [] as ToolContent[],
        };
        tools.set(d.toolCallId, item);
        items.push(item);
        break;
      }
      case "tool_call_update": {
        const item = tools.get(d.toolCallId);
        if (!item) break;
        const next = { ...item, status: d.status ?? item.status, content: d.content ?? item.content };
        replaceTool(items, tools, next);
        break;
      }
      case "permission_requested": {
        const item = tools.get(d.toolCallId);
        const permission: PermissionState = {
          requestId: d.requestId,
          options: d.options ?? [],
          command: d.command,
          paths: d.paths ?? undefined,
        };
        if (item) replaceTool(items, tools, { ...item, permission });
        break;
      }
      case "permission_decided": {
        const item = tools.get(d.toolCallId);
        if (!item) break;
        const decided = { outcome: d.outcome, actor: d.actor, optionId: d.optionId, reason: d.reason, persisted: d.persisted };
        const permission: PermissionState = item.permission
          ? { ...item.permission, decided }
          : { requestId: d.requestId, options: [], decided };
        replaceTool(items, tools, { ...item, permission });
        break;
      }
      case "turn_started":
        running = true;
        break;
      case "turn_ended":
        running = false;
        items.push({ kind: "turn_end", key, stopReason: d.stopReason ?? "", ts: ev.ts });
        break;
      case "usage_update":
        usage.inputTokens += d.inputTokens ?? 0;
        usage.outputTokens += d.outputTokens ?? 0;
        usage.cacheReadTokens += d.cacheReadTokens ?? 0;
        usage.cacheWriteTokens += d.cacheWriteTokens ?? 0;
        break;
      case "current_mode_update":
        mode = d.currentModeId ?? mode;
        items.push({ kind: "mode", key, mode: d.currentModeId });
        break;
      case "session_status":
        status = d.status ?? status;
        break;
      case "git_state":
        git = { repo: !!d.repo, branch: d.branch ?? "", head: d.head ?? "", dirty: d.dirty ?? 0, baseCommit: d.baseCommit ?? "" };
        break;
      case "error":
        items.push({ kind: "error", key, message: d.message ?? "Unknown error" });
        break;
    }
  }
  return { items, usage, git, mode, status, running };
}

function replaceTool(
  items: Item[],
  tools: Map<string, Extract<Item, { kind: "tool" }>>,
  next: Extract<Item, { kind: "tool" }>,
) {
  const i = items.indexOf(tools.get(next.toolCallId)!);
  if (i >= 0) items[i] = next;
  tools.set(next.toolCallId, next);
}

// pendingRequest returns the permission the user still has to answer.
export function awaitingDecision(item: Item): boolean {
  return item.kind === "tool" && !!item.permission && !item.permission.decided && item.permission.options.length > 0;
}

export interface DiffLine {
  sign: " " | "-" | "+";
  text: string;
}

// lineDiff is a small diff for edit previews: common leading and trailing
// lines are trimmed to `context` lines, the rest shows as removed then added.
export function lineDiff(oldText: string, newText: string, context = 2): DiffLine[] {
  const lines = (t: string) => (t === "" ? [] : t.replace(/\n$/, "").split("\n"));
  const a = lines(oldText);
  const b = lines(newText);
  let start = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) start++;
  let endA = a.length;
  let endB = b.length;
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--;
    endB--;
  }
  const out: DiffLine[] = [];
  for (let i = Math.max(0, start - context); i < start; i++) out.push({ sign: " ", text: a[i] });
  for (let i = start; i < endA; i++) out.push({ sign: "-", text: a[i] });
  for (let i = start; i < endB; i++) out.push({ sign: "+", text: b[i] });
  for (let i = endA; i < Math.min(a.length, endA + context); i++) out.push({ sign: " ", text: a[i] });
  return out;
}
