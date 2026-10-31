import type { ModelCall } from "./cost.ts";

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
  // An MCP server's request for input (MCP-008), asked as a form.
  form?: { server: string; message: string; schema?: unknown; url?: string };
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
  content?: { type: string; text?: string; uri?: string; name?: string };
  path?: string;
  oldText?: string;
  newText?: string;
  exitCode?: number;
  output?: string;
}

// Every item carries rev, the seq of the last event that changed it, so the
// view can skip re-rendering items that did not change.
// Items after a rewind's checkpoint carry rewound (SES-014).
export type Item = ItemKinds & { rewound?: boolean };

type ItemKinds =
  | {
      kind: "user";
      key: string;
      rev: number;
      text: string;
      ts: string;
      files: string[];
      checkpointSeq?: number;
      // The context was replaced after the checkpoint (a compaction or an
      // earlier rewind): rewinding restores the files only.
      filesOnly?: boolean;
    }
  | { kind: "agent"; key: string; rev: number; text: string }
  | { kind: "thought"; key: string; rev: number; text: string }
  | {
      kind: "tool";
      key: string;
      rev: number;
      toolCallId: string;
      tool: string;
      title: string;
      toolKind: string;
      status: string;
      input: unknown;
      content: ToolContent[];
      permission?: PermissionState;
    }
  | { kind: "turn_end"; key: string; rev: number; stopReason: string; ts: string; calls: ModelCall[] }
  | { kind: "error"; key: string; rev: number; message: string }
  | { kind: "mode"; key: string; rev: number; mode: string }
  | { kind: "model"; key: string; rev: number; model: string; providerId: string }
  | { kind: "notice"; key: string; rev: number; message: string; warn: boolean }
  | { kind: "compaction"; key: string; rev: number; reason: string; before: number; after: number; summary: string }
  | { kind: "handoff"; key: string; rev: number; fromTitle: string; summary: string; generated: boolean }
  | { kind: "pull_request"; key: string; rev: number; url: string; title: string; base: string; head: string; draft: boolean };

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
  // Every model call, for cost estimates (PRV-008).
  calls: ModelCall[];
  git: GitState | null;
  mode: string | null;
  status: string | null;
  running: boolean;
  // The slash commands the session's harness offers (GUI-004).
  commands: AvailableCommand[];
  // The latest model call's context and the model's window (HRN-013).
  context: { tokens: number; window: number } | null;
}

export interface AvailableCommand {
  name: string;
  description: string;
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
  let commands: AvailableCommand[] = [];
  // The latest checkpoint not yet tied to a user message, and the ranges
  // of events a rewind undid.
  let checkpoint: number | null = null;
  const rewound: { from: number; to: number }[] = [];
  // Seqs where the harness's context was replaced (history reset).
  const resets: number[] = [];
  let contextTokens = 0;
  let contextWindow = 0;
  // Model calls, all and since the last turn ended, and the model a call
  // that does not name its own was made with ("" until a switch: the
  // session's).
  const calls: ModelCall[] = [];
  let turnCalls: ModelCall[] = [];
  let current = { providerId: "", model: "" };

  const last = () => items[items.length - 1];
  for (const ev of events) {
    const d = (ev.data ?? {}) as Data;
    const key = String(ev.seq);
    const rev = ev.seq;
    switch (ev.type) {
      case "user_message_chunk": {
        // A resource_link is a file the message mentions (GUI-004).
        const prev = last();
        if (d.content?.type === "resource_link") {
          if (prev?.kind === "user") items[items.length - 1] = { ...prev, rev, files: [...prev.files, d.content.name ?? d.content.uri] };
          break;
        }
        items.push({ kind: "user", key, rev, text: d.content?.text ?? "", ts: ev.ts, files: [], checkpointSeq: checkpoint ?? undefined });
        checkpoint = null;
        break;
      }
      case "agent_message_chunk":
      case "agent_thought_chunk": {
        const kind = ev.type === "agent_message_chunk" ? "agent" : "thought";
        const prev = last();
        const text = d.content?.text ?? "";
        if (prev && prev.kind === kind) {
          items[items.length - 1] = { ...prev, rev, text: prev.text + text };
        } else {
          items.push({ kind, key, rev, text });
        }
        break;
      }
      case "pull_request":
        items.push({ kind: "pull_request", key, rev, url: d.url ?? "", title: d.title ?? "", base: d.base ?? "", head: d.head ?? "", draft: !!d.draft });
        break;
      case "handoff":
        items.push({ kind: "handoff", key, rev, fromTitle: d.fromTitle ?? "", summary: d.summary ?? "", generated: !!d.generated });
        break;
      case "history_message":
        if (d.reset) resets.push(ev.seq);
        break;
      case "available_commands_update":
        commands = Array.isArray(d.availableCommands) ? d.availableCommands : [];
        break;
      case "tool_call": {
        const item = {
          kind: "tool" as const,
          key,
          rev,
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
        // Agents send the command or path later, as a new title and input.
        const next = {
          ...item,
          rev,
          status: d.status ?? item.status,
          content: d.content ?? item.content,
          title: d.title || item.title,
          input: d.rawInput && Object.keys(d.rawInput).length > 0 ? d.rawInput : item.input,
        };
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
          form: d.form ?? undefined,
        };
        if (item) replaceTool(items, tools, { ...item, rev, permission });
        break;
      }
      case "permission_decided": {
        const item = tools.get(d.toolCallId);
        if (!item) break;
        const decided = { outcome: d.outcome, actor: d.actor, optionId: d.optionId, reason: d.reason, persisted: d.persisted };
        const permission: PermissionState = item.permission
          ? { ...item.permission, decided }
          : { requestId: d.requestId, options: [], decided };
        replaceTool(items, tools, { ...item, rev, permission });
        break;
      }
      case "turn_started":
        running = true;
        break;
      case "turn_ended":
        running = false;
        items.push({ kind: "turn_end", key, rev, stopReason: d.stopReason ?? "", ts: ev.ts, calls: turnCalls });
        turnCalls = [];
        break;
      case "usage_update": {
        const c: ModelCall = {
          providerId: d.model ? (d.providerId ?? "") : current.providerId,
          model: d.model || current.model,
          inputTokens: d.inputTokens ?? 0,
          outputTokens: d.outputTokens ?? 0,
          cacheReadTokens: d.cacheReadTokens ?? 0,
          cacheWriteTokens: d.cacheWriteTokens ?? 0,
        };
        calls.push(c);
        turnCalls.push(c);
        usage.inputTokens += c.inputTokens;
        usage.outputTokens += c.outputTokens;
        usage.cacheReadTokens += c.cacheReadTokens;
        usage.cacheWriteTokens += c.cacheWriteTokens;
        if (d.contextTokens) {
          contextTokens = d.contextTokens;
          contextWindow = d.contextWindow ?? 0;
        }
        break;
      }
      case "current_mode_update":
        mode = d.currentModeId ?? mode;
        items.push({ kind: "mode", key, rev, mode: d.currentModeId });
        break;
      case "checkpoint":
        checkpoint = ev.seq;
        break;
      case "rewind":
        rewound.push({ from: d.checkpointSeq ?? 0, to: ev.seq });
        resets.push(ev.seq);
        items.push({
          kind: "notice",
          key,
          rev,
          message: d.context
            ? "Rewound to before the dimmed turns: the files are restored and the conversation continues from there."
            : "Rewound the files to before the dimmed turns; the agent still remembers them.",
          warn: false,
        });
        break;
      case "env_health":
        items.push({
          kind: "notice",
          key,
          rev,
          message: d.status === "healthy" ? `Environment ready${d.detail ? `: ${d.detail}` : ""}.` : `Environment unhealthy: ${d.detail ?? ""}`,
          warn: d.status !== "healthy",
        });
        break;
      case "compaction_update":
        if (d.status === "completed") {
          items.push({ kind: "compaction", key, rev, reason: d.reason ?? "", before: d.contextTokens ?? 0, after: d.afterTokens ?? 0, summary: d.summary ?? "" });
          if (contextTokens > 0) contextTokens = d.afterTokens ?? 0;
        }
        break;
      case "notice":
        items.push({ kind: "notice", key, rev, message: d.message ?? "", warn: true });
        break;
      case "hook_ran": {
        // Hooks that pass stay quiet; one that blocked or failed says so
        // (EXT-005).
        const message = hookMessage(d);
        if (message) items.push({ kind: "notice", key, rev, message, warn: true });
        break;
      }
      case "model_changed":
        if (d.model) current = { providerId: d.providerId ?? current.providerId, model: d.model };
        items.push({ kind: "model", key, rev, model: d.model ?? "", providerId: d.providerId ?? "" });
        break;
      case "session_status":
        status = d.status ?? status;
        break;
      case "git_state":
        git = { repo: !!d.repo, branch: d.branch ?? "", head: d.head ?? "", dirty: d.dirty ?? 0, baseCommit: d.baseCommit ?? "" };
        break;
      case "error":
        items.push({ kind: "error", key, rev, message: d.message ?? "Unknown error" });
        break;
    }
  }
  const context = contextTokens > 0 ? { tokens: contextTokens, window: contextWindow } : null;
  for (let i = 0; i < items.length; i++) {
    const it = items[i];
    if (it.kind === "user" && it.checkpointSeq && resets.some((s) => s > it.checkpointSeq!)) items[i] = { ...it, filesOnly: true, rev: Math.max(it.rev, resets[resets.length - 1]) };
  }
  for (const r of rewound) {
    for (let i = 0; i < items.length; i++) {
      const seq = Number(items[i].key);
      if (seq > r.from && seq < r.to && !items[i].rewound) items[i] = { ...items[i], rewound: true, rev: r.to };
    }
  }
  return { items, usage, calls, git, mode, status, running, commands, context };
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

// ---- Right panel (GUI-001) ----

export type ToolItem = Extract<Item, { kind: "tool" }>;

export interface FileChange {
  path: string;
  edits: number;
  // The tool call of the latest edit, to jump to its card.
  lastToolCallId: string;
}

export interface PanelData {
  pending: ToolItem[];
  decided: ToolItem[];
  changes: FileChange[];
  turns: number;
}

// panelData collects what the right panel shows: approvals waiting and
// decided (mode approvals left out as noise), and the files the agent's
// finished edits touched, from the diffs in the transcript.
export function panelData(view: SessionView): PanelData {
  const pending: ToolItem[] = [];
  const decided: ToolItem[] = [];
  const changes = new Map<string, FileChange>();
  let turns = 0;
  for (const item of view.items) {
    if (item.kind === "turn_end") turns++;
    if (item.kind !== "tool") continue;
    if (awaitingDecision(item)) pending.push(item);
    const d = item.permission?.decided;
    if (d && !(d.actor === "mode" && d.outcome === "allowed")) decided.push(item);
    if (item.status !== "completed") continue;
    for (const c of item.content) {
      if (c.type !== "diff" || !c.path) continue;
      const prev = changes.get(c.path);
      changes.set(c.path, { path: c.path, edits: (prev?.edits ?? 0) + 1, lastToolCallId: item.toolCallId });
    }
  }
  return { pending, decided: decided.reverse(), changes: [...changes.values()].sort((a, b) => a.path.localeCompare(b.path)), turns };
}

// hookMessage describes a hook that blocked or failed, or returns "" for
// one that passed.
export function hookMessage(d: Record<string, any>): string {
  const what = `${d.event ?? "A"} hook${d.tool ? ` for ${d.tool}` : ""}`;
  const said = String(d.stderr || d.stdout || "").trim();
  if (d.stopped) return "";
  if (d.blocked) return `The ${what} blocked it: ${said || d.command}`;
  if (d.error) return `The ${what} could not run (${d.command}): ${d.error}`;
  if (d.timedOut) return `The ${what} timed out (${d.command}).`;
  if (d.exitCode) return `The ${what} failed with exit code ${d.exitCode} (${d.command})${said ? `: ${said}` : "."}`;
  return "";
}
