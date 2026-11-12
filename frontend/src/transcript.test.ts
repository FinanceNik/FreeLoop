import { test } from "node:test";
import assert from "node:assert/strict";
import { optimizerCounts, apply, awaitingDecision, buildView as foldOnce, emptyTranscript, viewOf, type SessionView, type Transcript, HELD_BY, hookFailedText, lineDiff, panelData, summary, toolRuns, type Item, type TranscriptEvent } from "./transcript.ts";
import { setLanguage } from "./i18n.ts";

// stepped folds events the way a window gets them: in batches through apply, the view
// taken after each (NFR-PERF-001). cuts says where the batches end.
function stepped(events: readonly TranscriptEvent[], cuts: readonly number[]): SessionView {
  // Built as apply does, but without its demand that seqs run from 1: some fixtures start later.
  const held: TranscriptEvent[] = [];
  let view = viewOf({ events: held, length: 0, lastSeq: 0 });
  let from = 0;
  for (const to of [...cuts, events.length]) {
    if (to <= from) continue;
    held.push(...events.slice(from, to));
    view = viewOf({ events: held, length: held.length, lastSeq: events[to - 1].seq });
    from = to;
  }
  return view;
}

// buildView here is the fold at once, and every use of it in this file also checks that
// the same events folded in steps (one at a time, in halves, and at seeded random cuts)
// give an equal view: the fold goes on from where it was without drifting.
function buildView(events: readonly TranscriptEvent[]): SessionView {
  const once = foldOnce(events);
  const n = events.length;
  const rand = (seed: number) => () => ((seed = (seed * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff);
  const ones = Array.from({ length: n }, (_, i) => i);
  const random = (seed: number) => {
    const r = rand(seed);
    return Array.from({ length: Math.ceil(n / 3) }, () => Math.floor(r() * n)).sort((a, b) => a - b);
  };
  for (const cuts of [ones, [n >> 1], random(1), random(2), random(3)]) assert.deepStrictEqual(stepped(events, cuts), once);
  return once;
}

const ev = (seq: number, data: unknown = {}): TranscriptEvent => ({
  v: 1,
  seq,
  ts: "2026-10-28T00:00:00Z",
  sid: "ses_1",
  type: "notice",
  data,
});

test("appends contiguous events", () => {
  const r = apply(emptyTranscript, [ev(1), ev(2)]);
  assert.equal(r.missingAfter, null);
  assert.equal(r.transcript.lastSeq, 2);
  assert.deepEqual(r.transcript.events.map((e) => e.seq), [1, 2]);
});

test("accepts out-of-order input within one batch", () => {
  const r = apply(emptyTranscript, [ev(2), ev(1), ev(3)]);
  assert.equal(r.transcript.lastSeq, 3);
  assert.equal(r.missingAfter, null);
});

test("ignores duplicates and returns the same object when nothing changes", () => {
  const t = apply(emptyTranscript, [ev(1), ev(2)]).transcript;
  const r = apply(t, [ev(1), ev(2)]);
  assert.equal(r.transcript, t);
  assert.equal(r.missingAfter, null);
});

test("stops at a gap and reports where to fetch from", () => {
  const t = apply(emptyTranscript, [ev(1)]).transcript;
  const r = apply(t, [ev(2), ev(4), ev(5)]);
  assert.equal(r.transcript.lastSeq, 2);
  assert.equal(r.missingAfter, 2);
});

test("replay then live is the same as live alone", () => {
  const replayed = apply(apply(emptyTranscript, [ev(1), ev(2)]).transcript, [ev(2), ev(3)]).transcript;
  const live = apply(emptyTranscript, [ev(1), ev(2), ev(3)]).transcript;
  assert.deepEqual(replayed, live);
});

test("summary prefers text content", () => {
  assert.equal(summary(ev(1, { content: { type: "text", text: "hi" } })), "hi");
  assert.equal(summary(ev(1, { status: "idle" })), "idle");
  assert.equal(summary(ev(1, { a: 1 })), '{"a":1}');
});

const at = (seq: number, type: string, data: unknown): TranscriptEvent => ({ v: 1, seq, ts: "2026-10-28T00:00:00Z", sid: "s", type, data });

test("a request a failed guard hook put to the user says so", () => {
  const v = buildView([
    at(1, "tool_call", { toolCallId: "c1", title: "Write a.go", kind: "edit", status: "pending" }),
    at(2, "permission_requested", { requestId: "r1", toolCallId: "c1", hookFailed: 'the pre_tool_use hook "guard" timed out after 1 s', options: [] }),
    at(3, "tool_call", { toolCallId: "c2", title: "Write b.go", kind: "edit", status: "pending" }),
    at(4, "permission_requested", { requestId: "r2", toolCallId: "c2", options: [] }),
  ]);
  const [a, b] = v.items as any[];
  assert.equal(a.permission.hookFailed, 'the pre_tool_use hook "guard" timed out after 1 s');
  assert.equal(b.permission.hookFailed, undefined);
  assert.match(hookFailedText(a.permission.hookFailed), /guard hook did not answer: the pre_tool_use hook "guard" timed out after 1 s\./);
});

test("buildView folds a tool-calling turn", () => {
  const v = buildView([
    at(1, "turn_started", {}),
    at(2, "user_message_chunk", { content: { type: "text", text: "fix it" } }),
    at(3, "agent_thought_chunk", { content: { type: "text", text: "hm " } }),
    at(4, "agent_thought_chunk", { content: { type: "text", text: "ok" } }),
    at(5, "agent_message_chunk", { content: { type: "text", text: "Let me " } }),
    at(6, "agent_message_chunk", { content: { type: "text", text: "look." } }),
    at(7, "usage_update", { inputTokens: 10, outputTokens: 5 }),
    at(8, "tool_call", { toolCallId: "c1", title: "Write a.go", kind: "edit", status: "pending", tool: "write_file" }),
    at(9, "permission_requested", { requestId: "r1", toolCallId: "c1", options: [{ optionId: "allow_once", name: "Allow once", kind: "allow_once" }] }),
  ]);
  assert.deepEqual(v.items.map((i) => i.kind), ["user", "thought", "agent", "tool"]);
  assert.equal((v.items[1] as any).text, "hm ok");
  assert.equal((v.items[2] as any).text, "Let me look.");
  assert.equal(awaitingDecision(v.items[3]), true);
  assert.equal(v.running, true);

  const v2 = buildView([
    at(8, "tool_call", { toolCallId: "c1", title: "Write a.go", kind: "edit", status: "pending" }),
    at(9, "permission_requested", { requestId: "r1", toolCallId: "c1", options: [{ optionId: "allow_once" }] }),
    at(10, "permission_decided", { requestId: "r1", toolCallId: "c1", outcome: "allowed", actor: "user" }),
    at(11, "tool_call_update", { toolCallId: "c1", status: "completed", content: [{ type: "diff", path: "a.go", oldText: "", newText: "x" }] }),
    at(12, "usage_update", { inputTokens: 20, outputTokens: 1, cacheReadTokens: 7 }),
    at(13, "turn_ended", { stopReason: "end_turn" }),
  ]);
  const tool = v2.items[0] as any;
  assert.equal(tool.status, "completed");
  assert.equal(tool.permission.decided.outcome, "allowed");
  assert.equal(awaitingDecision(tool), false);
  assert.equal(v2.usage.inputTokens, 20);
  assert.equal(v2.usage.cacheReadTokens, 7);
  assert.equal(v2.running, false);
});

test("lineDiff trims common lines to context", () => {
  const d = lineDiff("a\nb\nc\nd\ne\nf", "a\nb\nc\nX\ne\nf", 1);
  assert.deepEqual(d.map((l) => l.sign + l.text), [" c", "-d", "+X", " e"]);
  assert.deepEqual(lineDiff("", "new\n").map((l) => l.sign + l.text), ["+new"]);
});

test("buildView keeps the latest git state", () => {
  const v = buildView([
    at(1, "git_state", { repo: true, branch: "main", head: "a", dirty: 0, baseCommit: "a" }),
    at(2, "git_state", { repo: true, branch: "main", head: "a", dirty: 3, baseCommit: "a" }),
  ]);
  assert.equal(v.git?.dirty, 3);
  assert.equal(buildView([]).git, null);
});

test("buildView attaches mentioned files to the user message", () => {
  const v = buildView([
    at(1, "user_message_chunk", { content: { type: "text", text: "fix @a.go" } }),
    at(2, "user_message_chunk", { content: { type: "resource_link", uri: "file:///r/a.go", name: "a.go" } }),
  ]);
  assert.equal(v.items.length, 1);
  assert.deepEqual(v.items[0].kind === "user" && v.items[0].files, ["a.go"]);
  assert.equal(v.items[0].rev, 2);
});

test("panelData lists approvals and finished edits", () => {
  const v = buildView([
    at(1, "tool_call", { toolCallId: "c1", title: "Write a.go", kind: "edit", status: "pending" }),
    at(2, "permission_requested", { requestId: "r1", toolCallId: "c1", options: [{ optionId: "allow_once" }] }),
    at(3, "permission_decided", { requestId: "r1", toolCallId: "c1", outcome: "allowed", actor: "user" }),
    at(4, "tool_call_update", { toolCallId: "c1", status: "completed", content: [{ type: "diff", path: "a.go", oldText: "", newText: "x" }] }),
    at(5, "tool_call", { toolCallId: "c2", title: "Edit a.go", kind: "edit", status: "pending" }),
    at(6, "permission_decided", { requestId: "r2", toolCallId: "c2", outcome: "allowed", actor: "mode" }),
    at(7, "tool_call_update", { toolCallId: "c2", status: "completed", content: [{ type: "diff", path: "a.go", oldText: "x", newText: "y" }] }),
    at(8, "tool_call", { toolCallId: "c3", title: "Write b.go", kind: "edit", status: "pending", content: [{ type: "diff", path: "b.go", newText: "z" }] }),
    at(9, "permission_requested", { requestId: "r3", toolCallId: "c3", options: [{ optionId: "allow_once" }] }),
    at(10, "turn_ended", { stopReason: "end_turn" }),
  ]);
  const p = panelData(v);
  assert.deepEqual(p.pending.map((t) => t.toolCallId), ["c3"]);
  assert.deepEqual(p.decided.map((t) => t.toolCallId), ["c1"]);
  assert.deepEqual(p.changes, [{ path: "a.go", edits: 2, lastToolCallId: "c2" }]);
  assert.equal(p.turns, 1);
});

test("buildView keeps the latest advertised commands", () => {
  const cmd = (seq: number, names: string[]): TranscriptEvent => ({
    ...ev(seq, { availableCommands: names.map((name) => ({ name, description: "" })) }),
    type: "available_commands_update",
  });
  assert.deepEqual(buildView([]).commands, []);
  assert.deepEqual(
    buildView([cmd(1, ["a", "b"]), cmd(2, ["review"])]).commands.map((c) => c.name),
    ["review"],
  );
});

const typed = (seq: number, type: string, data: unknown): TranscriptEvent => ({ ...ev(seq, data), type });

test("buildView keeps an agent's context report out of the calls, and its window for later calls", () => {
  const v = buildView([
    typed(1, "usage_update", { inputTokens: 5, cacheReadTokens: 100, contextTokens: 105, model: "m" }),
    typed(2, "usage_update", { contextTokens: 150, contextWindow: 1000 }),
    typed(3, "usage_update", { inputTokens: 5, cacheReadTokens: 200, contextTokens: 205, model: "m" }),
  ]);
  assert.equal(v.calls.length, 2);
  assert.equal(v.usage.inputTokens, 10);
  assert.deepEqual(v.context, { tokens: 205, window: 1000 });
});

test("buildView says when a call sent the whole context again", () => {
  const notes = (events: ReturnType<typeof typed>[]) => buildView(events).items.filter((i) => i.kind === "cache_resend");
  const call = (seq: number, read: number, write: number) => typed(seq, "usage_update", { inputTokens: 2, cacheReadTokens: read, cacheWriteTokens: write, contextTokens: 2 + read + write, model: "m" });
  const seen = notes([call(1, 0, 90_000), call(2, 90_000, 500), call(3, 0, 91_000)]);
  assert.equal(seen.length, 1);
  assert.ok(seen[0].kind === "cache_resend" && seen[0].tokens === 91_000);
  // A compaction replaces the context, so what follows it is no resend.
  const compaction = typed(2, "compaction_update", { status: "completed", reason: "auto", contextTokens: 90_000, afterTokens: 80_000, summary: "s" });
  assert.equal(notes([call(1, 90_000, 500), compaction, call(3, 0, 89_000)]).length, 0);
  // Old transcripts have no context figures, and their totals may be of whole turns.
  assert.equal(notes([typed(1, "usage_update", { inputTokens: 2, cacheWriteTokens: 90_000 }), typed(2, "usage_update", { inputTokens: 2, cacheWriteTokens: 91_000 })]).length, 0);
});

test("buildView tracks the context and records compactions", () => {
  const v = buildView([
    typed(1, "usage_update", { inputTokens: 10, outputTokens: 1, contextTokens: 850, contextWindow: 1000 }),
    typed(2, "compaction_update", { status: "started", reason: "auto", contextTokens: 850 }),
    typed(3, "compaction_update", { status: "completed", reason: "auto", contextTokens: 850, afterTokens: 40, summary: "did things" }),
  ]);
  assert.deepEqual(v.context, { tokens: 40, window: 1000 });
  const c = v.items.find((i) => i.kind === "compaction");
  assert.ok(c && c.kind === "compaction" && c.summary === "did things" && c.before === 850);
});

test("buildView ties checkpoints to user messages and dims rewound turns", () => {
  const text = (t: string) => ({ content: { type: "text", text: t } });
  const v = buildView([
    typed(1, "checkpoint", { commit: "a", historyLen: 0 }),
    typed(2, "user_message_chunk", text("first")),
    typed(3, "agent_message_chunk", text("one")),
    typed(4, "checkpoint", { commit: "b", historyLen: 2 }),
    typed(5, "user_message_chunk", text("second")),
    typed(6, "agent_message_chunk", text("two")),
    typed(7, "rewind", { checkpointSeq: 4, commit: "b", saved: "c", context: true }),
  ]);
  const users = v.items.filter((i) => i.kind === "user");
  assert.deepEqual(
    users.map((u) => (u.kind === "user" ? [u.text, u.checkpointSeq, !!u.rewound] : null)),
    [
      ["first", 1, false],
      ["second", 4, true],
    ],
  );
  assert.equal(v.items.find((i) => i.kind === "agent" && i.text === "two")?.rewound, true);
  assert.equal(v.items.find((i) => i.kind === "agent" && i.text === "one")?.rewound, undefined);
});

test("buildView records each model call with its model, per turn", () => {
  const v = buildView([
    at(1, "usage_update", { inputTokens: 10, outputTokens: 1, providerId: "p", model: "big" }),
    at(2, "usage_update", { inputTokens: 20, outputTokens: 2 }),
    at(3, "turn_ended", { stopReason: "end_turn" }),
    at(4, "model_changed", { providerId: "q", model: "small" }),
    at(5, "usage_update", { inputTokens: 30, outputTokens: 3 }),
    at(6, "turn_ended", { stopReason: "end_turn" }),
  ]);
  assert.deepEqual(
    v.calls.map((c) => `${c.providerId}/${c.model}/${c.inputTokens}`),
    ["p/big/10", "//20", "q/small/30"],
  );
  const ends = v.items.filter((i) => i.kind === "turn_end");
  assert.deepEqual(ends.map((e) => (e.kind === "turn_end" ? e.calls.length : -1)), [2, 1]);
  assert.equal(v.usage.inputTokens, 60);
});

test("buildView says when the token optimizer changes", () => {
  const v = buildView([at(1, "optimizer_changed", { optimizer: "rtk", name: "RTK" }), at(2, "optimizer_changed", { optimizer: "", name: "" })]);
  assert.deepEqual(
    v.items.map((i) => (i.kind === "notice" ? i.message : i.kind)),
    ["Token optimizer set to RTK. It applies from the next command.", "Token optimizer turned off."],
  );
});

test("buildView shows hooks that blocked or failed, not those that passed", () => {
  const v = buildView([
    at(1, "hook_ran", { event: "pre_tool_use", tool: "shell", command: "./check.sh", exitCode: 0 }),
    at(2, "hook_ran", { event: "pre_tool_use", tool: "shell", command: "./check.sh", exitCode: 2, blocked: true, stderr: "no curl\n" }),
    at(3, "hook_ran", { event: "post_tool_use", tool: "write_file", command: "fmt", exitCode: 1 }),
    at(4, "hook_ran", { event: "session_start", command: "sleep 99", exitCode: -1, timedOut: true }),
  ]);
  assert.deepEqual(
    v.items.map((i) => (i.kind === "notice" ? i.message : i.kind)),
    [
      "The pre_tool_use hook for shell blocked it: no curl",
      "The post_tool_use hook for write_file failed with exit code 1 (fmt).",
      "The session_start hook timed out (sleep 99).",
    ],
  );
});

test("buildView marks messages a rewind can no longer cut the context back to", () => {
  const v = buildView([
    at(1, "checkpoint", { commit: "a", historyLen: 0 }),
    at(2, "user_message_chunk", { content: { type: "text", text: "first" } }),
    at(3, "history_message", { index: 0, reset: true, message: {} }),
    at(4, "checkpoint", { commit: "b", historyLen: 1 }),
    at(5, "user_message_chunk", { content: { type: "text", text: "second" } }),
  ]);
  const users = v.items.filter((i) => i.kind === "user");
  assert.deepEqual(users.map((u) => u.kind === "user" && !!u.filesOnly), [true, false]);
});

test("an opened pull request shows in the conversation (SES-011)", () => {
  const v = buildView([
    { ...ev(1, { url: "https://github.com/acme/app/pull/7", title: "Add the feature", base: "main", head: "feature", draft: true }), type: "pull_request" },
  ]);
  assert.deepEqual(
    v.items.map((i) => (i.kind === "pull_request" ? [i.url, i.title, i.base, i.head, i.draft] : i.kind)),
    [["https://github.com/acme/app/pull/7", "Add the feature", "main", "feature", true]],
  );
});

test("a tool call's later title and input replace the first ones", () => {
  const v = buildView([
    { ...ev(1, { toolCallId: "t1", title: "Terminal", tool: "shell", kind: "execute", status: "pending", rawInput: {} }), type: "tool_call" },
    { ...ev(2, { toolCallId: "t1", title: "git status --short", rawInput: { command: "git status --short" } }), type: "tool_call_update" },
    { ...ev(3, { toolCallId: "t1", status: "completed" }), type: "tool_call_update" },
  ]);
  const t = v.items.find((i) => i.kind === "tool");
  assert.ok(t && t.kind === "tool");
  assert.equal(t.title, "git status --short");
  assert.deepEqual(t.input, { command: "git status --short" });
  assert.equal(t.status, "completed");
});

test("the optimizer's name stays on a tool call when its output arrives (HRN-019)", () => {
  const v = buildView([
    { ...ev(1, { toolCallId: "t1", title: "Terminal", tool: "shell", kind: "execute", status: "pending", rawInput: {} }), type: "tool_call" },
    { ...ev(2, { toolCallId: "t1", via: "RTK" }), type: "tool_call_update" },
    { ...ev(3, { toolCallId: "t1", status: "completed", content: [{ type: "content", content: { type: "text", text: "* main" } }] }), type: "tool_call_update" },
  ]);
  const t = v.items.find((i) => i.kind === "tool");
  assert.ok(t && t.kind === "tool");
  assert.equal(t.via, "RTK");
});

test("an imported message can start with its picture (SES-020)", () => {
  const v = buildView([
    { ...ev(1, { content: { type: "image", uri: "attachment:a.png", mimeType: "image/png" } }), type: "user_message_chunk" },
    { ...ev(2, { content: { type: "image", uri: "not-ours", mimeType: "image/png" } }), type: "user_message_chunk" },
  ]);
  const users = v.items.filter((i) => i.kind === "user");
  assert.equal(users.length, 1);
  assert.deepEqual(users[0].kind === "user" ? users[0].images : [], [{ name: "a.png", mimeType: "image/png" }]);
});

test("pictures the user attached join their message (SES-015)", () => {
  const view = buildView([
    { v: 1, seq: 1, ts: "2026-10-31T10:00:00Z", sid: "s", type: "user_message_chunk", data: { content: { type: "text", text: "what is this?" } } },
    { v: 1, seq: 2, ts: "2026-10-31T10:00:00Z", sid: "s", type: "user_message_chunk", data: { content: { type: "image", uri: "attachment:ab.png", mimeType: "image/png" } } },
    { v: 1, seq: 3, ts: "2026-10-31T10:00:00Z", sid: "s", type: "user_message_chunk", data: { content: { type: "image", uri: "https://example.com/x.png" } } },
  ]);
  const user = view.items.find((i) => i.kind === "user");
  assert.ok(user && user.kind === "user");
  assert.deepEqual(user.images, [{ name: "ab.png", mimeType: "image/png" }]);
});

test("the worktree setup is one card that follows its run (SES-016)", () => {
  const ev = (seq: number, data: object) => ({ v: 1, seq, ts: "2026-10-31T10:00:00Z", sid: "s", type: "worktree_setup", data });
  const v = buildView([
    ev(1, { status: "running", command: "npm install" }),
    ev(2, { status: "failed", command: "npm install", error: "exit 1", output: "npm ERR!" }),
    ev(3, { status: "skipped" }),
  ]);
  const cards = v.items.filter((i) => i.kind === "setup");
  assert.equal(cards.length, 1);
  assert.ok(cards[0].kind === "setup" && cards[0].status === "skipped" && cards[0].command === "npm install");
  assert.equal(v.setup, "skipped");
  const retried = buildView([ev(1, { status: "failed", error: "x" }), ev(2, { status: "running" }), ev(3, { status: "done", copied: [".env"] })]);
  assert.deepEqual(
    retried.items.map((i) => (i.kind === "setup" ? i.status : "")),
    ["failed", "done"],
  );
});

test("the latest plan from any harness is the to-do list (HRN-017)", () => {
  const ev = (seq: number, data: object) => ({ v: 1, seq, ts: "2026-11-01T10:00:00Z", sid: "s", type: "plan", data });
  const v = buildView([
    ev(1, { entries: [{ content: "old", status: "pending" }] }),
    ev(2, { entries: [{ content: "read", status: "completed", priority: "high" }, { content: "write", status: "in_progress" }, { bad: 1 }] }),
  ]);
  assert.deepEqual(v.plan, [
    { content: "read", status: "completed" },
    { content: "write", status: "in_progress" },
  ]);
  assert.deepEqual(buildView([]).plan, []);
});

test("commands the sandbox ran stay out of the decided list (ENV-009)", () => {
  const evs = [
    { type: "tool_call", data: { toolCallId: "a", title: "go test", status: "completed", kind: "execute", tool: "shell" } },
    { type: "permission_decided", data: { toolCallId: "a", requestId: "r1", outcome: "allowed", actor: "sandbox" } },
    { type: "tool_call", data: { toolCallId: "b", title: "npm install", status: "completed", kind: "execute", tool: "shell" } },
    { type: "permission_decided", data: { toolCallId: "b", requestId: "r2", outcome: "allowed", actor: "user" } },
  ].map((e, i) => ({ ...e, seq: i + 1, sessionId: "s", at: "" }));
  const data = panelData(buildView(evs as never));
  assert.deepEqual(data.decided.map((i) => i.toolCallId), ["b"]);
});

test("toolRuns groups finished calls and leaves out what the user must see", () => {
  const tool = (key: string, extra: Record<string, unknown> = {}) => ({ kind: "tool", key, status: "completed", tool: "shell", ...extra }) as unknown as Item;
  const agent = (key: string) => ({ kind: "agent", key, text: "x" }) as unknown as Item;
  const shape = (items: Item[]) => toolRuns(items).map((r) => r.items.map((i) => i.key).join(""));
  assert.deepEqual(shape([tool("a"), tool("b"), agent("c"), tool("d"), tool("e"), tool("f")]), ["ab", "c", "def"]);
  // Running, failed, refused, waiting and a plan each stand alone and break a run.
  assert.deepEqual(shape([tool("a"), tool("b", { status: "in_progress" }), tool("c")]), ["a", "b", "c"]);
  assert.deepEqual(shape([tool("a"), tool("b", { status: "failed" }), tool("c")]), ["a", "b", "c"]);
  assert.deepEqual(shape([tool("a"), tool("b", { permission: { requestId: "r", options: [], decided: { outcome: "denied" } } }), tool("c")]), ["a", "b", "c"]);
  assert.deepEqual(shape([tool("a"), tool("b", { permission: { requestId: "r", options: [{ optionId: "o" }] } }), tool("c")]), ["a", "b", "c"]);
  assert.deepEqual(shape([tool("a"), tool("b", { tool: "present_plan" }), tool("c")]), ["a", "b", "c"]);
  // An allowed call stays quiet.
  assert.deepEqual(shape([tool("a"), tool("b", { permission: { requestId: "r", options: [{ optionId: "o" }], decided: { outcome: "allowed" } } })]), ["ab"]);
});

const shell = (id: string, via?: string) => ({ kind: "tool", toolKind: "execute", toolCallId: id, via, content: [] }) as never;
const read = { kind: "tool", toolKind: "read", toolCallId: "r", content: [] } as never;
const set = (optimizer: string) => ({ kind: "notice", optimizer, message: "", warn: false }) as never;

test("an optimizer's count takes only shell commands from while it was on", () => {
  assert.deepEqual(optimizerCounts({ items: [shell("a"), set("rtk"), shell("b"), shell("c", "RTK"), read, set(""), shell("d")] }, false), { commands: 2, rewritten: 1 });
});

test("a session started with an optimizer counts from the start", () => {
  assert.deepEqual(optimizerCounts({ items: [shell("a", "RTK"), shell("b")] }, true), { commands: 2, rewritten: 1 });
  assert.deepEqual(optimizerCounts({ items: [shell("a"), set(""), shell("b")] }, false), { commands: 1, rewritten: 0 });
  assert.deepEqual(optimizerCounts({ items: [shell("a")] }, false), { commands: 0, rewritten: 0 });
});

test("a rewrite the content records counts too", () => {
  const item = { kind: "tool", toolKind: "execute", toolCallId: "a", content: [{ type: "terminal_output", via: "RTK" }] } as never;
  assert.deepEqual(optimizerCounts({ items: [item] }, true), { commands: 1, rewritten: 1 });
});


// SEC-014.
test("buildView lists what the policy held, named from the tool call's final title", () => {
  const v = buildView([
    at(1, "tool_call", { toolCallId: "a", tool: "shell", kind: "execute", title: "Terminal" }),
    at(2, "permission_requested", { requestId: "r1", toolCallId: "a", command: "git push", options: [] }),
    at(3, "permission_decided", { requestId: "r1", toolCallId: "a", outcome: "denied", actor: "user" }),
    at(4, "tool_call", { toolCallId: "b", tool: "read_file", kind: "read", title: "Read" }),
    at(5, "tool_call_update", { toolCallId: "b", title: "Read secrets.env" }),
    at(6, "permission_decided", { requestId: "r2", toolCallId: "b", outcome: "denied", actor: "rule", reason: "deny read (global rule)" }),
    at(7, "permission_decided", { requestId: "r3", toolCallId: "gone", outcome: "denied", actor: "protected_path", reason: "~/.ssh is protected" }),
    at(8, "mode_refused", { from: "plan", to: "bypass" }),
    // None of these held anything.
    at(9, "permission_requested", { requestId: "f1", form: { server: "s" }, options: [] }),
    at(10, "permission_decided", { requestId: "f1", outcome: "denied", actor: "user" }),
    at(11, "permission_decided", { requestId: "r4", toolCallId: "a", outcome: "allowed", actor: "user" }),
    at(12, "permission_decided", { requestId: "r5", toolCallId: "a", outcome: "cancelled", actor: "user" }),
  ]);
  assert.deepEqual(
    v.held.map((h) => [h.seq, h.kind, h.what]),
    [
      [1, "user", "git push"],
      [4, "rule", "Read secrets.env"],
      [7, "protected_path", "~/.ssh is protected"],
      [8, "mode_change", "plan to bypass"],
    ],
  );
  assert.equal(v.held[1].reason, "deny read (global rule)");
  const notice = v.items.find((i) => i.key === "8");
  assert.ok(notice?.kind === "notice" && notice.message.includes("from plan to bypass on its own"));
});

test("buildView holds nothing in a session that was never stopped", () => {
  assert.deepEqual(buildView([at(1, "turn_started", {})]).held, []);
});

test("a stuck session says why and where it could go on, until a turn starts or it continues elsewhere", () => {
  const stuck = { reason: "tool_failures", detail: "shell failed 3 times in a row", profileId: "fb", profileName: "Fallback" };
  let v = buildView([at(1, "turn_started", {}), at(2, "stuck", stuck)]);
  assert.equal(v.stuck?.profileName, "Fallback");
  assert.ok(v.items.some((i) => i.kind === "notice" && i.message.includes("shell failed 3 times")));
  v = buildView([at(1, "turn_started", {}), at(2, "stuck", stuck), at(3, "turn_started", {})]);
  assert.equal(v.stuck, null);
  v = buildView([at(1, "stuck", stuck), at(2, "handed_off", { toSessionId: "s2", toTitle: "Next", profileName: "Fallback" })]);
  assert.equal(v.stuck, null);
  assert.ok(v.items.some((i) => i.kind === "notice" && i.message === 'Continued in "Next" on Fallback.'));
});

test("a to-do list finished in an earlier turn does not come back with the next one (HRN-017)", () => {
  const plan = (seq: number, status: string) => at(seq, "plan", { entries: [{ content: "a", status }, { content: "b", status }] });
  // Finished two turns ago: a new turn does not show it.
  let v = buildView([at(1, "turn_started", {}), plan(2, "completed"), at(3, "turn_ended", { stopReason: "end_turn" }), at(4, "turn_started", {})]);
  assert.equal(v.planThisTurn, false);
  // Updated in the turn that runs: it shows, finished or not.
  v = buildView([at(1, "turn_started", {}), plan(2, "completed")]);
  assert.equal(v.planThisTurn, true);
  v = buildView([at(1, "turn_started", {}), plan(2, "pending"), at(3, "turn_ended", { stopReason: "end_turn" }), at(4, "turn_started", {})]);
  assert.equal(v.planThisTurn, false);
});


test("what the model sees is kept, and never shows in the conversation", () => {
  const ctx = (seq: number, purpose: string, system: string): TranscriptEvent => ({ ...ev(seq, { purpose, model: "m", system, tools: [{ name: "read_file", description: "Read" }] }), type: "model_context" });
  const view = buildView([ctx(1, "turn", "one"), ctx(2, "turn", "two"), ctx(3, "commit_message", "commits")]);
  assert.deepEqual(
    view.modelContext.map((c) => [c.purpose, c.system]),
    [
      ["turn", "one"],
      ["turn", "two"],
      ["commit_message", "commits"],
    ],
  );
  assert.equal(view.items.length, 0);
});

test("a workflow stage starting is an item of its own (WFL-008)", () => {
  const e = (seq: number, type: string, data: object) => ({ v: 1, seq, ts: "2026-11-08T10:00:00Z", sid: "s", type, data });
  const v = buildView([e(1, "workflow_stage", { workflow: "w", command: "prd", title: "Write the PRD", kind: "prompt" }), e(2, "workflow_stage", { workflow: "w", command: "go", kind: "prompt" })]);
  assert.deepEqual(
    v.items.map((i) => (i.kind === "stage" ? `${i.command}:${i.title}` : i.kind)),
    ["prd:Write the PRD", "go:/go"],
  );
});

// GUI-048: the app's own sentences in the conversation are written in the language in
// force; what they quote (a hook's event, a command, what it said) is shown as given.
test("the app's own words in the conversation follow the language", () => {
  setLanguage("de");
  try {
    const v = buildView([
      at(1, "optimizer_changed", { optimizer: "", name: "" }),
      at(2, "hook_ran", { event: "pre_tool_use", tool: "shell", command: "./check.sh", exitCode: 2, blocked: true, stderr: "no curl\n" }),
      at(3, "hook_ran", { event: "session_start", command: "sleep 99", exitCode: -1, timedOut: true }),
      at(4, "mode_refused", { from: "plan", to: "bypass" }),
      at(5, "effort_changed", { effort: "high" }),
      at(6, "tool_call", { toolCallId: "c" }),
      at(7, "error", {}),
      at(8, "notice", { message: "From the engine." }),
    ]);
    assert.deepEqual(
      v.items.map((i) => (i.kind === "notice" || i.kind === "error" ? i.message : i.kind === "tool" ? i.title : i.kind)),
      [
        "Token-Optimierer ausgeschaltet.",
        "Der Hook-Befehl pre_tool_use für shell hat blockiert: no curl",
        "Der Hook-Befehl session_start hat das Zeitlimit überschritten (sleep 99).",
        "Der Agent versuchte, den Modus eigenmächtig von plan auf bypass zu ändern; der Modus bleibt plan.",
        "Aufwand auf high gesetzt. Gilt ab dem nächsten Modellaufruf.",
        "Werkzeugaufruf",
        "Unbekannter Fehler",
        "From the engine.",
      ],
    );
    assert.equal(HELD_BY.user, "Manuell abgelehnt");
    assert.equal(HELD_BY.unknown, undefined);
    assert.match(hookFailedText("it timed out"), /^Rückfrage, weil ein prüfender Hook-Befehl nicht antwortete: it timed out\./);
  } finally {
    setLanguage("en");
  }
  assert.equal(HELD_BY.user, "You denied it");
});

// GUI-049: what the @ picker puts first, and what a command expects.
test("the files a session touched, the most recent first", () => {
  const e = (seq: number, type: string, data: unknown): TranscriptEvent => ({ v: 1, seq, ts: "2026-11-11T00:00:00Z", sid: "ses_1", type, data });
  const view = buildView([
    e(1, "user_message_chunk", { content: { type: "text", text: "look at @src/a.ts" } }),
    e(2, "user_message_chunk", { content: { type: "resource_link", name: "src/a.ts", uri: "file:///p/src/a.ts" } }),
    e(3, "tool_call", { toolCallId: "t1", tool: "read_file", locations: [{ path: "/p/src/b.ts" }] }),
    e(4, "tool_call", { toolCallId: "t2", tool: "edit_file", locations: [] }),
    e(5, "tool_call_update", { toolCallId: "t2", status: "completed", content: [{ type: "diff", path: "src/c.ts", newText: "x" }] }),
    e(6, "tool_call_update", { toolCallId: "t1", locations: [{ path: "/p/src/a.ts" }] }),
    e(7, "tool_call_update", { toolCallId: "unknown", locations: [{ path: "/p/ignored.ts" }] }),
  ]);
  assert.deepEqual(view.touched, ["/p/src/a.ts", "src/c.ts", "/p/src/b.ts", "src/a.ts"]);
});

test("a command keeps the hint its agent gives for its input", () => {
  const view = buildView([
    { v: 1, seq: 1, ts: "", sid: "ses_1", type: "available_commands_update", data: { availableCommands: [{ name: "review", description: "Review a PR", input: { hint: "pull request number" } }, { name: "plain", description: "" }] } },
  ]);
  assert.equal(view.commands[0].input?.hint, "pull request number");
  assert.equal(view.commands[1].input, undefined);
});

// A long session's cost (NFR-PERF-002, issues 040 and 044): what is measured prints, and
// a batch of new events folding in under 2 ms is a limit (the PRD's "numbers to meet").
import { longFixture, LONG_EVENTS, type FixtureShape } from "./longfixture.ts";

const ms = (f: () => void) => { const t = performance.now(); f(); return performance.now() - t; };

for (const shape of ["text", "tools"] as FixtureShape[]) {
  test(`timing: folding ${LONG_EVENTS} ${shape} events at once and in batches`, () => {
    const { events, lastError } = longFixture(shape);
    assert.equal(events.length, LONG_EVENTS);
    assert.ok(lastError > 0 && lastError < LONG_EVENTS * 0.7);

    // At once: the whole transcript in one fold, the way a session opens.
    let view = foldOnce(events);
    const once = ms(() => { view = foldOnce(events); });
    assert.ok(view.items.length > 0);

    // The last 2000 events through apply and the fold in batches, the way a window
    // gets them: one event at a time (a token) and fifty at a time (a busy frame).
    for (const size of [1, 50]) {
      const batches = size === 1 ? 1000 : 40;
      const start = LONG_EVENTS - batches * size;
      let tr = apply(emptyTranscript, events.slice(0, start)).transcript;
      viewOf(tr);
      const times: number[] = [];
      for (let i = 0; i < batches; i++) {
        const batch = events.slice(start + i * size, start + (i + 1) * size);
        times.push(ms(() => { tr = apply(tr, batch).transcript; viewOf(tr); }));
      }
      assert.equal(tr.lastSeq, LONG_EVENTS);
      assert.deepEqual(viewOf(tr).items, view.items);
      const mean = times.reduce((a, b) => a + b, 0) / batches;
      console.log(`fold ${shape}: at once ${once.toFixed(0)} ms (${((once * 1000) / LONG_EVENTS).toFixed(2)} us per event); a batch of ${size} at the end ${mean.toFixed(2)} ms (apply and view)`);
      // The limit is 2 ms; a loaded machine gets some room, since the timings vary by 30%.
      assert.ok(mean < 2, `a batch of ${size} took ${mean.toFixed(2)} ms on average`);
    }
  });

  test(`the ${shape} fixture folded in steps cut at random places equals the fold at once`, () => {
    const { events } = longFixture(shape, 20000);
    const once = foldOnce(events);
    let seed = 99;
    const rand = () => ((seed = (seed * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff);
    const cuts = Array.from({ length: 40 }, () => Math.floor(rand() * events.length)).sort((a, b) => a - b);
    assert.deepStrictEqual(stepped(events, cuts), once);
    // One at a time over the last stretch too, where the last turn's items still change.
    assert.deepStrictEqual(stepped(events, Array.from({ length: 300 }, (_, i) => events.length - 300 + i)), once);
  });
}

test("a tool call is found by its ID without a scan", () => {
  const e = (seq: number, type: string, data: unknown): TranscriptEvent => ({ v: 1, seq, ts: "", sid: "s", type, data });
  const calls = 30000;
  const events: TranscriptEvent[] = [];
  for (let i = 0; i < calls; i++) events.push(e(events.length + 1, "tool_call", { toolCallId: `c${i}`, tool: "read_file", status: "pending" }));
  // Updates to the earliest calls: a scan for each would cross the whole list.
  const updates = 20000;
  for (let i = 0; i < updates; i++) events.push(e(events.length + 1, "tool_call_update", { toolCallId: `c${i % 100}`, status: "completed" }));
  const took = ms(() => { foldOnce(events); });
  assert.ok(took < 400, `${updates} updates among ${calls} calls took ${took.toFixed(0)} ms`);
});

test("appending events does not copy the ones already held", () => {
  const first = apply(emptyTranscript, [ev(1), ev(2)]).transcript;
  const next = apply(first, [ev(3)]).transcript;
  assert.equal(next.events, first.events);
  assert.equal(first.length, 2);
  assert.deepEqual(viewOf(first).items.map((i) => i.key), ["1", "2"]);
  assert.deepEqual(viewOf(next).items.map((i) => i.key), ["1", "2", "3"]);
  // The shared empty transcript is never appended to, and an older transcript that
  // gets events copies before it does, so neither changes what another holds.
  assert.equal(emptyTranscript.events.length, 0);
  const other = apply(first, [ev(3, { message: "another" })]).transcript;
  assert.notEqual(other.events, next.events);
  assert.equal(next.events[2].data && (next.events[2].data as { message?: string }).message, undefined);
  assert.equal((viewOf(other).items[2] as { message: string }).message, "another");
  assert.equal((viewOf(next).items[2] as { message: string }).message, "");
});

test("an item that did not change is the same object in the next view", () => {
  const e = (seq: number, type: string, data: unknown): TranscriptEvent => ({ v: 1, seq, ts: "", sid: "s", type, data });
  const evs = [
    e(1, "user_message_chunk", { content: { type: "text", text: "go" } }),
    e(2, "tool_call", { toolCallId: "a", tool: "read_file", status: "completed" }),
    e(3, "tool_call", { toolCallId: "b", tool: "read_file", status: "completed" }),
    e(4, "agent_message_chunk", { content: { type: "text", text: "one" } }),
  ];
  let tr = apply(emptyTranscript, evs).transcript;
  const before = viewOf(tr);
  tr = apply(tr, [e(5, "agent_message_chunk", { content: { type: "text", text: " two" } })]).transcript;
  const after = viewOf(tr);
  assert.notEqual(after, before);
  assert.equal(after.items[0], before.items[0]);
  assert.equal(after.items[1], before.items[1]);
  assert.notEqual(after.items[3], before.items[3]);
  assert.equal((after.items[3] as { text: string }).text, "one two");
  assert.equal(after.runs[0], before.runs[0]);
  assert.equal(after.panel, before.panel);
  assert.equal(after.touched, before.touched);
  assert.equal(viewOf(tr), after);
});

test("a rewind, a history reset and a change of language still give the right view", () => {
  const e = (seq: number, type: string, data: unknown): TranscriptEvent => ({ v: 1, seq, ts: "", sid: "s", type, data });
  const user = (seq: number, text: string) => e(seq, "user_message_chunk", { content: { type: "text", text } });
  const evs = [
    e(1, "checkpoint", {}),
    user(2, "one"),
    e(3, "tool_call", { toolCallId: "t1", tool: "edit_file", status: "completed" }),
    e(4, "checkpoint", {}),
    user(5, "two"),
    e(6, "tool_call", { toolCallId: "t2", tool: "edit_file", status: "pending" }),
    e(7, "tool_call_update", { toolCallId: "t2", status: "completed" }),
  ];
  const users = (v: SessionView) => v.items.filter((i) => i.kind === "user").map((i) => [i.kind === "user" && i.filesOnly, !!i.rewound, i.rev]);
  // A rewind to the second checkpoint dims what came after it, in the view already built.
  let tr = apply(emptyTranscript, evs).transcript;
  const early = viewOf(tr);
  assert.deepEqual(users(early), [[undefined, false, 2], [undefined, false, 5]]);
  tr = apply(tr, [e(8, "rewind", { checkpointSeq: 4, context: true })]).transcript;
  const rewound = viewOf(tr);
  assert.deepEqual(rewound.items.map((i) => !!i.rewound), [false, false, true, true, false]);
  assert.deepEqual(rewound.rewindable.map((i) => i.key), ["2"]);
  assert.equal(early.items[3].rewound, undefined, "the earlier view is not touched");
  // The rewind replaced the context, so the first message can restore files only.
  assert.equal(rewound.items[0].kind === "user" && rewound.items[0].filesOnly, true);
  // A history reset does the same to every earlier checkpoint, and a message after it is whole.
  tr = apply(tr, [e(9, "history_message", { reset: true }), e(10, "checkpoint", {}), user(11, "three")]).transcript;
  const reset = viewOf(tr);
  assert.deepEqual(reset.items.filter((i) => i.kind === "user").map((i) => i.kind === "user" && i.filesOnly), [true, true, undefined]);
  assert.deepStrictEqual(reset, foldOnce(tr.events));
  // A checkpoint followed by a reset and then its message: the message is files only at once.
  assert.equal(buildView([e(1, "checkpoint", {}), e(2, "history_message", { reset: true }), user(3, "late")]).items[0].kind === "user" && true, true);
  assert.equal((buildView([e(1, "checkpoint", {}), e(2, "history_message", { reset: true }), user(3, "late")]).items[0] as { filesOnly?: boolean }).filesOnly, true);

  // The app's own words follow the language: the same transcript is folded again.
  const effort = apply(emptyTranscript, [e(1, "effort_changed", { effort: "high" })]).transcript;
  const english = viewOf(effort);
  setLanguage("de");
  try {
    const german = viewOf(effort);
    assert.notDeepEqual((german.items[0] as { message: string }).message, (english.items[0] as { message: string }).message);
    assert.deepStrictEqual(german, foldOnce(effort.events));
    // Events that come afterwards are folded in the language in force.
    const more = apply(effort, [e(2, "effort_changed", { effort: "low" })]).transcript;
    assert.deepStrictEqual(viewOf(more), foldOnce(more.events));
  } finally {
    setLanguage("en");
  }
  assert.deepStrictEqual(viewOf(effort), english);
});

test("an image whose uri is not a string is left out, not thrown on", () => {
  for (const uri of [42, null, { a: 1 }, ["attachment:a.png"]]) {
    const { items } = buildView([{ ...ev(1, { content: { type: "image", uri, mimeType: "image/png" } }), type: "user_message_chunk" }]);
    assert.deepEqual(items.filter((i) => i.kind === "user"), []);
  }
});
