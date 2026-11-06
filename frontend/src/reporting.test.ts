import assert from "node:assert/strict";
import { test } from "node:test";
import { heldNote, reportingNotes } from "./reporting.ts";

const agent = (r: { agent: string }) => r.agent;

test("turns an agent reported nothing for are counted, by agent", () => {
  const rows = [
    { sessionId: "a", agent: "OpenCode", unreported: 2 },
    { sessionId: "b", agent: "OpenCode", unreported: 1 },
    { sessionId: "c", agent: "Claude Code" },
  ];
  assert.deepEqual(reportingNotes(rows, agent), ["OpenCode reported no tokens for 3 turns in 2 sessions, so the cost leaves them out."]);
});

test("agents counted by their turn totals are named", () => {
  const rows = [
    { sessionId: "a", agent: "Codex CLI", turnTotals: 4 },
    { sessionId: "b", agent: "Gemini CLI", turnTotals: 1 },
    { sessionId: "c", agent: "OpenCode", unreported: 1 },
  ];
  assert.deepEqual(reportingNotes(rows, agent), [
    "OpenCode reported no tokens for 1 turn in 1 session, so the cost leaves it out.",
    "Codex CLI and Gemini CLI are counted from each turn's total, as they report, not per model call.",
  ]);
  assert.deepEqual(reportingNotes([{ sessionId: "a", agent: "Codex CLI", turnTotals: 1 }], agent), ["Codex CLI is counted from each turn's total, as it reports, not per model call."]);
});

test("the held count reads as a sentence, or not at all", () => {
  assert.equal(heldNote(0), "");
  assert.equal(heldNote(1), "The permission policy held 1 tool call in this time.");
  assert.equal(heldNote(12), "The permission policy held 12 tool calls in this time.");
});
