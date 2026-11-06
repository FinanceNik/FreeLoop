import { test } from "node:test";
import assert from "node:assert/strict";
import { agentQuestions, formAnswers, formFields, initialValues } from "./forms.ts";

const schema = {
  type: "object",
  properties: {
    env: { type: "string", enum: ["staging", "production"], enumNames: ["Staging", "Production"] },
    region: { type: "string", oneOf: [{ const: "eu", title: "Europe" }, { const: "us" }] },
    replicas: { type: "integer", title: "Replicas", default: 2 },
    ratio: { type: "number" },
    notify: { type: "boolean", default: true },
    note: { type: "string", description: "Why" },
    nested: { type: "object" },
  },
  required: ["env", "replicas"],
};

test("fields follow the schema and skip what a form cannot ask", () => {
  const fields = formFields(schema);
  assert.deepEqual(
    fields.map((f) => [f.name, f.type, f.required]),
    [
      ["env", "enum", true],
      ["region", "enum", false],
      ["replicas", "integer", true],
      ["ratio", "number", false],
      ["notify", "boolean", false],
      ["note", "string", false],
    ],
  );
  assert.deepEqual(fields[0].options, [
    { value: "staging", label: "Staging" },
    { value: "production", label: "Production" },
  ]);
  assert.deepEqual(fields[1].options, [
    { value: "eu", label: "Europe" },
    { value: "us", label: "us" },
  ]);
  assert.equal(fields[2].label, "Replicas");
  assert.deepEqual(formFields(null), []);
  assert.deepEqual(formFields({ type: "object" }), []);
});

test("defaults fill the form", () => {
  assert.deepEqual(initialValues(schema), { replicas: "2", notify: true });
});

test("answers are typed, and empty or wrong fields are named", () => {
  const fields = formFields(schema);
  const ok = formAnswers(fields, { env: "staging", replicas: "3", ratio: "0.5", notify: false, note: "  keep spaces " });
  assert.deepEqual(ok, {
    answers: { env: "staging", replicas: 3, ratio: 0.5, notify: false, note: "  keep spaces " },
    missing: [],
    invalid: [],
  });
  const bad = formAnswers(fields, { replicas: "2.5", env: "", region: "mars" });
  assert.deepEqual(bad.missing, ["env"]);
  assert.deepEqual(bad.invalid, ["region", "Replicas"]);
  assert.deepEqual(bad.answers, { notify: false });
});

test("an agent's question reads its choices, their meaning and several picks", () => {
  const schema = {
    type: "object",
    properties: {
      question_0: { type: "string", title: "Order", oneOf: [{ const: "All at once", title: "All at once", description: "Paste all first" }, { const: "One by one", title: "One by one" }] },
      question_0_custom: { type: "string", title: "Other" },
      question_1: { type: "array", title: "Checks", items: { anyOf: [{ const: "Tests", title: "Tests" }, { const: "Lint", title: "Lint" }] } },
    },
  };
  const fields = formFields(schema);
  assert.deepEqual(
    fields.map((f) => [f.name, f.type]),
    [
      ["question_0", "enum"],
      ["question_0_custom", "string"],
      ["question_1", "multi"],
    ],
  );
  assert.equal(fields[0].options![0].description, "Paste all first");
  assert.deepEqual(initialValues(schema).question_1, []);
  const { answers, missing } = formAnswers(fields, { question_0: "One by one", question_0_custom: "", question_1: ["Tests", "Nope"] });
  assert.deepEqual(answers, { question_0: "One by one", question_1: ["Tests"] });
  assert.deepEqual(missing, []);
  assert.deepEqual(formAnswers(fields, { question_1: [] }).answers, {});
});

test("the questions an agent's call carries are read for its card", () => {
  assert.deepEqual(agentQuestions({ questions: [{ question: "Which?", options: [{ label: "A" }, { label: "B" }] }] }), [{ question: "Which?", options: ["A", "B"] }]);
  assert.equal(agentQuestions({ questions: [] }), null);
  assert.equal(agentQuestions({ command: "ls" }), null);
  assert.equal(agentQuestions(null), null);
});

