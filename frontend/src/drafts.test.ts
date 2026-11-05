import { beforeEach, test } from "node:test";
import assert from "node:assert/strict";
import { draftOf, extrasOf, forgetDraft, pruneDrafts, rememberDraft, rememberExtras, storeDraft, useDraftStorage } from "./drafts.ts";

function fakeStorage() {
  const m = new Map<string, string>();
  return {
    m,
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
    removeItem: (k: string) => void m.delete(k),
    key: (i: number) => [...m.keys()][i] ?? null,
    get length() {
      return m.size;
    },
  };
}
const same = async (s: string) => s;
let disk = fakeStorage();
// Each test has sessions of its own, since what is in memory outlives a view on purpose.
let n = 0;
let sid = "";
beforeEach(() => {
  disk = fakeStorage();
  useDraftStorage(disk);
  sid = `ses_${++n}`;
});

test("a draft outlives its view and belongs to its session", () => {
  rememberDraft(sid, "half a thought");
  assert.equal(draftOf(sid), "half a thought");
  assert.equal(draftOf(`${sid}_other`), "");
});

test("a stored draft comes back after a restart", async () => {
  rememberDraft(sid, "half a thought");
  await storeDraft(sid, "half a thought", same);
  assert.equal(disk.getItem(`freeloop.draft.${sid}`), "half a thought");
});

test("what is stored is redacted, while what is typed stays as typed", async () => {
  rememberDraft(sid, "key sk-secret");
  await storeDraft(sid, "key sk-secret", async (s) => s.replace("sk-secret", "[REDACTED]"));
  assert.equal(disk.getItem(`freeloop.draft.${sid}`), "key [REDACTED]");
  assert.equal(draftOf(sid), "key sk-secret");
  assert.ok(![...disk.m.values()].join().includes("sk-secret"));
});

test("if redacting fails, nothing is written", async () => {
  rememberDraft(sid, "key sk-secret");
  await storeDraft(sid, "key sk-secret", async () => {
    throw new Error("no engine");
  });
  assert.equal(disk.m.size, 0);
});

test("a draft changed while redacting is not overwritten by the older text", async () => {
  rememberDraft(sid, "older");
  const storing = storeDraft(sid, "older", same);
  rememberDraft(sid, "");
  await storing;
  assert.equal(disk.m.size, 0);
});

test("sending or deleting the text removes it everywhere", async () => {
  rememberDraft(sid, "gone soon");
  await storeDraft(sid, "gone soon", same);
  rememberExtras(sid, ["picture"]);
  forgetDraft(sid);
  assert.equal(draftOf(sid), "");
  assert.equal(extrasOf(sid), undefined);
  assert.equal(disk.m.size, 0);
  // Emptied by hand, the stored copy goes too, and no older one comes back.
  rememberDraft(sid, "again");
  await storeDraft(sid, "again", same);
  rememberDraft(sid, "");
  assert.equal(draftOf(sid), "");
  assert.equal(disk.m.size, 0);
});

test("a very long draft stays in memory only", async () => {
  const long = "x".repeat(70 * 1024);
  rememberDraft(sid, long);
  await storeDraft(sid, long, same);
  assert.equal(draftOf(sid), long);
  assert.equal(disk.m.size, 0);
});

test("drafts of deleted sessions are removed, and nothing else", () => {
  disk.setItem("freeloop.draft.kept", "a");
  disk.setItem("freeloop.draft.gone", "b");
  disk.setItem("freeloop.other", "c");
  pruneDrafts((id) => id === "kept");
  assert.deepEqual([...disk.m.keys()].sort(), ["freeloop.draft.kept", "freeloop.other"]);
});

test("without storage it still works in memory", async () => {
  useDraftStorage({
    getItem() {
      throw new Error("blocked");
    },
    setItem() {
      throw new Error("blocked");
    },
    removeItem() {
      throw new Error("blocked");
    },
    key: () => null,
    length: 0,
  });
  rememberDraft(sid, "kept");
  await storeDraft(sid, "kept", same);
  assert.equal(draftOf(sid), "kept");
  rememberDraft(sid, "");
  assert.equal(draftOf(sid), "");
});
