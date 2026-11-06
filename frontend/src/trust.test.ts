import { test } from "node:test";
import assert from "node:assert/strict";
import { trustLines, trustStep } from "./trust.ts";

test("a project whose hooks are unknown is not trusted yet", () => {
  assert.equal(trustStep(null, true), "lookup");
  assert.equal(trustStep(null, false), "lookup");
});

test("hooks not on screen are confirmed, and no hooks need no question", () => {
  assert.equal(trustStep([{ command: "x" }], true), "confirm");
  assert.equal(trustStep([], true), "trust");
});

test("hooks the banner already lists need no second question", () => {
  assert.equal(trustStep([{ command: "x" }], false), "trust");
});

test("trusting names every hook it would let run, and agent settings that try to loosen", () => {
  const lines = trustLines({
    hooks: [{ event: "session_start", command: "make setup" }],
    agentConfig: [
      { path: ".claude/settings.json", hooks: ["SessionStart"], allows: true },
      { path: ".codex/config.toml", allows: true },
      { path: "opencode.json" },
    ],
  });
  assert.deepEqual(lines, [
    "Hook, session_start: make setup",
    "Claude Code hooks in .claude/settings.json: SessionStart",
    ".claude/settings.json tells its agent not to ask. Freeloop's policy still decides.",
    ".codex/config.toml tells its agent not to ask. Freeloop's policy still decides.",
  ]);
  assert.deepEqual(trustLines({}), []);
});

