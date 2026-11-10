import assert from "node:assert/strict";
import { test } from "node:test";
import { modelGroups, modelLabel, namedModel } from "./modelChoice.ts";

const choices = [
  { value: "big\nb", name: "b", group: "Big" },
  { value: "big\nscripted", name: "", group: "Big" },
  { value: "small\ntiny", name: "tiny", group: "Small" },
];

test("models are grouped by provider in the order given, each with its provider", () => {
  const groups = modelGroups(choices);
  assert.deepEqual(
    groups.map((g) => [g.group, g.provider, g.items.map((i) => i.name)]),
    [
      ["Big", "big", ["b", "scripted"]],
      ["Small", "small", ["tiny"]],
    ],
  );
});

test("a model named by hand goes to its provider, and blank or multi-line names do not", () => {
  assert.equal(namedModel("small", "  qwen-3.8-flash "), "small\nqwen-3.8-flash");
  assert.equal(namedModel("small", "   "), "");
  assert.equal(namedModel("small", "a\nb"), "");
});

test("a value reads as its model, with its provider's name when it is not among the choices", () => {
  assert.equal(modelLabel(choices, "big\nb"), "b");
  assert.equal(modelLabel(choices, "small\nmine"), "mine (Small)");
  assert.equal(modelLabel(choices, "gone\nx"), "x (gone)");
  assert.equal(modelLabel(choices, ""), "");
});
