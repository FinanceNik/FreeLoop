import { test } from "node:test";
import assert from "node:assert/strict";
import { assignSlots, defaultLetters, sourceColor, sourceIndex, sourceName, validLetters } from "./source.ts";
import { setLanguage } from "./i18n.ts";

const providers = [{ id: "custom-openai-compatible", name: "b.chat" }, { id: "anthropic", name: "Anthropic" }];
const harnesses = [{ id: "builtin", name: "Built-in" }, { id: "claude-code", name: "Claude Code" }];

test("a session runs on its agent if it has one, else on its provider", () => {
  assert.equal(sourceName({ harnessId: "claude-code", providerId: "" }, providers, harnesses), "Claude Code");
  assert.equal(sourceName({ harnessId: "builtin", providerId: "anthropic" }, providers, harnesses), "Anthropic");
  // The name follows the provider a model switch moved it to.
  assert.equal(sourceName({ harnessId: "builtin", providerId: "custom-openai-compatible" }, providers, harnesses), "b.chat");
  assert.equal(sourceName({ harnessId: "builtin", providerId: "" }, providers, harnesses), "Unknown provider");
});

test("letters are initials, or the start of one word", () => {
  assert.equal(defaultLetters("Claude Code"), "CC");
  assert.equal(defaultLetters("b.chat"), "BC");
  assert.equal(defaultLetters("Custom OpenAI-compatible"), "CO");
  assert.equal(defaultLetters("Anthropic"), "AN");
  assert.equal(defaultLetters("goose"), "GO");
  assert.equal(defaultLetters("Émile Zola"), "ÉZ");
  assert.equal(defaultLetters(""), "?");
});

test("only one or two letters or digits are taken", () => {
  for (const ok of ["A", "AB", "g5", "ÉZ"]) assert.ok(validLetters(ok), ok);
  for (const bad of ["", "ABC", "<b", "a b"]) assert.ok(!validLetters(bad), bad);
});

test("each gets a slot of its own, kept whatever else is there", () => {
  const ids = ["a", "b", "c", "d", "e", "f"];
  const slots = assignSlots(ids, null);
  assert.equal(new Set(slots.values()).size, ids.length);
  for (const s of slots.values()) assert.ok(s >= 1 && s <= 8);
  // Alone, or among others that do not collide with it, an ID has the same slot.
  assert.equal(assignSlots(["claude-code"], null).get("claude-code"), assignSlots(["claude-code"], null).get("claude-code"));
  // Order does not matter.
  assert.deepEqual([...assignSlots([...ids].reverse(), null)].sort(), [...slots].sort());
});

test("more than there are slots share, and all still have one", () => {
  const ids = Array.from({ length: 12 }, (_, i) => `p${i}`);
  const slots = assignSlots(ids, null);
  assert.equal(slots.size, 12);
  assert.equal(new Set(slots.values()).size, 8);
});

test("the user's choice wins, and others step aside", () => {
  const free = assignSlots(["x", "y"], null);
  const slots = assignSlots(["x", "y"], { y: { slot: free.get("x") } });
  assert.equal(slots.get("y"), free.get("x"));
  assert.notEqual(slots.get("x"), slots.get("y"));
  // A slot out of range from a file is ignored.
  assert.ok(assignSlots(["x"], { x: { slot: 99 } }).get("x")! <= 8);
});

test("a session's mark comes from all of that", () => {
  const of = sourceIndex(providers, harnesses, [], { "claude-code": { letters: "Me" } });
  const claude = of({ harnessId: "claude-code", providerId: "" });
  assert.equal(claude.letters, "Me");
  assert.equal(claude.name, "Claude Code");
  assert.ok(claude.slot >= 1);
  const gpt = of({ harnessId: "builtin", providerId: "custom-openai-compatible" });
  assert.equal(gpt.letters, "BC");
  assert.notEqual(gpt.slot, claude.slot);
  // Letters a file got wrong fall back.
  assert.equal(sourceIndex(providers, harnesses, [], { anthropic: { letters: "<script>" } })({ harnessId: "builtin", providerId: "anthropic" }).letters, "AN");
});

test("the built-in harness itself is no source, and what is gone is neutral", () => {
  const of = sourceIndex(providers, harnesses, [{ harnessId: "builtin", providerId: "deleted" }], null);
  const gone = of({ harnessId: "builtin", providerId: "deleted" });
  assert.equal(gone.slot, 0);
  assert.equal(gone.name, "deleted");
  assert.equal(sourceColor(gone.slot), "var(--source-none)");
  assert.equal(sourceColor(3), "var(--source-3)");
});

test("until the lists arrive, a session keeps its colour and does not look deleted", () => {
  const session = { harnessId: "claude-code", providerId: "" };
  const early = sourceIndex(providers, [], [session], null)(session);
  assert.ok(early.slot >= 1);
  assert.equal(early.letters, "CC");
  assert.equal(early.slot, sourceIndex(providers, harnesses, [session], null)(session).slot);
  const other = { harnessId: "builtin", providerId: "anthropic" };
  assert.ok(sourceIndex([], harnesses, [other], null)(other).slot >= 1);
});

test("a provider no longer known is named in the language in force (GUI-048)", () => {
  setLanguage("de");
  assert.equal(sourceName({ harnessId: "builtin", providerId: "" }, providers, harnesses), "Unbekannter Anbieter");
  setLanguage("nl");
  assert.equal(sourceName({ harnessId: "builtin", providerId: "" }, providers, harnesses), "Onbekende aanbieder");
  setLanguage("en");
});
