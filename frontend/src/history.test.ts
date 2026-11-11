import { beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import {
  historyEntries,
  MAX_ENTRIES,
  nextWord,
  onFirstLine,
  onLastLine,
  pruneHistory,
  recallList,
  rememberPrompt,
  rightToLeft,
  searchPrompts,
  subscribeHistory,
  suggestion,
  useHistoryStorage,
  type PastPrompt,
} from "./history.ts";

function fakeStorage() {
  const m = new Map<string, string>();
  return { m, getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
}
const same = async (s: string) => s;
let disk = fakeStorage();
beforeEach(() => {
  disk = fakeStorage();
  useHistoryStorage(disk);
});

const entry = (text: string, projectRoot = "/p", sessionId = "s1", at = 0): PastPrompt => ({ text, sessionId, projectRoot, at });

test("a sent prompt is kept newest first, redacted, and the same text again moves to the top", async () => {
  await rememberPrompt({ text: "run the tests", sessionId: "s1", projectRoot: "/p" }, same, 1);
  await rememberPrompt({ text: "  key sk-secret  ", sessionId: "s1", projectRoot: "/p" }, async (s) => s.replace("sk-secret", "[REDACTED]"), 2);
  await rememberPrompt({ text: "run the tests", sessionId: "s2", projectRoot: "/p" }, same, 3);
  assert.deepEqual(historyEntries().map((x) => [x.text, x.sessionId, x.at]), [
    ["run the tests", "s2", 3],
    ["key [REDACTED]", "s1", 2],
  ]);
  assert.ok(!disk.m.get("freeloop.history")!.includes("sk-secret"));
  // In another project it is its own entry.
  await rememberPrompt({ text: "run the tests", sessionId: "s3", projectRoot: "/q" }, same, 4);
  assert.equal(historyEntries().length, 3);
});

test("if redacting fails, or the prompt is empty or too long, nothing is recorded", async () => {
  await rememberPrompt({ text: "key sk-secret", sessionId: "s1", projectRoot: "/p" }, async () => {
    throw new Error("no engine");
  });
  await rememberPrompt({ text: "   ", sessionId: "s1", projectRoot: "/p" }, same);
  await rememberPrompt({ text: "x".repeat(5000), sessionId: "s1", projectRoot: "/p" }, same);
  assert.deepEqual(historyEntries(), []);
  assert.equal(disk.m.size, 0);
});

test("history keeps the newest 500 and comes back from storage", async () => {
  for (let i = 0; i < MAX_ENTRIES + 3; i++) await rememberPrompt({ text: `prompt ${i}`, sessionId: "s1", projectRoot: "/p" }, same, i);
  assert.equal(historyEntries().length, MAX_ENTRIES);
  assert.equal(historyEntries()[0].text, `prompt ${MAX_ENTRIES + 2}`);
  useHistoryStorage(disk);
  assert.equal(historyEntries().length, MAX_ENTRIES);
});

test("damaged storage reads as no history", () => {
  disk.setItem("freeloop.history", "{not json");
  assert.deepEqual(historyEntries(), []);
  useHistoryStorage(disk);
  disk.setItem("freeloop.history", JSON.stringify([{ text: 1 }, entry("kept")]));
  assert.deepEqual(historyEntries().map((x) => x.text), ["kept"]);
});

test("what was typed in a deleted session goes with it, and listeners hear of it", async () => {
  await rememberPrompt({ text: "from a kept session", sessionId: "kept", projectRoot: "/p" }, same);
  await rememberPrompt({ text: "from a deleted one", sessionId: "gone", projectRoot: "/p" }, same);
  let heard = 0;
  const stop = subscribeHistory(() => heard++);
  pruneHistory((sid) => sid === "kept");
  stop();
  assert.deepEqual(historyEntries().map((x) => x.text), ["from a kept session"]);
  assert.equal(heard, 1);
  // Nothing to remove changes nothing.
  const before = historyEntries();
  pruneHistory(() => true);
  assert.equal(historyEntries(), before);
});

test("a suggestion is the rest of this project's newest prompt that starts with the draft", () => {
  const list = [entry("run the tests again", "/p"), entry("run the linter", "/p"), entry("run elsewhere", "/q")];
  assert.equal(suggestion("run", list, "/p"), " the tests again");
  assert.equal(suggestion("run the l", list, "/p"), "inter");
  assert.equal(suggestion("run e", list, "/p"), "");
  assert.equal(suggestion("run e", list, "/q"), "lsewhere");
  // Case matters: what is accepted is what was typed before.
  assert.equal(suggestion("Run", list, "/p"), "");
  // Too short, the whole of one already, or more than one line: none.
  assert.equal(suggestion("r", list, "/p"), "");
  assert.equal(suggestion(" r ", list, "/p"), "");
  assert.equal(suggestion("run the linter", list, "/p"), "");
  assert.equal(suggestion("run\nthe", [entry("run\nthe tests")], "/p"), "");
});

test("one word of a suggestion is the next word with the spaces before it", () => {
  assert.equal(nextWord(" the tests"), " the");
  assert.equal(nextWord("ning fast"), "ning");
  assert.equal(nextWord("  "), "  ");
});

test("↑ steps through this project's prompts, or those starting with what is typed", () => {
  const list = [entry("fix the build", "/p"), entry("elsewhere", "/q"), entry("fix the docs", "/p"), entry("fix the build", "/p"), entry("explain", "/p")];
  assert.deepEqual(recallList(list, "/p", ""), ["fix the build", "fix the docs", "explain"]);
  assert.deepEqual(recallList(list, "/p", "fix"), ["fix the build", "fix the docs"]);
  assert.deepEqual(recallList(list, "/p", "explain"), []);
});

test("a recalled prompt of several lines keeps ↑ and ↓ for its lines first", () => {
  const text = "one\ntwo\nthree";
  assert.ok(onFirstLine(text, 2));
  assert.ok(!onFirstLine(text, 5));
  assert.ok(onLastLine(text, text.length));
  assert.ok(!onLastLine(text, 5));
  assert.ok(onFirstLine("", 0) && onLastLine("", 0));
});

test("the search needs every word, whatever the case, and puts this project first", () => {
  const list = [entry("Fix the BUILD on linux", "/q"), entry("fix the build", "/p"), entry("fix the docs", "/p")];
  assert.deepEqual(searchPrompts("build fix", list, "/p").map((x) => x.text), ["fix the build", "Fix the BUILD on linux"]);
  assert.deepEqual(searchPrompts("", list, "/p").map((x) => x.text), ["fix the build", "fix the docs", "Fix the BUILD on linux"]);
  assert.deepEqual(searchPrompts("nothing", list, "/p"), []);
  assert.equal(searchPrompts("fix", list, "/p", 2).length, 2);
});

test("right-to-left text is told apart", () => {
  assert.ok(rightToLeft("שלום"));
  assert.ok(rightToLeft("مرحبا"));
  assert.ok(!rightToLeft("hello wörld"));
});
