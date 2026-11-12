import { contextResent } from "./contextMeter.ts";
import type { ModelCall } from "./cost.ts";
import type { ModelContext } from "./modelContext.ts";
import { locale, t, type PlainKey } from "./i18n.ts";

// A session's transcript in the UI is a pure function of its events. Replay
// from disk and the live stream both go through apply, so they cannot drift.
// Its runtime imports are plain modules, so `node --test` can run it directly.
// What the app writes itself is in the language in force when the events are
// folded (GUI-048), so the view is folded again when the language changes.

export interface TranscriptEvent {
  v: number;
  seq: number;
  ts: string;
  sid: string;
  type: string;
  data: unknown;
}

// A transcript holds its events in one array that only grows: apply appends to it where
// it stands, so a batch costs its own size and not the transcript's (NFR-PERF-001).
// A transcript is the first `length` events of that array; one older than the latest
// copies what it needs before it appends, so no one sees events it did not have.
export interface Transcript {
  events: TranscriptEvent[];
  length: number;
  lastSeq: number;
}

export const emptyTranscript: Transcript = { events: [], length: 0, lastSeq: 0 };

export interface ApplyResult {
  transcript: Transcript;
  // When incoming events skip ahead (the bridge dropped some), the caller
  // fetches everything after this seq and applies it.
  missingAfter: number | null;
}

// apply appends the events that extend tr contiguously, ignores ones already
// seen, and stops at the first gap.
export function apply(tr: Transcript, incoming: readonly TranscriptEvent[]): ApplyResult {
  const sorted = [...incoming].sort((a, b) => a.seq - b.seq);
  const added: TranscriptEvent[] = [];
  let lastSeq = tr.lastSeq;
  for (const ev of sorted) {
    if (ev.seq <= lastSeq) continue;
    if (ev.seq !== lastSeq + 1) {
      return { transcript: extend(tr, added, lastSeq), missingAfter: lastSeq };
    }
    added.push(ev);
    lastSeq = ev.seq;
  }
  return { transcript: extend(tr, added, lastSeq), missingAfter: null };
}

function extend(tr: Transcript, added: TranscriptEvent[], lastSeq: number): Transcript {
  if (added.length === 0) return tr;
  // The shared array is appended to only by the newest transcript, and never when empty
  // (emptyTranscript is shared by everyone).
  const events = tr.length > 0 && tr.events.length === tr.length ? tr.events : tr.events.slice(0, tr.length);
  for (const ev of added) events.push(ev);
  return { events, length: events.length, lastSeq };
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
  // A guard hook that failed, which is why this is asked (EXT-005).
  hookFailed?: string;
  // An MCP server's request for input (MCP-008), asked as a form.
  form?: { server: string; message: string; schema?: unknown; url?: string };
  decided?: {
    outcome: string;
    actor: string;
    optionId?: string;
    reason?: string;
    persisted?: { tool: string; command?: string; path?: string; scope: string };
    // Where the user decided when not in this computer's window (GUI-052).
    via?: string;
  };
}

export interface ToolContent {
  type: string;
  content?: { type: string; text?: string; uri?: string; name?: string; mimeType?: string };
  path?: string;
  oldText?: string;
  newText?: string;
  exitCode?: number;
  output?: string;
  // A shell command a token optimizer rewrote (HRN-019): which one, and what ran.
  via?: string;
  command?: string;
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
      // Pictures the user attached (SES-015): names in the session's
      // attachments, which the view loads.
      images?: { name: string; mimeType: string }[];
      checkpointSeq?: number;
      // The context was replaced after the checkpoint (a compaction or an
      // earlier rewind): rewinding restores the files only.
      filesOnly?: boolean;
      // Where it was sent from when not this computer's window: "phone:<device>"
      // (GUI-052).
      via?: string;
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
      // The token optimizer that ran this command in a shorter form (HRN-019).
      via?: string;
    }
  | { kind: "turn_end"; key: string; rev: number; stopReason: string; ts: string; calls: ModelCall[] }
  | { kind: "error"; key: string; rev: number; message: string }
  | { kind: "mode"; key: string; rev: number; mode: string }
  // A stage of the session's workflow starting (WFL-008).
  | { kind: "stage"; key: string; rev: number; command: string; title: string }
  | { kind: "model"; key: string; rev: number; model: string; providerId: string }
  | { kind: "notice"; key: string; rev: number; message: string; warn: boolean; optimizer?: string }
  | { kind: "cache_resend"; key: string; rev: number; tokens: number }
  | { kind: "compaction"; key: string; rev: number; reason: string; before: number; after: number; summary: string }
  | { kind: "handoff"; key: string; rev: number; fromTitle: string; summary: string; generated: boolean }
  | { kind: "pull_request"; key: string; rev: number; url: string; title: string; base: string; head: string; draft: boolean }
  // The setup of the session's own worktree (SES-016), updated in place.
  | { kind: "setup"; key: string; rev: number; status: string; copied: string[]; missing: string[]; command: string; output: string; error: string };

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
  // The items in runs (toolRuns), kept from one view to the next as far as they did not change.
  runs: ItemRun[];
  usage: Usage;
  // Every model call, for cost estimates (PRV-008).
  calls: ModelCall[];
  // How an external agent's usage was counted (PRV-008): turns counted by their
  // total, and turns the agent reported no tokens for.
  measured: { turns: number; unreported: number };
  // What each kind of model call carried besides the conversation, as recorded
  // (NFR-PRIV-003); empty for an agent that builds its own requests.
  modelContext: ModelContext[];
  git: GitState | null;
  mode: string | null;
  // Counts changes to the session's model, effort and rules, for what shows them.
  settings: number;
  status: string | null;
  running: boolean;
  // The slash commands the session's harness offers (GUI-004).
  commands: AvailableCommand[];
  // The files the session's tools named and its messages mentioned, the most recent
  // first, as written (GUI-049): absolute, or relative to the root.
  touched: string[];
  // The latest model call's context and the model's window (HRN-013).
  context: { tokens: number; window: number } | null;
  // The worktree setup's latest status (SES-016), null when none ran.
  setup: string | null;
  // The agent's latest to-do list, from any harness (HRN-017).
  plan: PlanEntry[];
  // Whether the to-do list changed in the latest turn: one finished in an earlier turn
  // is not shown again when the next one starts.
  planThisTurn: boolean;
  // What the user's policy stopped (SEC-014).
  held: Held[];
  // Why the session is not getting on, and where it could continue (HRN-020); null once
  // a turn starts again or it continued elsewhere.
  stuck: Stuck | null;
  // The tool calls waiting for a decision, and the user messages a rewind can go back to
  // (the latest first); both are the same arrays until one changes.
  waiting: ToolItem[];
  // How many times the session's model was named (model items).
  models: number;
  rewindable: Extract<Item, { kind: "user" }>[];
  // What the right panel shows (panelData).
  panel: PanelData;
}

export interface Stuck {
  reason: string;
  detail: string;
  profileId?: string;
  profileName?: string;
  auto?: boolean;
}

// Held is one thing the user's policy stopped (SEC-014): a request denied by the
// user, a rule, the mode, a protected path, a hook or the sandbox, or an agent's own
// switch to a looser mode. seq is the moment in the transcript.
export interface Held {
  seq: number;
  kind: string;
  what: string;
  reason: string;
}

const HELD_KEYS: Record<string, PlainKey> = {
  user: "transcript.held.user",
  rule: "transcript.held.rule",
  mode: "transcript.held.mode",
  protected_path: "transcript.held.protectedPath",
  hook: "transcript.held.hook",
  sandbox: "transcript.held.sandbox",
  policy: "transcript.held.policy",
  mode_change: "transcript.held.modeChange",
};

// HELD_BY says who held it, by a decision's actor or "mode_change", in the language
// in force when it is read (GUI-048); a kind it does not know is undefined.
export const HELD_BY: Readonly<Record<string, string>> = Object.defineProperties(
  {},
  Object.fromEntries(Object.entries(HELD_KEYS).map(([kind, key]) => [kind, { get: () => t(key), enumerable: true }])),
);

export interface PlanEntry {
  content: string;
  status: string; // pending, in_progress or completed
}

export interface AvailableCommand {
  name: string;
  description: string;
  // What the command expects after its name, as an ACP agent describes it (GUI-049).
  input?: { hint?: string } | null;
}

type Data = Record<string, any>;

// A run of tool calls that read as one line (see toolRuns).
export interface ItemRun {
  items: Item[];
}

type ToolEntry = { item: ToolItem; i: number };

// A Fold is a session's view being built from its events, which it takes in order and
// keeps the state of, so new events cost what they are and not what came before them
// (NFR-PERF-001). view() gives what the conversation shows as of the events so far; what
// did not change since the last view is the same object, an item the same Item.
interface Fold {
  // How many events it has taken, and the language its words are in.
  count: number;
  loc: string;
  add(ev: TranscriptEvent): void;
  view(): SessionView;
}

function newFold(): Fold {
  const items: Item[] = [];
  // Tool calls by ID, with where their item stands in items: found without a scan.
  const tools = new Map<string, ToolEntry>();
  // What was held, named from its tool call once that has its final title. A form is
  // the user answering an MCP server, which holds nothing back.
  const denied: (Held & { toolCallId?: string })[] = [];
  const deniedCalls = new Set<string>();
  const forms = new Set<string>();
  const usage: Usage = { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0 };
  let mode: string | null = null;
  let settings = 0;
  let status: string | null = null;
  let running = false;
  let stuck: Stuck | null = null;
  let git: GitState | null = null;
  let commands: AvailableCommand[] = [];
  // In the order last named; a Map keeps it.
  const touched = new Map<string, true>();
  let touchedList: string[] = [];
  let touchedDirty = false;
  const touch = (path: unknown) => {
    if (typeof path !== "string" || !path) return;
    touched.delete(path);
    touched.set(path, true);
    touchedDirty = true;
  };
  const touchTool = (d: Data) => {
    if (Array.isArray(d.locations)) for (const l of d.locations) touch(l?.path);
    if (Array.isArray(d.content)) for (const c of d.content) if (c?.type === "diff") touch(c.path);
  };
  // The latest checkpoint not yet tied to a user message, and the seq of the latest
  // event that replaced the harness's context (a history reset or a rewind).
  let checkpoint: number | null = null;
  let lastReset = 0;
  // Where the user items with a checkpoint stand, and those a rewind has not undone.
  const checkpointed: number[] = [];
  let rewindable: Extract<Item, { kind: "user" }>[] = [];
  let rewindableDirty = false;
  let contextTokens = 0;
  let contextWindow = 0;
  // The context the latest model call sent, to see one sending it all again (GUI-036).
  let lastCallContext = 0;
  // Model calls, all and since the last turn ended, and the model a call
  // that does not name its own was made with ("" until a switch: the
  // session's).
  const calls: ModelCall[] = [];
  let callsList: ModelCall[] = [];
  let callsDirty = false;
  const measured = { turns: 0, unreported: 0 };
  const modelContext: ModelContext[] = [];
  let modelContextList: ModelContext[] = [];
  let modelContextDirty = false;
  let turnCalls: ModelCall[] = [];
  let setup: string | null = null;
  let plan: PlanEntry[] = [];
  let planThisTurn = false;
  let current = { providerId: "", model: "" };

  // Where the items that changed since the last view begin: the runs of tool calls and
  // the copy of items a view holds are made again from there.
  let changedFrom = Infinity;
  let itemsList: Item[] = [];
  const changed = (i: number) => {
    if (i < changedFrom) changedFrom = i;
  };
  const push = (item: Item) => {
    changed(items.length);
    items.push(item);
  };
  const put = (i: number, item: Item) => {
    changed(i);
    items[i] = item;
  };
  const last = () => items[items.length - 1];

  // The right panel's lists and the approvals waiting, kept by where their items stand
  // and made again only when one of those items changed (panelData).
  const waitingAt = new Set<number>();
  const decidedAt = new Set<number>();
  const editedAt = new Set<number>();
  let turnEnds = 0;
  let models = 0;
  let waiting: ToolItem[] = [];
  let waitingDirty = false;
  let panel: PanelData = { pending: [], decided: [], changes: [], turns: 0 };
  let panelDirty = false;
  let held: Held[] = [];
  let heldDirty = false;
  const edits = (item: ToolItem) => item.status === "completed" && item.content.some((c) => c.type === "diff" && !!c.path);
  const decidedHere = (item: ToolItem) => {
    const d = item.permission?.decided;
    return !!d && !(d.actor === "mode" && d.outcome === "allowed") && d.actor !== "sandbox";
  };
  // trackTool notes what the tool item at i now is for the lists above.
  const trackTool = (i: number) => {
    const item = items[i] as ToolItem;
    const was = [waitingAt.has(i), decidedAt.has(i), editedAt.has(i)];
    const is = [awaitingDecision(item), decidedHere(item), edits(item)];
    [waitingAt, decidedAt, editedAt].forEach((set, k) => {
      if (is[k]) set.add(i);
      else set.delete(i);
    });
    if (was[0] || is[0]) waitingDirty = true;
    if (was.some(Boolean) || is.some(Boolean)) panelDirty = true;
    if (deniedCalls.has(item.toolCallId)) heldDirty = true;
  };
  const deny = (h: Held & { toolCallId?: string }) => {
    denied.push(h);
    if (h.toolCallId) deniedCalls.add(h.toolCallId);
    heldDirty = true;
  };

  const replaceTool = (next: ToolItem) => {
    const entry = tools.get(next.toolCallId);
    if (!entry) return;
    put(entry.i, next);
    entry.item = next;
    trackTool(entry.i);
  };

  const pushUser = (item: Extract<Item, { kind: "user" }>) => {
    const ck = item.checkpointSeq;
    if (ck) {
      checkpointed.push(items.length);
      rewindableDirty = true;
      if (lastReset > ck) item = { ...item, filesOnly: true, rev: Math.max(item.rev, lastReset) };
    }
    push(item);
  };
  // reset: the context was replaced at seq, so a message with an earlier checkpoint
  // can restore the files only (SES-014).
  const reset = (seq: number) => {
    lastReset = seq;
    for (const i of checkpointed) {
      const it = items[i];
      if (it.kind === "user" && it.checkpointSeq! < seq) put(i, { ...it, filesOnly: true, rev: Math.max(it.rev, seq) });
    }
  };
  // markRewound: a rewind at `to` undid what came after the checkpoint `from`.
  const markRewound = (from: number, to: number) => {
    rewindableDirty = true;
    // Keys are the seqs the items began at, so they ascend.
    let lo = 0;
    let hi = items.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (Number(items[mid].key) > from) hi = mid;
      else lo = mid + 1;
    }
    for (let i = lo; i < items.length && Number(items[i].key) < to; i++) {
      if (!items[i].rewound) put(i, { ...items[i], rewound: true, rev: to });
    }
  };

  const add = (ev: TranscriptEvent) => {
    const d = (ev.data ?? {}) as Data;
    const key = String(ev.seq);
    const rev = ev.seq;
    switch (ev.type) {
      case "user_message_chunk": {
        // A resource_link is a file the message mentions (GUI-004).
        const prev = last();
        if (d.content?.type === "resource_link") {
          if (prev?.kind === "user") put(items.length - 1, { ...prev, rev, files: [...prev.files, d.content.name ?? d.content.uri] });
          touch(d.content.name);
          break;
        }
        if (d.content?.type === "image") {
          const uri = typeof d.content.uri === "string" ? d.content.uri : "";
          const image = { name: uri.slice("attachment:".length), mimeType: d.content.mimeType ?? "" };
          if (uri.startsWith("attachment:")) {
            if (prev?.kind === "user") put(items.length - 1, { ...prev, rev, images: [...(prev.images ?? []), image] });
            // An imported message may start with its picture (SES-020).
            else pushUser({ kind: "user", key, rev, text: "", ts: ev.ts, files: [], images: [image], checkpointSeq: checkpoint ?? undefined });
          }
          break;
        }
        pushUser({ kind: "user", key, rev, text: d.content?.text ?? "", ts: ev.ts, files: [], checkpointSeq: checkpoint ?? undefined, ...(d.via ? { via: d.via } : {}) });
        checkpoint = null;
        break;
      }
      case "agent_message_chunk":
      case "agent_thought_chunk": {
        const kind = ev.type === "agent_message_chunk" ? "agent" : "thought";
        const prev = last();
        const text = d.content?.text ?? "";
        if (prev && prev.kind === kind) {
          put(items.length - 1, { ...prev, rev, text: prev.text + text });
        } else {
          push({ kind, key, rev, text });
        }
        break;
      }
      case "pull_request":
        push({ kind: "pull_request", key, rev, url: d.url ?? "", title: d.title ?? "", base: d.base ?? "", head: d.head ?? "", draft: !!d.draft });
        break;
      case "plan":
        planThisTurn = true;
        plan = Array.isArray(d.entries) ? d.entries.filter((x: PlanEntry) => typeof x?.content === "string").map((x: PlanEntry) => ({ content: x.content, status: String(x.status ?? "pending") })) : [];
        break;
      case "worktree_setup": {
        setup = d.status ?? null;
        // A run's later reports replace its card; a retry starts a new one.
        let at = items.length - 1;
        while (at >= 0 && items[at].kind !== "setup") at--;
        const prev = at >= 0 ? items[at] : undefined;
        const item = {
          kind: "setup" as const,
          key: prev && prev.kind === "setup" && prev.status === "running" ? prev.key : key,
          rev,
          status: d.status ?? "",
          copied: d.copied ?? [],
          missing: d.missing ?? [],
          command: d.command ?? (prev?.kind === "setup" && d.status === "skipped" ? prev.command : ""),
          output: d.output ?? "",
          error: d.error ?? "",
        };
        if (prev && prev.kind === "setup" && (prev.status === "running" || (d.status === "skipped" && prev.status === "failed"))) put(at, item);
        else push(item);
        break;
      }
      case "handoff":
        push({ kind: "handoff", key, rev, fromTitle: d.fromTitle ?? "", summary: d.summary ?? "", generated: !!d.generated });
        break;
      case "history_message":
        if (d.reset) reset(ev.seq);
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
          title: d.title ?? d.tool ?? t("transcript.toolCall"),
          toolKind: d.kind ?? "other",
          status: d.status ?? "pending",
          input: d.rawInput,
          content: [] as ToolContent[],
        };
        tools.set(d.toolCallId, { item, i: items.length });
        push(item);
        trackTool(items.length - 1);
        touchTool(d);
        break;
      }
      case "tool_call_update": {
        const item = tools.get(d.toolCallId)?.item;
        if (!item) break;
        touchTool(d);
        // Agents send the command or path later, as a new title and input.
        const next = {
          ...item,
          rev,
          status: d.status ?? item.status,
          content: d.content ?? item.content,
          title: d.title || item.title,
          input: d.rawInput && Object.keys(d.rawInput).length > 0 ? d.rawInput : item.input,
          via: d.via ?? item.via,
        };
        replaceTool(next);
        break;
      }
      case "permission_requested": {
        if (d.form) forms.add(d.requestId);
        const item = tools.get(d.toolCallId)?.item;
        const permission: PermissionState = {
          requestId: d.requestId,
          options: d.options ?? [],
          command: d.command,
          paths: d.paths ?? undefined,
          hookFailed: d.hookFailed || undefined,
          form: d.form ?? undefined,
        };
        if (item) replaceTool({ ...item, rev, permission });
        break;
      }
      case "permission_decided": {
        const item = tools.get(d.toolCallId)?.item;
        if (d.outcome === "denied" && !forms.has(d.requestId)) {
          deny({ seq: item ? Number(item.key) : ev.seq, kind: d.actor ?? "", what: "", reason: d.reason ?? "", toolCallId: d.toolCallId });
        }
        if (!item) break;
        const decided = { outcome: d.outcome, actor: d.actor, optionId: d.optionId, reason: d.reason, persisted: d.persisted, ...(d.via ? { via: d.via } : {}) };
        const permission: PermissionState = item.permission
          ? { ...item.permission, decided }
          : { requestId: d.requestId, options: [], decided };
        replaceTool({ ...item, rev, permission });
        break;
      }
      case "mode_refused":
        // The user chose the mode approving a plan, and the app does not allow it here.
        if (d.chosen) {
          const message = d.reason ? t("transcript.modeRefused.chosen", { mode: String(d.to), reason: String(d.reason) }) : t("transcript.modeRefused.chosenUnavailable", { mode: String(d.to) });
          push({ kind: "notice", key, rev, message, warn: true });
          break;
        }
        push({ kind: "notice", key, rev, message: t("transcript.modeRefused.agent", { from: String(d.from), to: String(d.to) }), warn: false });
        deny({ seq: ev.seq, kind: "mode_change", what: t("transcript.held.modeChangeWhat", { from: String(d.from), to: String(d.to) }), reason: "" });
        break;
      case "turn_started":
        running = true;
        stuck = null;
        planThisTurn = false;
        break;
      case "stuck":
        stuck = { reason: d.reason ?? "", detail: d.detail ?? "", profileId: d.profileId, profileName: d.profileName, auto: !!d.auto };
        push({ kind: "notice", key, rev, message: t("transcript.stuck", { detail: String(d.detail ?? d.reason) }), warn: true });
        break;
      case "workflow_stage":
        push({ kind: "stage", key, rev, command: d.command ?? "", title: d.title || `/${d.command ?? ""}` });
        break;
      case "handed_off":
        stuck = null;
        push({
          kind: "notice",
          key,
          rev,
          message: d.profileName != null ? t("transcript.handedOff", { title: String(d.toTitle ?? ""), profile: String(d.profileName) }) : t("transcript.handedOffAnother", { title: String(d.toTitle ?? "") }),
          warn: false,
        });
        break;
      case "turn_ended":
        running = false;
        turnEnds++;
        push({ kind: "turn_end", key, rev, stopReason: d.stopReason ?? "", ts: ev.ts, calls: turnCalls });
        turnCalls = [];
        break;
      case "model_context":
        modelContext.push(d as ModelContext);
        modelContextDirty = true;
        break;
      case "usage_update": {
        if (d.measured === "turn") measured.turns++;
        if (d.measured === "none") measured.unreported++;
        const c: ModelCall = {
          providerId: d.model ? (d.providerId ?? "") : current.providerId,
          model: d.model || current.model,
          inputTokens: d.inputTokens ?? 0,
          outputTokens: d.outputTokens ?? 0,
          cacheReadTokens: d.cacheReadTokens ?? 0,
          cacheWriteTokens: d.cacheWriteTokens ?? 0,
          cacheWrite1hTokens: d.cacheWrite1hTokens ?? 0,
          adjustment: d.adjustment === true,
          turn: d.measured === "turn",
        };
        // An agent's report of its context spends nothing and is no call.
        const spent = c.inputTokens + c.outputTokens + c.cacheReadTokens + c.cacheWriteTokens > 0;
        if (spent) {
          calls.push(c);
          callsDirty = true;
          turnCalls.push(c);
          usage.inputTokens += c.inputTokens;
          usage.outputTokens += c.outputTokens;
          usage.cacheReadTokens += c.cacheReadTokens;
          usage.cacheWriteTokens += c.cacheWriteTokens;
        }
        if (d.contextTokens) {
          if (spent) {
            if (contextResent(lastCallContext, d.contextTokens, c.cacheWriteTokens)) push({ kind: "cache_resend", key, rev, tokens: c.cacheWriteTokens });
            lastCallContext = d.contextTokens;
          }
          contextTokens = d.contextTokens;
          // A call may come before the agent has said what its window is.
          contextWindow = d.contextWindow ?? contextWindow;
        }
        break;
      }
      case "current_mode_update":
        mode = d.currentModeId ?? mode;
        push({ kind: "mode", key, rev, mode: d.currentModeId });
        break;
      case "checkpoint":
        checkpoint = ev.seq;
        break;
      case "rewind":
        markRewound(d.checkpointSeq ?? 0, ev.seq);
        reset(ev.seq);
        push({
          kind: "notice",
          key,
          rev,
          message: d.context ? t("transcript.rewound") : t("transcript.rewoundFiles"),
          warn: false,
        });
        break;
      case "env_health": {
        const healthy = d.status === "healthy";
        const detail = String(d.detail ?? "");
        const message = !healthy ? t("transcript.env.unhealthy", { detail }) : detail ? t("transcript.env.readyDetail", { detail }) : t("transcript.env.ready");
        push({ kind: "notice", key, rev, message, warn: !healthy });
        break;
      }
      case "compaction_update":
        if (d.status === "completed") {
          push({ kind: "compaction", key, rev, reason: d.reason ?? "", before: d.contextTokens ?? 0, after: d.afterTokens ?? 0, summary: d.summary ?? "" });
          if (contextTokens > 0) contextTokens = d.afterTokens ?? 0;
          lastCallContext = 0;
        }
        break;
      case "notice":
        push({ kind: "notice", key, rev, message: d.message ?? "", warn: true });
        break;
      case "hook_ran": {
        // Hooks that pass stay quiet; one that blocked or failed says so
        // (EXT-005).
        const message = hookMessage(d);
        if (message) push({ kind: "notice", key, rev, message, warn: true });
        break;
      }
      case "model_changed":
        if (d.model) current = { providerId: d.providerId ?? current.providerId, model: d.model };
        models++;
        push({ kind: "model", key, rev, model: d.model ?? "", providerId: d.providerId ?? "" });
        break;
      case "effort_changed":
        settings++;
        push({ kind: "notice", key, rev, message: t("transcript.effort", { effort: String(d.effort) }), warn: false });
        break;
      case "config_option_update":
        settings++;
        break;
      case "session_rule_changed":
        settings++;
        break;
      case "optimizer_changed":
        push({ kind: "notice", key, rev, optimizer: d.optimizer ?? "", message: d.name ? t("transcript.optimizer.set", { name: String(d.name) }) : t("transcript.optimizer.off"), warn: false });
        break;
      case "session_status":
        status = d.status ?? status;
        break;
      case "git_state":
        git = { repo: !!d.repo, branch: d.branch ?? "", head: d.head ?? "", dirty: d.dirty ?? 0, baseCommit: d.baseCommit ?? "" };
        break;
      case "error":
        push({ kind: "error", key, rev, message: d.message ?? t("transcript.unknownError") });
        break;
    }
  };

  // Runs of tool calls: the runs before the one the first change falls in (and the one
  // before that, which it may join) are kept.
  let runs: ItemRun[] = [];
  let runStarts: number[] = [];
  const makeRuns = () => {
    const r = Math.max(0, upperBound(runStarts, Math.min(changedFrom, items.length)) - 2);
    const keep = Math.min(r, runs.length);
    const start = runStarts[keep] ?? 0;
    runs = runs.slice(0, keep);
    runStarts = runStarts.slice(0, keep);
    extendRuns(runs, runStarts, items, start);
  };

  let snapshot: SessionView | null = null;
  let snapshotCount = -1;
  const fold: Fold = {
    count: 0,
    loc: locale(),
    add(ev) {
      add(ev);
      fold.count++;
    },
    view() {
      if (snapshot && snapshotCount === fold.count) return snapshot;
      if (changedFrom !== Infinity) {
        makeRuns();
        itemsList = items.slice();
        changedFrom = Infinity;
      }
      if (touchedDirty) {
        touchedList = [...touched.keys()].reverse();
        touchedDirty = false;
      }
      if (callsDirty) {
        callsList = calls.slice();
        callsDirty = false;
      }
      if (modelContextDirty) {
        modelContextList = modelContext.slice();
        modelContextDirty = false;
      }
      if (rewindableDirty) {
        rewindable = checkpointed
          .flatMap((i) => {
            const it = items[i];
            return it.kind === "user" && !it.rewound ? [it] : [];
          })
          .reverse();
        rewindableDirty = false;
      }
      if (heldDirty) {
        held = denied.map(({ toolCallId, ...h }) => {
          const tool = toolCallId ? tools.get(toolCallId)?.item : undefined;
          return { ...h, what: h.what || tool?.permission?.command || tool?.title || h.reason || t("transcript.aRequest") };
        });
        heldDirty = false;
      }
      if (waitingDirty) {
        waiting = [...waitingAt].sort((a, b) => a - b).map((i) => items[i] as ToolItem);
        waitingDirty = false;
        panelDirty = true;
      }
      if (panelDirty || panel.turns !== turnEnds) {
        const changes = new Map<string, FileChange>();
        for (const i of [...editedAt].sort((a, b) => a - b)) {
          const item = items[i] as ToolItem;
          for (const c of item.content) {
            if (c.type !== "diff" || !c.path) continue;
            const prev = changes.get(c.path);
            changes.set(c.path, { path: c.path, edits: (prev?.edits ?? 0) + 1, lastToolCallId: item.toolCallId });
          }
        }
        panel = {
          pending: waiting,
          decided: [...decidedAt].sort((a, b) => b - a).map((i) => items[i] as ToolItem),
          changes: [...changes.values()].sort((a, b) => a.path.localeCompare(b.path)),
          turns: turnEnds,
        };
        panelDirty = false;
      }
      snapshotCount = fold.count;
      snapshot = {
        items: itemsList,
        runs,
        usage: { ...usage },
        calls: callsList,
        measured: { ...measured },
        modelContext: modelContextList,
        git,
        mode,
        status,
        running,
        commands,
        touched: touchedList,
        context: contextTokens > 0 ? { tokens: contextTokens, window: contextWindow } : null,
        setup,
        plan,
        planThisTurn,
        settings,
        held,
        stuck,
        waiting,
        models,
        rewindable,
        panel,
      };
      return snapshot;
    },
  };
  return fold;
}

// upperBound is the index of the first element above x in a sorted list.
function upperBound(sorted: readonly number[], x: number): number {
  let lo = 0;
  let hi = sorted.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (sorted[mid] > x) hi = mid;
    else lo = mid + 1;
  }
  return lo;
}

// quiet says a tool call is finished and unremarkable.
function quiet(i: Item): boolean {
  return i.kind === "tool" && i.status === "completed" && i.tool !== "present_plan" && !awaitingDecision(i) && (!i.permission?.decided || i.permission.decided.outcome === "allowed");
}

// extendRuns groups items from index `from` on into runs after the ones already in
// `runs`, noting where each begins. A run it did not make is copied before it takes
// another item, since an earlier view holds it.
function extendRuns(runs: ItemRun[], starts: number[], items: readonly Item[], from: number) {
  const fresh = runs.length;
  for (let i = from; i < items.length; i++) {
    const item = items[i];
    const end = runs.length - 1;
    if (end >= 0 && quiet(item) && quiet(runs[end].items[0])) {
      if (end < fresh) runs[end] = { items: [...runs[end].items] };
      runs[end].items.push(item);
    } else {
      runs.push({ items: [item] });
      starts.push(i);
    }
  }
}

// buildView folds a session's events into what the conversation shows.
export function buildView(events: readonly TranscriptEvent[]): SessionView {
  const fold = newFold();
  for (const ev of events) fold.add(ev);
  return fold.view();
}

// The folds of the transcripts the window holds, each keyed by the events array its
// transcripts share.
const folds = new WeakMap<TranscriptEvent[], Fold>();

// viewOf is buildView(tr.events.slice(0, tr.length)) for a transcript, going on from where
// the last call for its events left off. It starts over when it must: for a transcript
// older than the one last folded, and when the language changed, which the app's own
// words follow (GUI-048).
export function viewOf(tr: Transcript): SessionView {
  let fold = folds.get(tr.events);
  if (!fold || fold.count > tr.length || fold.loc !== locale()) {
    fold = newFold();
    folds.set(tr.events, fold);
  }
  for (let i = fold.count; i < tr.length; i++) fold.add(tr.events[i]);
  return fold.view();
}

// hookFailedText says why a request reached the user although the mode or a rule
// might have allowed it: a guard hook did not answer (EXT-005).
export function hookFailedText(why: string): string {
  return t("transcript.hookFailed", { why });
}

// pendingRequest returns the permission the user still has to answer.
export function awaitingDecision(item: Item): boolean {
  return item.kind === "tool" && !!item.permission && !item.permission.decided && item.permission.options.length > 0;
}

// toolRuns groups finished tool calls that follow each other, so a long stretch
// of them reads as one line. A call still running, failed, refused or waiting
// stands alone, as does a plan: they are what the user has to see.
export function toolRuns(items: readonly Item[]): ItemRun[] {
  const runs: ItemRun[] = [];
  extendRuns(runs, [], items, 0);
  return runs;
}

export interface DiffLine {
  sign: " " | "-" | "+";
  text: string;
}

// lineDiff is a small diff for edit previews: common leading and trailing
// lines are trimmed to `context` lines, the rest shows as removed then added.
export function lineDiff(oldText: string, newText: string, context = 2): DiffLine[] {
  const lines = (text: string) => (text === "" ? [] : text.replace(/\n$/, "").split("\n"));
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
// decided (mode and sandbox approvals left out as noise), and the files the agent's
// finished edits touched, from the diffs in the transcript.
export function panelData(view: SessionView): PanelData {
  return view.panel;
}

// optimizerCounts says how many shell commands ran while an optimizer was on and how
// many it shortened (HRN-019). A session started with one has no event for that, so
// it counts as on from the start unless its first change turned one on.
export function optimizerCounts(view: { items: Item[] }, on: boolean): { commands: number; rewritten: number } {
  const changes = view.items.filter((i) => i.kind === "notice" && i.optimizer !== undefined);
  let active = changes.length > 0 ? (changes[0] as { optimizer?: string }).optimizer === "" : on;
  let commands = 0;
  let rewritten = 0;
  for (const item of view.items) {
    if (item.kind === "notice" && item.optimizer !== undefined) active = item.optimizer !== "";
    if (item.kind !== "tool" || item.toolKind !== "execute" || !active) continue;
    commands++;
    if (item.via || item.content.some((c) => c.via)) rewritten++;
  }
  return { commands, rewritten };
}

// hookMessage describes a hook that blocked or failed, or returns "" for
// one that passed.
export function hookMessage(d: Record<string, any>): string {
  // The engine always names the event; without one, English said "The A hook".
  const v = { event: String(d.event ?? "A"), tool: String(d.tool ?? ""), command: String(d.command ?? "") };
  const said = String(d.stderr || d.stdout || "").trim();
  if (d.stopped) return "";
  if (d.blocked) return t(d.tool ? "transcript.hook.blockedFor" : "transcript.hook.blocked", { ...v, said: said || v.command });
  if (d.error) return t(d.tool ? "transcript.hook.couldNotRunFor" : "transcript.hook.couldNotRun", { ...v, error: String(d.error) });
  if (d.timedOut) return t(d.tool ? "transcript.hook.timedOutFor" : "transcript.hook.timedOut", v);
  if (d.exitCode) {
    const code = String(d.exitCode);
    if (said) return t(d.tool ? "transcript.hook.failedSaidFor" : "transcript.hook.failedSaid", { ...v, code, said });
    return t(d.tool ? "transcript.hook.failedFor" : "transcript.hook.failed", { ...v, code });
  }
  return "";
}

// phoneName is the device a message or decision came from, from its via
// ("phone:<device>", GUI-052); "" when it came from this computer's window.
export function phoneName(via: string | undefined): string {
  return via?.startsWith("phone:") ? via.slice("phone:".length) || "?" : "";
}
