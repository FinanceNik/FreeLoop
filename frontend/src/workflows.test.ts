import assert from "node:assert/strict";
import { test } from "node:test";
import { stageAction, stageCall, stageCommands, type WorkflowLike } from "./workflows.ts";

const w: WorkflowLike = {
  id: "plan-slice-afk",
  name: "Plan, slice, run AFK",
  stages: [
    { command: "grill", title: "Grill the brief", description: "The agent interviews you" },
    { command: "afk", title: "Run the AFK issues", kind: "afk" },
  ],
};

test("a workflow's stages are slash commands", () => {
  const cmds = stageCommands(w);
  assert.deepEqual(
    cmds.map((c) => c.name),
    ["grill", "afk"],
  );
  assert.equal(cmds[0].stage, true);
  assert.match(cmds[0].description, /Plan, slice, run AFK: The agent interviews you/);
  assert.deepEqual(stageCommands(undefined), []);
});

test("a stage's command takes what follows it", () => {
  assert.deepEqual(stageCall("/grill add dark mode", w), { command: "grill", input: "add dark mode" });
  assert.deepEqual(stageCall("  /afk  ", w), { command: "afk", input: "" });
  assert.deepEqual(stageCall("/grill line one\nline two", w), { command: "grill", input: "line one\nline two" });
  assert.equal(stageCall("/plan", w), null);
  assert.equal(stageCall("please /grill", w), null);
  assert.equal(stageCall("/grill", undefined), null);
});

test("each kind says what it does", () => {
  assert.match(stageAction("afk"), /AFK run/);
  assert.match(stageAction(undefined), /prompt/);
});
