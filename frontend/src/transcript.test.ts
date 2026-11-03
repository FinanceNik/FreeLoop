import { test } from "node:test";
import assert from "node:assert/strict";
import { apply, awaitingDecision, buildView, emptyTranscript, lineDiff, panelData, summary, type TranscriptEvent } from "./transcript.ts";

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

test("buildView tracks the context and records compactions", () => {
  const typed = (seq: number, type: string, data: unknown): TranscriptEvent => ({ ...ev(seq, data), type });
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
  const typed = (seq: number, type: string, data: unknown): TranscriptEvent => ({ ...ev(seq, data), type });
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
