import { test } from "node:test";
import assert from "node:assert/strict";
import { fuzzyScore, insertMention, mentionAt, rankFiles, sessionFiles } from "./mentions.ts";

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

test("a Windows path's file name counts the same as one written with slashes", () => {
  assert.equal(fuzzyScore("app", "src\\App.tsx"), fuzzyScore("app", "src/App.tsx"));
  assert.equal(rankFiles("api", ["internal\\rpc\\server.go", "src\\api.ts"])[0], "src\\api.ts");
});

// GUI-049: the session's own files come first.
test("the session's files come first with little typed, and win a close call only", () => {
  const files = ["README.md", "src/App.tsx", "src/api.ts", "internal/engine/runtime.go", "internal/engine/apply.go"];
  const touched = ["/p/internal/engine/runtime.go", "/p/src/api.ts", "/elsewhere/x.go", "/p/gone.go"];
  const changed = [
    { path: "docs/new.md", status: "untracked" },
    { path: "src/App.tsx", status: "modified" },
    { path: "old.go", status: "deleted" },
  ];
  const s = sessionFiles(files, touched, "/p", changed);
  // A file git just saw is offered; a deleted one is not.
  assert.deepEqual(s.files, [...files, "docs/new.md"]);
  // Nothing typed: touched, most recent first, then git's changes, then the rest.
  assert.deepEqual(rankFiles("", s.files, 5, s.boost), ["internal/engine/runtime.go", "src/api.ts", "docs/new.md", "src/App.tsx", "README.md"]);
  // One character typed: the most recent of those it matches.
  assert.equal(rankFiles("s", s.files, 1, s.boost)[0], "src/api.ts");
  // A close call goes to the session's file; a clearly better match still wins.
  const close = sessionFiles(["src/preview.ts", "src/SessionView.tsx", "src/App.tsx", "src/wrapper.go"], ["src/SessionView.tsx", "src/wrapper.go"], "/p");
  assert.equal(rankFiles("view", close.files, 1)[0], "src/preview.ts");
  assert.equal(rankFiles("view", close.files, 1, close.boost)[0], "src/SessionView.tsx");
  assert.equal(rankFiles("app", close.files, 1, close.boost)[0], "src/App.tsx");
  // Paths outside the project, or no longer there, get no boost.
  assert.equal(s.boost.has("/elsewhere/x.go"), false);
  assert.equal(s.boost.has("gone.go"), false);
});

test("a session's paths on Windows match the list's forward slashes", () => {
  const s = sessionFiles(["src/api.ts"], ["C:\\p\\src\\api.ts"], "C:\\p");
  assert.ok(s.boost.has("src/api.ts"));
  // Relative ones, as a model may write them, count too.
  assert.ok(sessionFiles(["src/api.ts"], ["./src/api.ts"], "/p").boost.has("src/api.ts"));
});
