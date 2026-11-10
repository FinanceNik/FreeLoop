import assert from "node:assert/strict";
import { test } from "node:test";
import { commandFrom, commandOutcome, costAmount, defaultAllow, issueCostLine, issueStep, runCostLine, runLine, stepLabel, waitingStep } from "./afk.ts";

test("a run offers to allow what would ask, never what is denied", () => {
  const cmds = [
    { command: "git commit *", from: "workflow", outcome: "ask" },
    { command: "git push *", from: "workflow", outcome: "denied" },
    { command: "go test *", from: "project", outcome: "allowed" },
    { command: "make test", from: "project", outcome: "ask" },
  ];
  assert.deepEqual(defaultAllow(cmds), ["git commit *", "make test"]);
  assert.equal(commandOutcome(cmds[1]), "denied by your policy");
  assert.equal(commandOutcome(cmds[2]), "already allowed");
});

test("a run sums itself up in a line", () => {
  const steps = [
    { issue: "001-store", title: "Store", state: "qa" },
    { issue: "002-search", title: "Search", state: "reviewing" },
  ];
  assert.equal(runLine({ state: "running", steps }), "AFK run going on: 1 of 2 issues done · 002-search in review");
  assert.equal(runLine({ state: "finished", steps: [steps[0]], reason: "Every AFK issue is done." }), "AFK run finished: 1 of 1 issue done · Every AFK issue is done.");
  // WFL-014: where its issues went.
  assert.equal(runLine({ state: "finished", steps: [steps[0]], modelLabel: "small (Demo small)", effort: "low" }), "AFK run finished: 1 of 1 issue done · Issues on small (Demo small) · low");
  assert.equal(stepLabel({ issue: "x", title: "x", state: "qa_done" }), "QA'd");
});

test("a run says why its issue has not started", () => {
  const waiting = "Claude Code's weekly limit is at 96 % until Tue 06:00";
  const run = { state: "running", steps: [{ issue: "001-store", title: "Store", state: "working", waiting }] };
  assert.equal(runLine(run), `AFK run going on: 0 of 1 issue done · 001-store waiting to start: ${waiting}`);
  assert.equal(waitingStep(run)?.issue, "001-store");
  assert.equal(issueStep(run, "001-store")?.state, "working");
  assert.equal(issueStep(run, "002-search"), undefined);
  // Once it has started, or the run has ended, nothing waits.
  assert.equal(waitingStep({ ...run, steps: [{ ...run.steps[0], waiting: "" }] }), undefined);
  assert.equal(waitingStep({ ...run, state: "stopped" }), undefined);
});

test("the project's other check targets are offered unticked (WFL-009)", () => {
  const cmds = [
    { command: "make test", from: "project", outcome: "ask" },
    { command: "make check-app", from: "project-more", outcome: "ask" },
  ];
  assert.deepEqual(defaultAllow(cmds), ["make test"]);
  assert.equal(commandFrom("project-more"), "another of the project's checks");
});

// WFL-015: what a run and an issue cost, by part and model, unpriced usage said.
test("a run's cost in one line, and an issue's", () => {
  const part = (cost: number, unpriced: boolean, ...models: string[]) => ({ cost, unpriced, models: models.map((model) => ({ providerId: "p", model, cost, priced: !unpriced })) });
  assert.equal(costAmount(part(1.2, false)), "$1.20");
  assert.equal(costAmount(part(0.5, true)), "$0.50 + unpriced");
  assert.equal(costAmount(part(0, true)), "unpriced");
  const rc = { planning: part(1.2, false, "opus"), issues: part(0.08, false, "qwen"), reviews: part(0, true, "b"), total: 1.28, unpriced: true };
  assert.equal(runCostLine(rc), "Planning $1.20 on opus · Issues $0.08 on qwen · Reviews unpriced on b · total $1.28 + unpriced");
  assert.equal(runCostLine({ ...rc, planning: part(0, false) }), "Issues $0.08 on qwen · Reviews unpriced on b · total $1.28 + unpriced");
  assert.equal(runCostLine({ planning: part(0, false), issues: part(0, false), reviews: part(0, false), total: 0, unpriced: false }), "");
  assert.equal(issueCostLine({ ...part(0.3, false, "qwen", "b"), commit: "0123456789abcdef" }), "Cost $0.30 on qwen, b · commit 0123456");
});
