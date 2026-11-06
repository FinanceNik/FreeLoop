import assert from "node:assert/strict";
import { test } from "node:test";
import { budgetLine, budgetName, worst } from "./budgets.ts";

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
