import { test } from "node:test";
import assert from "node:assert/strict";
import { commandHint, matchCommands, modeCommands, modeCommand, modeHint, modeName, MODES, slashQuery } from "./commands.ts";
import { setLanguage } from "./i18n.ts";

test("slashQuery is the first word after a leading slash", () => {
  assert.equal(slashQuery("/", 1), "");
  assert.equal(slashQuery("/rev", 4), "rev");
  assert.equal(slashQuery("/rev", 2), "r");
  assert.equal(slashQuery("/review now", 11), null);
  assert.equal(slashQuery("say /rev", 8), null);
  assert.equal(slashQuery("", 0), null);
});

test("matchCommands puts prefix matches first", () => {
  const list = [...modeCommands(), { name: "review", description: "" }, { name: "preview", description: "" }];
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

// GUI-048: the modes and their commands are the app's own, so they are named and
// described in the language in force; a command's name is typed, so it stays.
test("the app's modes and mode commands follow the language", () => {
  setLanguage("de");
  assert.equal(modeName("plan"), "Planen");
  assert.equal(modeName("accept_edits"), "Änderungen annehmen");
  assert.match(modeHint("ask") ?? "", /fragt/i);
  const plan = modeCommands().find((c) => c.mode === "plan");
  assert.equal(plan?.name, "plan");
  assert.equal(plan?.description, "Nur lesen und planen; keine Änderungen");
  setLanguage("nl");
  assert.equal(modeName("ask"), "Vragen");
  setLanguage("en");
  assert.deepEqual(
    MODES.map((m) => modeName(m.id)),
    ["Ask", "Accept edits", "Plan", "Auto", "Bypass"],
  );
  assert.equal(modeCommands().find((c) => c.mode === "bypass")?.description, "Run tool calls without asking");
  // A mode the app does not know, such as an agent's own, is shown as given.
  assert.equal(modeName("yolo"), "yolo");
  assert.equal(modeHint("yolo"), undefined);
});

// GUI-049: where a command's input goes, the box says what it expects.
test("a command's hint shows once its name and a space are typed, until more is", () => {
  const list = [{ name: "review", description: "", hint: "pull request number" }, { name: "plain", description: "" }];
  assert.equal(commandHint("/review ", list), "pull request number");
  assert.equal(commandHint("/review", list), "");
  assert.equal(commandHint("/review 12", list), "");
  assert.equal(commandHint("/plain ", list), "");
  assert.equal(commandHint("/other ", list), "");
  assert.equal(commandHint("say /review ", list), "");
});
