import assert from "node:assert/strict";
import { test } from "node:test";
import { commandOutcome, defaultAllow, issueStep, runLine, stepLabel, waitingStep } from "./afk.ts";

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
