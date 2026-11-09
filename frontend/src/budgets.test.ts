import assert from "node:assert/strict";
import { test } from "node:test";
import { budgetLine, budgetName, worst } from "./budgets.ts";
import { setLanguage } from "./i18n.ts";

const b = { period: "day", amount: 10, profileId: "", spent: 8, unpriced: false, unreported: 0, level: "warning" };

test("a budget names its period, amount and profile", () => {
  assert.equal(budgetName(b), "Daily budget, $10.00");
  assert.equal(budgetName({ ...b, period: "month", profileId: "work" }, "Work"), "Monthly budget, $10.00 for Work");
});

test("a budget's line says what it leaves out", () => {
  assert.equal(budgetLine(b), "≈ $8.00 spent, 80%");
  assert.equal(budgetLine({ ...b, unpriced: true, unreported: 2 }), "≈ $8.00 spent, 80%, not counting models without a price or 2 turns an agent did not report");
  assert.equal(budgetLine({ ...b, spent: 12, level: "reached" }), "≈ $12.00 spent, 120%. Reached: the queue waits, running sessions go on.");
});

test("the worst level wins", () => {
  assert.equal(worst([]), "");
  assert.equal(worst([{ level: "" }, { level: "warning" }]), "warning");
  assert.equal(worst([{ level: "warning" }, { level: "reached" }]), "reached");
});

test("a budget is written in the language in force (GUI-048)", () => {
  assert.equal(budgetLine({ ...b, unreported: 1 }), "≈ $8.00 spent, 80%, not counting 1 turn an agent did not report");
  assert.equal(budgetLine({ ...b, unpriced: true }), "≈ $8.00 spent, 80%, not counting models without a price");
  setLanguage("de");
  assert.equal(budgetName(b), "Täglicher Kostenrahmen, 10,00\u00a0$");
  assert.equal(budgetName({ ...b, period: "month", profileId: "work" }, "Work"), "Monatlicher Kostenrahmen, 10,00\u00a0$ für Work");
  assert.equal(budgetLine(b), "≈ 8,00\u00a0$ ausgegeben, 80\u00a0%");
  assert.equal(budgetLine({ ...b, unreported: 1 }), "≈ 8,00\u00a0$ ausgegeben, 80\u00a0%, ausgenommen 1 Runde, die ein Agent nicht gemeldet hat");
  assert.equal(
    budgetLine({ ...b, spent: 12, level: "reached", unpriced: true, unreported: 2 }),
    "≈ 12,00\u00a0$ ausgegeben, 120\u00a0%, ausgenommen Modelle ohne Preis und 2 Runden, die ein Agent nicht gemeldet hat. Erreicht: Die Warteschlange wartet, laufende Sitzungen laufen weiter.",
  );
  setLanguage("nl");
  assert.equal(budgetName(b), "Dagbudget, $\u00a010,00");
  setLanguage("en");
});
