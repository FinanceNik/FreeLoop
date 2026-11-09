import assert from "node:assert/strict";
import { test } from "node:test";
import { guided, stageAction, stageCall, stageCommands, stageOutputs, stageProgress, withoutStageSkills, type WorkflowLike } from "./workflows.ts";
import { setLanguage } from "./i18n.ts";

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

const flow: WorkflowLike = {
  id: "plan",
  name: "Plan",
  stages: [
    { command: "grill", title: "Grill", skill: "grill-the-brief", needsInput: true },
    { command: "prd", title: "PRD", skill: "write-a-prd" },
    { command: "slice", title: "Slice", skill: "slice-into-issues" },
    { command: "afk", title: "AFK", kind: "afk" },
    { command: "review", title: "Review", kind: "review" },
  ],
};
const states = (steps: { stage: { command: string }; state: string }[]) => steps.map((s) => `${s.stage.command}:${s.state}`).join(" ");

test("the stage strip follows the path, not a review", () => {
  assert.deepEqual(guided(flow).map((s) => s.command), ["grill", "prd", "slice", "afk"]);
  // Nothing ran: the first stage is next.
  assert.equal(states(stageProgress(flow, [], false)), "grill:next prd:todo slice:todo afk:todo");
  // The grill runs, then ends: the PRD is next.
  const grilling = [{ kind: "stage", command: "grill" }, { kind: "user" }];
  assert.equal(states(stageProgress(flow, grilling, true)), "grill:running prd:todo slice:todo afk:todo");
  const grilled = [...grilling, { kind: "turn_end" }];
  assert.equal(states(stageProgress(flow, grilled, false)), "grill:done prd:next slice:todo afk:todo");
  // A later turn without a stage leaves the PRD next.
  assert.equal(states(stageProgress(flow, [...grilled, { kind: "user" }], true)), "grill:done prd:next slice:todo afk:todo");
  // Skipping ahead: after slicing, the AFK run is next, and the PRD stays to do.
  assert.equal(states(stageProgress(flow, [{ kind: "stage", command: "slice" }, { kind: "turn_end" }], false)), "grill:todo prd:todo slice:done afk:next");
});

test("the AFK stage follows the project's run", () => {
  const sliced = [{ kind: "stage", command: "slice" }, { kind: "turn_end" }];
  const run = { state: "running", steps: [{ state: "qa" }, { state: "working" }] };
  const steps = stageProgress(flow, sliced, false, run);
  assert.equal(states(steps), "grill:todo prd:todo slice:done afk:running");
  assert.equal(steps[3].detail, "1 of 2 done");
  assert.equal(states(stageProgress(flow, sliced, false, { ...run, state: "finished" })), "grill:todo prd:todo slice:done afk:done");
});

test("the handoff lists what the last stage wrote", () => {
  const items = [
    { kind: "stage", command: "prd" },
    { kind: "tool", toolKind: "edit", status: "completed", content: [{ path: "backlog/prds/old.md" }] },
    { kind: "stage", command: "slice" },
    { kind: "tool", toolKind: "read", status: "completed", content: [{ path: "backlog/prds/x.md" }] },
    { kind: "tool", toolKind: "edit", status: "completed", content: [{ path: "backlog/issues/001.md" }] },
    { kind: "tool", toolKind: "edit", status: "failed", content: [{ path: "backlog/issues/bad.md" }] },
    { kind: "tool", toolKind: "edit", status: "completed", input: { file_path: "backlog/issues/002.md" } },
    { kind: "tool", toolKind: "edit", status: "completed", content: [{ path: "backlog/issues/001.md" }] },
  ];
  assert.deepEqual(stageOutputs(items), ["backlog/issues/001.md", "backlog/issues/002.md"]);
  assert.deepEqual(stageOutputs([{ kind: "user" }]), []);
});

test("the menu offers the stage, not the agent's own command for its skill", () => {
  const cmds = [{ name: "freeloop:grill-the-brief" }, { name: "write-a-prd" }, { name: "compact" }, { name: "work-an-issue" }];
  assert.deepEqual(withoutStageSkills(cmds, flow).map((c) => c.name), ["compact", "work-an-issue"]);
  assert.equal(withoutStageSkills(cmds, undefined).length, 4);
});

// GUI-048: what a kind of stage does and how far a run got are the app's words, in the
// language in force; the workflow's names and descriptions are shown as given.
test("the app's words about stages follow the language", () => {
  setLanguage("de");
  try {
    assert.equal(stageAction("afk"), "Startet einen AFK-Lauf");
    assert.equal(stageAction(undefined), "Sendet seinen Prompt hierher");
    const run = { state: "running", steps: [{ state: "qa" }, { state: "working" }] };
    assert.equal(stageProgress(flow, [{ kind: "stage", command: "slice" }, { kind: "turn_end" }], false, run)[3].detail, "1 von 2 erledigt");
    assert.equal(stageCommands(w)[0].description, "Plan, slice, run AFK: The agent interviews you");
  } finally {
    setLanguage("en");
  }
});
