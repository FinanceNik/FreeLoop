import assert from "node:assert/strict";
import { test } from "node:test";
import { fromSetNote, keptEffort, modelGroups, modelLabel, modelOffered, namedModel } from "./modelChoice.ts";

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

test("an effort stays only while the model offers it", () => {
  const levels = ["low", "medium", "high"].map((value) => ({ value }));
  assert.equal(keptEffort(levels, "low"), "low");
  assert.equal(keptEffort([], "low"), "");
  assert.equal(keptEffort(levels, "max"), "");
  assert.equal(keptEffort(levels, ""), "");
});

test("a run's model says whether it is the model set's, or changed for this run", () => {
  assert.equal(fromSetNote("small\ntiny", "small\ntiny", "Split"), "from model set Split");
  assert.equal(fromSetNote("small\ntiny", "big\nb", "Split"), "changed for this run");
  assert.equal(fromSetNote("", "big\nb", "Split"), "");
  assert.equal(fromSetNote("small\ntiny", "", ""), "");
});

test("a model is offered when listed, or named on a listed provider where naming is allowed", () => {
  const models = { choices, custom: true };
  assert.equal(modelOffered(models, ""), true);
  assert.equal(modelOffered(models, "big\nb"), true);
  assert.equal(modelOffered(models, "small\nmine"), true);
  assert.equal(modelOffered(models, "gone\nx"), false);
  assert.equal(modelOffered({ choices, custom: false }, "small\nmine"), false);
  // An external agent's profile, which lists none of a provider's models.
  assert.equal(modelOffered({ choices: [{ value: "opus", name: "Opus", group: "" }], custom: false }, "small\ntiny"), false);
});
