// A long session to measure with (NFR-PERF-002, issue 040): the TypeScript twin of
// internal/store/longfixture_test.go. The shape comes from counting event types and
// line sizes in one real transcript (81,325 events, 92 MB; types and byte counts
// only, no content); the text is words drawn from a list. Nothing is checked in:
// tests build it in memory, and scripts/appcheck/longsession.ts writes it as JSONL
// for the lived-in app check. Later issues (041, 043) reuse it.
import type { TranscriptEvent } from "./transcript.ts";

export type FixtureShape = "text" | "tools";
export const LONG_EVENTS = 80000;

const WORDS = `the a to of and in that it is was for on with as this be at by from or an are
file function return value error test session agent tool result line check update read write
build run path commit change model request store event index config option name type data
list item first next last before after because still only then when where which would should
could might need make find look see use try fix add remove move open close start stop wait`.split(/\s+/);

// A small seeded generator, so a run is the same every time.
function rng(seed: number) {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export interface LongFixture {
  events: TranscriptEvent[];
  lastError: number;
}

// longFixture makes n events of the shape, seq 1 to n, for session sid.
export function longFixture(shape: FixtureShape, n = LONG_EVENTS, sid = "ses_made_up"): LongFixture {
  const rand = rng(7);
  const int = (k: number) => Math.floor(rand() * k);
  const text = (size: number) => {
    let out = "";
    while (out.length < size) out += (out ? " " : "") + WORDS[int(WORDS.length)];
    return out;
  };
  const events: TranscriptEvent[] = [];
  let lastError = 0;
  const t0 = Date.parse("2026-11-01T09:00:00Z");
  const hex = () => (int(1e9) * 1e9 + int(1e9)).toString(16).padStart(40, "0");
  const emit = (type: string, data: unknown) => {
    if (events.length >= n) return;
    const seq = events.length + 1;
    events.push({ v: 1, seq, ts: new Date(t0 + seq * 180).toISOString(), sid, type, data } as TranscriptEvent);
    if (type === "error") lastError = seq;
  };
  const chunk = (type: string, size: number) =>
    emit(type, { messageId: "msg_made_up", content: { type: "text", text: text(size) } });
  const usage = () =>
    emit("usage_update", { inputTokens: 3000 + int(60000), outputTokens: 100 + int(900), cacheReadTokens: int(90000), cacheWriteTokens: int(3000) });
  const outputSize = () => {
    const p = int(100);
    return p < 60 ? 300 + int(500) : p < 96 ? 1500 + int(7500) : 15000 + int(30000);
  };
  let call = 0;
  const tool = () => {
    call++;
    const id = `toolu_made_up_${String(call).padStart(6, "0")}`;
    emit("tool_call", { toolCallId: id, title: "Run " + text(20), kind: ["read", "edit", "execute", "search"][int(4)], tool: "shell", status: "pending", rawInput: { command: text(40 + int(200)), description: text(40) } });
    if (int(3) === 0) emit("tool_call_update", { toolCallId: id, status: "in_progress" });
    const out = text(outputSize());
    emit("tool_call_update", { toolCallId: id, status: "completed", content: [{ type: "content", content: { type: "text", text: out } }], rawOutput: [{ type: "text", text: out }] });
  };
  const chunks = (k: number) => { for (let i = 0; i < k; i++) chunk("agent_message_chunk", 55 + int(30)); };
  while (events.length < n) {
    emit("turn_started", {});
    chunk("user_message_chunk", 60 + int(250));
    emit("checkpoint", { commit: hex(), historyLen: Math.floor(events.length / 40) });
    emit("session_status", { status: "running" });
    const length = 150 + int(500);
    const start = events.length;
    while (events.length - start < length && events.length < n) {
      if (shape === "text") {
        if (int(6) === 0) for (let i = 0, k = 10 + int(30); i < k; i++) chunk("agent_thought_chunk", 55 + int(30));
        chunks(40 + int(120));
        usage();
        if (int(4) === 0) tool();
      } else {
        chunks(2 + int(6));
        tool();
        if (int(3) === 0) tool();
        usage();
      }
    }
    if (int(12) === 0) {
      const seq = events.length;
      emit("permission_requested", { requestId: `perm_${seq}`, toolCallId: `toolu_made_up_${String(call).padStart(6, "0")}`, tool: "shell", kind: "execute", title: "Run a command", input: { command: text(200) }, command: text(200), options: [{ optionId: "allow_once", name: "Allow once", kind: "allow_once" }] });
      emit("permission_decided", { requestId: `perm_${seq}`, outcome: "allow_once", decidedBy: "user" });
    }
    // A last error well before the end, so finding it means looking back.
    const at = events.length;
    if ((at > n * 0.6 && at < n * 0.7 && lastError < n * 0.6) || (int(40) === 0 && at < n * 0.6)) emit("error", { message: "made-up error: " + text(80) });
    emit("git_state", { repo: true, branch: "main", head: hex(), dirty: int(9), baseCommit: hex() });
    emit("turn_ended", { stopReason: "end_turn" });
    emit("session_status", { status: "idle" });
  }
  return { events, lastError };
}

export function jsonl(events: readonly TranscriptEvent[]): string {
  return events.map((e) => JSON.stringify(e)).join("\n") + "\n";
}
