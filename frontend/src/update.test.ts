import { test } from "node:test";
import assert from "node:assert/strict";
import { updateView } from "./update.ts";

// SET-008: what Settings offers follows from what the engine says.
test("an update is offered only when one is known and this copy can install it", () => {
  assert.deepEqual(updateView(null), { kind: "none" });
  assert.deepEqual(updateView({ newer: false, latest: "0.1.0" }), { kind: "none" });
  assert.deepEqual(updateView({ newer: true, latest: "0.2.0", canInstall: true }), { kind: "offer", version: "0.2.0", failed: "" });
  assert.deepEqual(updateView({ newer: true, latest: "0.2.0", canInstall: false, whyNot: "installed as a package" }), {
    kind: "cannot",
    version: "0.2.0",
    reason: "installed as a package",
  });
});

test("a download shows how far it is", () => {
  assert.deepEqual(updateView({ newer: true, latest: "0.2.0", step: "downloading", received: 50, total: 200 }), { kind: "downloading", version: "0.2.0", percent: 25 });
  // Before the size is known, and never past the end.
  assert.equal((updateView({ step: "downloading" }) as { percent: number }).percent, 0);
  assert.equal((updateView({ step: "downloading", received: 300, total: 200 }) as { percent: number }).percent, 100);
});

test("a downloaded update says when restarting would interrupt work", () => {
  assert.deepEqual(updateView({ step: "ready", ready: "0.2.0", latest: "0.2.0", busy: { working: 0, waiting: 0 } }), { kind: "ready", version: "0.2.0", busy: false, failed: "" });
  assert.equal((updateView({ step: "ready", ready: "0.2.0", busy: { working: 1, waiting: 0 } }) as { busy: boolean }).busy, true);
  assert.equal((updateView({ step: "ready", ready: "0.2.0", busy: { working: 0, waiting: 2 } }) as { busy: boolean }).busy, true);
  assert.deepEqual(updateView({ step: "installing", ready: "", latest: "0.2.0" }), { kind: "installing", version: "0.2.0" });
});

test("a failure is shown with the offer it leaves", () => {
  assert.deepEqual(updateView({ newer: true, latest: "0.2.0", canInstall: true, installError: "not signed by the Freeloop project" }), {
    kind: "offer",
    version: "0.2.0",
    failed: "not signed by the Freeloop project",
  });
  assert.equal((updateView({ step: "ready", ready: "0.2.0", installError: "the folder cannot be written" }) as { failed: string }).failed, "the folder cannot be written");
});
