import { test } from "node:test";
import assert from "node:assert/strict";
import { capDone, cardMeta, LANES, laneTitle, placeIssues, readCollapsed, rowsFor, rowState, shortId, since, waitingIssues, writeCollapsed } from "./board.ts";
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

// GUI-016, decisions 5 and 6: In review is the agent's review and then the user's QA, and
// an issue a run stopped at is back in Backlog, with why, until a newer run starts.
test("in review, the agent's review and the user's QA are told apart", () => {
  const items = [item({ id: "001-a", state: "done" }), item({ id: "002-b", state: "done" }), item({ id: "003-c" })];
  const cols = placeIssues(items, {
    state: "running",
    steps: [
      { issue: "001-a", title: "", state: "reviewing", reviewId: "r1", sessionId: "s1" },
      { issue: "002-b", title: "", state: "qa", reviewId: "r2", sessionId: "s2" },
      { issue: "003-c", title: "", state: "working", sessionId: "s3" },
    ],
  });
  assert.deepEqual(cols.review.map((c) => [c.id, c.review, c.step?.reviewId]), [["001-a", "reviewing", "r1"], ["002-b", "qa", "r2"]]);
  assert.equal(cols.working[0].review, undefined);
});

test("an issue a run stopped at is back in Backlog with the reason, until a newer run", () => {
  const items = [item({ id: "001-a" }), item({ id: "002-b" }), item({ id: "003-c" })];
  const stopped = {
    state: "stopped",
    reason: "The session for 001-a failed: see the session",
    steps: [
      { issue: "001-a", title: "", state: "failed", note: "its session ended without marking it done", sessionId: "s1" },
      { issue: "002-b", title: "", state: "failed", sessionId: "s2" },
    ],
  };
  const cols = placeIssues(items, stopped);
  assert.deepEqual(ids(cols.backlog), ["001-a", "002-b", "003-c"]);
  // The step's own note says why; without one, the run's reason does.
  assert.deepEqual(cols.backlog.map((c) => c.stopped), ["its session ended without marking it done", "The session for 001-a failed: see the session", undefined]);
  assert.equal(cols.backlog[0].step?.sessionId, "s1");
  // A newer run has not been to the issue yet: nothing marks it.
  const newer = placeIssues(items, { state: "running", steps: [] });
  assert.deepEqual(newer.backlog.map((c) => c.stopped), [undefined, undefined, undefined]);
  // And one that has taken it up has it in Working, not stopped.
  assert.equal(placeIssues(items, { state: "running", steps: [{ issue: "001-a", title: "", state: "working" }] }).working[0].stopped, undefined);
  // No reason at all still says the run stopped there.
  assert.equal(placeIssues([item({ id: "001-a" })], { state: "stopped", steps: [{ issue: "001-a", title: "", state: "failed" }] }).backlog[0].stopped, "");
});

// GUI-016, decision 9: Done shows what the run finished most recently first, then the rest by
// id, and only the last five until asked for all.
test("Done is newest first by when the run finished each issue, then the rest by id, capped at five", () => {
  const items = ["001-a", "002-b", "003-c", "004-d", "005-e", "006-f", "007-g", "008-h"].map((id) => item({ id, state: "done" }));
  const at = (issue: string, endedAt: string) => ({ issue, title: "", state: "qa_done", endedAt });
  const cols = placeIssues(items, { state: "finished", steps: [at("001-a", "2026-11-10T08:00:00Z"), at("006-f", "2026-11-10T09:00:00Z"), at("003-c", "2026-11-10T10:00:00Z")] });
  assert.deepEqual(ids(cols.done), ["003-c", "006-f", "001-a", "002-b", "004-d", "005-e", "007-g", "008-h"]);
  const capped = capDone(cols.done, false);
  assert.deepEqual([ids(capped.shown), capped.hidden], [["003-c", "006-f", "001-a", "002-b", "004-d"], 3]);
  assert.deepEqual([capDone(cols.done, true).shown.length, capDone(cols.done, true).hidden], [8, 0]);
  assert.deepEqual([capDone(cols.done.slice(0, 5), false).shown.length, capDone(cols.done.slice(0, 5), false).hidden], [5, 0]);
});

// GUI-016, decision 3: a row collapses and stays so; a filter shows one project; a row with
// an issue that waits for the user is shown open, whatever was chosen.
test("a row collapses and the choice is kept", () => {
  const store = (() => {
    const data = new Map<string, string>();
    return { getItem: (k: string) => data.get(k) ?? null, setItem: (k: string, v: string) => void data.set(k, v) };
  })();
  assert.deepEqual(readCollapsed(store), []);
  writeCollapsed(store, ["prj_a", "prj_b"]);
  assert.deepEqual(readCollapsed(store), ["prj_a", "prj_b"]);
  // What cannot be read, or no storage at all, is no choice.
  assert.deepEqual(readCollapsed({ getItem: () => "{not json", setItem: () => {} }), []);
  assert.deepEqual(readCollapsed({ getItem: () => '{"a":1}', setItem: () => {} }), []);
  assert.deepEqual(readCollapsed({ getItem: () => { throw new Error("blocked"); }, setItem: () => {} }), []);
  assert.deepEqual(readCollapsed(undefined), []);
  assert.doesNotThrow(() => writeCollapsed({ getItem: () => null, setItem: () => { throw new Error("full"); } }, ["x"]));
  assert.deepEqual(rowState(false, 0), { open: true, forced: false });
  assert.deepEqual(rowState(true, 0), { open: false, forced: false });
  assert.deepEqual(rowState(true, 2), { open: true, forced: true });
  assert.deepEqual(rowState(false, 2), { open: true, forced: false });
});

test("a filter shows one project's row or all of them", () => {
  const rows = [{ projectId: "a" }, { projectId: "b" }, { projectId: "c" }];
  assert.deepEqual(rowsFor(rows, "").map((r) => r.projectId), ["a", "b", "c"]);
  assert.deepEqual(rowsFor(rows, "b").map((r) => r.projectId), ["b"]);
  // A project that has no row any more (its backlog went) filters nothing out.
  assert.deepEqual(rowsFor(rows, "gone").map((r) => r.projectId), ["a", "b", "c"]);
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

