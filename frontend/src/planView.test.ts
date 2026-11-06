import assert from "node:assert/strict";
import { test } from "node:test";
import { approveLabel, planBlocks, planChoices, planFeedback, planText, planVersions, quoteOf } from "./planView.ts";
import type { Item } from "./transcript.ts";

type ToolItem = Extract<Item, { kind: "tool" }>;
const tool = (over: Partial<ToolItem>): ToolItem =>
  ({ kind: "tool", key: "k", rev: 0, toolCallId: "t", tool: "acp:switch_mode", title: "Approve Plan", toolKind: "switch_mode", status: "pending", input: { plan: "# Plan" }, content: [], ...over }) as ToolItem;

test("a plan is found by its kind and input, for either agent", () => {
  assert.equal(planText(tool({})), "# Plan");
  assert.equal(planText(tool({ tool: "present_plan", toolKind: "switch_mode" })), "# Plan");
  assert.equal(planText(tool({ toolKind: "edit", tool: "edit_file" })), null);
  assert.equal(planText(tool({ input: { plan: "  " } })), null);
  assert.equal(planText(tool({ input: { plan: 3 } })), null);
  assert.equal(planText(tool({ input: null })), null);
});

test("the versions say how each plan went", () => {
  const options = [{ optionId: "y", name: "Yes", kind: "allow_once" }];
  const items: Item[] = [
    tool({ toolCallId: "a", permission: { requestId: "r1", options, decided: { outcome: "denied", actor: "user" } } }),
    { kind: "agent", key: "x", rev: 0, text: "hi" },
    tool({ toolCallId: "b", permission: { requestId: "r2", options, decided: { outcome: "allowed", actor: "user" } } }),
    tool({ toolCallId: "c", permission: { requestId: "r3", options } }),
  ];
  assert.deepEqual(
    planVersions(items).map((v) => [v.toolCallId, v.state]),
    [
      ["a", "sent back"],
      ["b", "approved"],
      ["c", "waiting"],
    ],
  );
});

test("blocks split at blank lines and keep code fences whole", () => {
  assert.deepEqual(planBlocks("# Title\n\nOne\ntwo\n\n\n- a\n- b"), ["# Title", "One\ntwo", "- a\n- b"]);
  assert.deepEqual(planBlocks("```\na\n\nb\n```\n\nafter"), ["```\na\n\nb\n```", "after"]);
});

test("a comment quotes the line it is about, without its marks", () => {
  assert.equal(quoteOf("## Context\nmore"), "Context");
  assert.equal(quoteOf("1. Read the code"), "Read the code");
  assert.equal(quoteOf("- [ ] item"), "[ ] item");
  assert.equal(quoteOf("x".repeat(150)).length, 100);
});

test("feedback lists the comments in the plan's order, then the general note", () => {
  const got = planFeedback(
    [
      { block: 3, quote: "Step three", note: "Skip this." },
      { block: 1, quote: "Step one", note: " Add tests first. " },
      { block: 2, quote: "Step two", note: "  " },
    ],
    "Otherwise fine.",
  );
  assert.equal(got, "> Step one\n\nAdd tests first.\n\n> Step three\n\nSkip this.\n\nOtherwise fine.");
  assert.equal(planFeedback([], ""), "");
});

test("options are sorted by kind, whatever they are called", () => {
  const { approve, back } = planChoices([
    { optionId: "a", name: "Yes, manually approve edits", kind: "allow_once" },
    { optionId: "b", name: "Keep planning", kind: "allow_always" },
    { optionId: "c", name: "Approve", kind: "reject_once" },
  ]);
  assert.deepEqual(
    approve.map((o) => o.optionId),
    ["a", "b"],
  );
  assert.deepEqual(
    back.map((o) => o.optionId),
    ["c"],
  );
});

test("an approving option is named by what follows", () => {
  assert.equal(approveLabel({ optionId: "a", name: "Yes, and use auto mode", kind: "allow_always" }), "Use auto mode");
  assert.equal(approveLabel({ optionId: "a", name: "Yes, manually approve edits", kind: "allow_once" }), "Manually approve edits");
  assert.equal(approveLabel({ optionId: "a", name: "Approve and accept edits", kind: "allow_once" }), "Accept edits");
  assert.equal(approveLabel({ optionId: "a", name: "Yes", kind: "allow_once" }), "Approve");
});
