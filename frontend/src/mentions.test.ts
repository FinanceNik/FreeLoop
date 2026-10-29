import { test } from "node:test";
import assert from "node:assert/strict";
import { fuzzyScore, insertMention, mentionAt, rankFiles } from "./mentions.ts";

test("mentionAt finds the mention under the caret", () => {
  assert.deepEqual(mentionAt("fix @src/ap", 11), { start: 4, query: "src/ap" });
  assert.deepEqual(mentionAt("@", 1), { start: 0, query: "" });
  assert.deepEqual(mentionAt("a\n@x", 4), { start: 2, query: "x" });
  assert.equal(mentionAt("mail me@example.com", 19), null);
  assert.equal(mentionAt("@done and more", 14), null);
  assert.equal(mentionAt("no mention", 5), null);
});

test("insertMention replaces the query and quotes spaces", () => {
  const text = "fix @ap please";
  const m = mentionAt(text, 7)!;
  assert.deepEqual(insertMention(text, m, 7, "src/App.tsx"), { text: "fix @src/App.tsx please", caret: 16 });
  assert.deepEqual(insertMention("@my", mentionAt("@my", 3)!, 3, "my notes.md"), { text: '@"my notes.md" ', caret: 15 });
});

test("rankFiles prefers file names, word starts and short paths", () => {
  const files = ["src/App.tsx", "src/api.ts", "docs/APPENDIX.md", "internal/app/deep/path/application.go", "README.md"];
  assert.deepEqual(rankFiles("app", files, 3), ["src/App.tsx", "docs/APPENDIX.md", "internal/app/deep/path/application.go"]);
  assert.equal(rankFiles("api", files)[0], "src/api.ts");
  assert.equal(rankFiles("rdme", files)[0], "README.md");
  assert.deepEqual(rankFiles("zzz", files), []);
  assert.equal(fuzzyScore("tsx", "src/App.tsx")! > fuzzyScore("tsx", "t/s/x/long/name.go")!, true);
  assert.equal(rankFiles("", files, 2).length, 2);
});
