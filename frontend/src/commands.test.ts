import { test } from "node:test";
import assert from "node:assert/strict";
import { matchCommands, MODE_COMMANDS, modeCommand, slashQuery } from "./commands.ts";

test("slashQuery is the first word after a leading slash", () => {
  assert.equal(slashQuery("/", 1), "");
  assert.equal(slashQuery("/rev", 4), "rev");
  assert.equal(slashQuery("/rev", 2), "r");
  assert.equal(slashQuery("/review now", 11), null);
  assert.equal(slashQuery("say /rev", 8), null);
  assert.equal(slashQuery("", 0), null);
});

test("matchCommands puts prefix matches first", () => {
  const list = [...MODE_COMMANDS, { name: "review", description: "" }, { name: "preview", description: "" }];
  assert.deepEqual(
    matchCommands("pl", list).map((c) => c.name),
    ["plan"],
  );
  assert.deepEqual(
    matchCommands("rev", list).map((c) => c.name),
    ["review", "preview"],
  );
  assert.equal(matchCommands("", list).length, list.length);
});

test("modeCommand recognises a bare mode command only", () => {
  assert.equal(modeCommand("/plan"), "plan");
  assert.equal(modeCommand(" /accept-edits "), "accept_edits");
  assert.equal(modeCommand("/plan the work"), null);
  assert.equal(modeCommand("/review"), null);
  assert.equal(modeCommand("plan"), null);
});
