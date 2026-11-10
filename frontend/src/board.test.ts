import { test } from "node:test";
import assert from "node:assert/strict";
import { cardMeta, LANES, laneTitle, placeIssues, shortId, since, waitingIssues } from "./board.ts";
import type { BacklogItem } from "./backlog.ts";
import { setLanguage } from "./i18n.ts";

const item = (over: Partial<BacklogItem>): BacklogItem => ({ id: "001-first", title: "First", path: "backlog/issues/001-first.md", state: "ready", type: "afk", waitsOn: [], blocks: [], ...over });
const ids = (cards: { id: string }[]) => cards.map((c) => c.id);

// GUI-016, decision 3 and 5: the Board is a scrum board of workflow issues, four columns.
test("an issue goes to the column its file and the run's step for it say", () => {
  const items = [
    item({ id: "001-ready" }),
    item({ id: "002-blocked", state: "blocked", waitsOn: ["001-ready"] }),
    item({ id: "003-human", type: "human" }),
    item({ id: "004-unreadable", type: "human", problem: "no frontmatter" }),
    item({ id: "005-working" }),
    item({ id: "006-reviewing", state: "done" }),
    item({ id: "007-qa", state: "done" }),
    item({ id: "008-qad", state: "done" }),
    item({ id: "009-done", state: "done" }),
    item({ id: "010-stopped" }),
  ];
  const run = {
    state: "running",
    steps: [
      { issue: "005-working", title: "", state: "working", startedAt: "2026-11-10T10:00:00Z" },
      { issue: "006-reviewing", title: "", state: "reviewing" },
      { issue: "007-qa", title: "", state: "qa" },
      { issue: "008-qad", title: "", state: "qa_done" },
      { issue: "010-stopped", title: "", state: "failed" },
      { issue: "099-gone", title: "", state: "working" },
    ],
  };
  const cols = placeIssues(items, run);
  assert.deepEqual(ids(cols.backlog), ["001-ready", "002-blocked", "003-human", "004-unreadable", "010-stopped"]);
  assert.deepEqual(ids(cols.working), ["005-working"]);
  assert.deepEqual(ids(cols.review), ["006-reviewing", "007-qa"]);
  assert.deepEqual(ids(cols.done), ["008-qad", "009-done"]);
});

test("without a run, the files say it all; an issue the run is working on stays in Working", () => {
  const cols = placeIssues([item({ id: "001-a" }), item({ id: "002-b", state: "done" })], null);
  assert.deepEqual([ids(cols.backlog), ids(cols.working), ids(cols.review), ids(cols.done)], [["001-a"], [], [], ["002-b"]]);
  // The session marked it done, and the run has not moved on yet: still being worked.
  const marked = placeIssues([item({ id: "001-a", state: "done" })], { state: "running", steps: [{ issue: "001-a", title: "", state: "working" }] });
  assert.deepEqual([ids(marked.working), ids(marked.done)], [["001-a"], []]);
  // A run's earlier step for an issue is replaced by its latest.
  const retried = placeIssues([item({ id: "001-a" })], {
    state: "running",
    steps: [
      { issue: "001-a", title: "", state: "failed" },
      { issue: "001-a", title: "", state: "working" },
    ],
  });
  assert.deepEqual(ids(retried.working), ["001-a"]);
  assert.deepEqual(placeIssues([], null), { backlog: [], working: [], review: [], done: [] });
  assert.deepEqual(placeIssues(null, undefined), { backlog: [], working: [], review: [], done: [] });
});

// GUI-016, decision 4: waiting is a mark, not a column. The card stays where the run has
// it, and says how many requests its session has waiting for the user.
test("an issue whose agent waits for the user is marked, and stays in its column", () => {
  const items = [item({ id: "001-a" }), item({ id: "002-b" }), item({ id: "003-c" })];
  const run = {
    state: "running",
    steps: [
      { issue: "001-a", title: "", state: "working", sessionId: "s1" },
      { issue: "002-b", title: "", state: "working", sessionId: "s2" },
      { issue: "003-c", title: "", state: "reviewing", sessionId: "s3" },
    ],
  };
  const cols = placeIssues(items, run, { s1: 2, s3: 1, other: 4 });
  assert.deepEqual(cols.working.map((c) => [c.id, c.waiting]), [["001-a", 2], ["002-b", 0]]);
  // Only the session working on an issue counts; a review does not stop the issue.
  assert.equal(cols.review[0].waiting, 0);
  assert.equal(waitingIssues(cols), 1);
  assert.deepEqual(placeIssues(items, run).working.map((c) => c.waiting), [0, 0]);
  assert.equal(waitingIssues(placeIssues(items, null, { s1: 1 })), 0);
});

test("a card has the short id, who does it, and what it waits on or how long it has worked", () => {
  const now = new Date("2026-11-10T12:00:00Z");
  const cards = placeIssues(
    [item({ id: "001-first" }), item({ id: "002-second", state: "blocked", waitsOn: ["001-first", "003-third"] }), item({ id: "004-mine", type: "human" }), item({ id: "005-live" }), item({ id: "notes" })],
    { state: "running", steps: [{ issue: "005-live", title: "", state: "working", startedAt: "2026-11-10T11:15:00Z" }] },
  );
  const [first, second, mine, notes] = cards.backlog;
  assert.deepEqual(cardMeta(first, now), ["001", "AFK"]);
  assert.deepEqual(cardMeta(second, now), ["002", "AFK", "after 001, 003"]);
  assert.deepEqual(cardMeta(mine, now), ["004", "Human"]);
  assert.deepEqual(cardMeta(notes, now), ["notes", "AFK"]);
  assert.deepEqual(cardMeta(cards.working[0], now), ["005", "AFK", "started 45 min ago"]);
  assert.equal(shortId("012-the-rest"), "012");
  assert.equal(shortId("fix-login"), "fix-login");
  // The hover text has the whole title and the file; a title is never cut by the data.
  assert.equal(first.hover, "First\nbacklog/issues/001-first.md");
  // A cycle or a file the app cannot read is said on the card, as the issue itself says it.
  const odd = placeIssues([item({ id: "006-loop", state: "blocked", cycle: true }), item({ id: "007-bad", type: "human", problem: "no frontmatter" })], null).backlog;
  assert.equal(odd[0].note, "waits on itself through others");
  assert.equal(odd[1].note, "can't be read: no frontmatter");
  assert.equal(first.note, "");
});

// GUI-048: the columns and the times follow the language.
test("the columns and how long ago are in the language in force", () => {
  const now = new Date("2026-10-31T12:00:00Z");
  assert.deepEqual(LANES.map((l) => laneTitle(l)), ["Backlog", "Working", "In review", "Done"]);
  setLanguage("de");
  assert.deepEqual(LANES.map((l) => laneTitle(l)), ["Backlog", "In Arbeit", "In Prüfung", "Erledigt"]);
  assert.equal(since("2026-10-31T11:15:00Z", now), "vor 45 min");
  assert.equal(since("2026-10-27T12:00:00Z", now), "vor 4 Tagen");
  assert.deepEqual(cardMeta(placeIssues([item({ id: "002-b", state: "blocked", waitsOn: ["001-a"] })], null).backlog[0], now), ["002", "AFK", "nach 001"]);
  setLanguage("nl");
  assert.deepEqual(LANES.map((l) => laneTitle(l)), ["Backlog", "In behandeling", "In beoordeling", "Klaar"]);
  assert.equal(since("2026-10-31T07:00:00Z", now), "5 uur geleden");
  assert.equal(since("2026-10-30T10:00:00Z", now), "gisteren");
  setLanguage("en");
});

test("how long ago, in words", () => {
  const now = new Date("2026-10-31T12:00:00Z");
  assert.equal(since("2026-10-31T11:59:30Z", now), "just now");
  assert.equal(since("2026-10-31T11:15:00Z", now), "45 min ago");
  assert.equal(since("2026-10-31T07:00:00Z", now), "5 h ago");
  assert.equal(since("2026-10-30T10:00:00Z", now), "yesterday");
  assert.equal(since("2026-10-27T12:00:00Z", now), "4 days ago");
});

