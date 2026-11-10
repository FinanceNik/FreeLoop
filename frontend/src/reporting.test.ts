import assert from "node:assert/strict";
import { test } from "node:test";
import { language, setLanguage } from "./i18n.ts";
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

test("the notes follow the language, with its plurals", () => {
  const before = language();
  const rows = [
    { sessionId: "a", agent: "OpenCode", unreported: 2 },
    { sessionId: "b", agent: "OpenCode", unreported: 1 },
    { sessionId: "c", agent: "Codex CLI", turnTotals: 4 },
    { sessionId: "d", agent: "Gemini CLI", turnTotals: 1 },
  ];
  setLanguage("de");
  assert.deepEqual(reportingNotes(rows, agent), [
    "OpenCode hat für 3 Runden in 2 Sitzungen keine Token gemeldet, daher bleiben sie in den Kosten unberücksichtigt.",
    "Codex CLI und Gemini CLI werden nach der Gesamtsumme jeder Runde gezählt, weil diese Agenten sie so melden, nicht pro Modellaufruf.",
  ]);
  assert.equal(heldNote(1), "Die Berechtigungsrichtlinie hat in diesem Zeitraum 1 Werkzeugaufruf aufgehalten.");
  assert.equal(heldNote(12), "Die Berechtigungsrichtlinie hat in diesem Zeitraum 12 Werkzeugaufrufe aufgehalten.");
  setLanguage("nl");
  assert.deepEqual(reportingNotes([{ sessionId: "a", agent: "OpenCode", unreported: 1 }], agent), ["OpenCode heeft voor 1 beurt in 1 sessie geen tokens gemeld, dus de kosten laten die buiten beschouwing."]);
  assert.deepEqual(reportingNotes(rows.slice(2), agent), ["Codex CLI en Gemini CLI worden geteld naar het totaal van elke beurt, omdat ze zo rapporteren, niet per modelaanroep."]);
  assert.equal(heldNote(0), "");
  assert.equal(heldNote(2), "Het toestemmingsbeleid hield in deze periode 2 tool-aanroepen tegen.");
  setLanguage(before);
});
