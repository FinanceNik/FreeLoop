// The board view's columns (GUI-016). Pure, so board.test.ts covers it.
import { t, type PlainKey } from "./i18n.ts";

import { relations, typeLabel, type BacklogItem } from "./backlog.ts";
import type { RunLike, StepLike } from "./afk.ts";

// The Board is a scrum board of workflow issues (GUI-016): one row per project, and in it
// one card per issue in one of four columns. A card's column only shows where the issue
// stands, from its file and the AFK run's step for it, so the Board can never disagree
// with either; nothing moves a card by hand.
export type Lane = "backlog" | "working" | "review" | "done";

export const LANES: Lane[] = ["backlog", "working", "review", "done"];

const LANE_TITLE: Record<Lane, PlainKey> = {
  backlog: "board.lane.backlog",
  working: "board.lane.working",
  review: "board.lane.review",
  done: "board.lane.done",
};

// laneTitle is a column's name in the language in force.
export function laneTitle(lane: Lane): string {
  return t(LANE_TITLE[lane]);
}

// IssueCard is an issue as the Board shows it.
export interface IssueCard {
  id: string;
  // The number it starts with ("001"), or its whole id when it has none.
  short: string;
  title: string;
  path: string;
  type: string;
  state: string;
  column: Lane;
  waitsOn: string[];
  // The run's step for it, when there is one, and when that began.
  step?: StepLike;
  startedAt?: string;
  // How many requests its session has waiting for the user, while the run works on it.
  waiting: number;
  // A cycle or a file the app cannot read, in words; empty when neither.
  note: string;
  // The whole title and the file, for hovering: the card cuts a long title short.
  hover: string;
}

// shortId is the number an issue's id starts with, which is how its neighbours name it.
export function shortId(id: string): string {
  const m = /^(\d+)-/.exec(id);
  return m ? m[1] : id;
}

// columnOf is the column an issue is in. A step the run is on decides it, working or
// reviewing or waiting for the user's QA; otherwise the file does: done, or still open.
// An issue a run stopped at is open again, so it is back in the backlog.
function columnOf(it: BacklogItem, step: StepLike | undefined): Lane {
  switch (step?.state) {
    case "working":
      return "working";
    case "reviewing":
    case "qa":
      return "review";
    case "qa_done":
      return "done";
  }
  return it.state === "done" ? "done" : "backlog";
}

// placeIssues sorts a project's issues into the Board's columns, in the order the
// engine listed them. The run's latest step for an issue counts; steps for issues no
// longer in the backlog are ignored.
export function placeIssues(items: BacklogItem[] | null | undefined, run: RunLike | null | undefined, waiting: Record<string, number> = {}): Record<Lane, IssueCard[]> {
  const out: Record<Lane, IssueCard[]> = { backlog: [], working: [], review: [], done: [] };
  const latest = new Map<string, StepLike>();
  for (const s of run?.steps ?? []) latest.set(s.issue, s);
  for (const it of items ?? []) {
    const step = latest.get(it.id);
    const column = columnOf(it, step);
    out[column].push({
      id: it.id,
      short: shortId(it.id),
      title: it.title,
      path: it.path ?? "",
      type: it.type,
      state: it.state,
      column,
      waitsOn: it.waitsOn ?? [],
      step,
      startedAt: step?.startedAt,
      // Waiting is a mark and not a column: the issue stays where the run has it. Only the
      // session working on the issue counts, not a review's.
      waiting: column === "working" && step?.sessionId ? (waiting[step.sessionId] ?? 0) : 0,
      note: it.cycle || it.problem ? relationsOf(it) : "",
      hover: it.path ? `${it.title}\n${it.path}` : it.title,
    });
  }
  return out;
}

// waitingIssues counts the cards whose agent waits for the user.
export function waitingIssues(cols: Record<Lane, IssueCard[]>): number {
  return LANES.reduce((n, lane) => n + cols[lane].filter((c) => c.waiting > 0).length, 0);
}

// relationsOf says only what is wrong with an issue's place in the backlog: a cycle, or
// a file the app cannot read. What it waits on is in the card's meta line.
function relationsOf(it: BacklogItem): string {
  return relations({ ...it, waitsOn: [], blocks: [] });
}

// cardMeta is a card's one meta line, in parts: its number, who does it, and what it
// waits on in the backlog or how long ago it started working.
export function cardMeta(c: IssueCard, now: Date = new Date()): string[] {
  const parts = [c.short, typeLabel({ type: c.type } as BacklogItem)];
  if (c.column === "backlog" && c.state === "blocked" && c.waitsOn.length > 0) parts.push(t("backlog.after", { issues: c.waitsOn.map(shortId).join(", ") }));
  if (c.column === "working" && c.startedAt) parts.push(t("board.meta.started", { when: since(c.startedAt, now) }));
  return parts;
}

// since says how long ago a time was, in the largest whole unit.
export function since(at: string | Date, now: Date = new Date()): string {
  const s = Math.max(0, Math.round((now.getTime() - new Date(at).getTime()) / 1000));
  if (s < 60) return t("board.since.now");
  const m = Math.floor(s / 60);
  if (m < 60) return t("board.since.minutes", { minutes: m });
  const h = Math.floor(m / 60);
  if (h < 24) return t("board.since.hours", { hours: h });
  const d = Math.floor(h / 24);
  return d === 1 ? t("board.since.yesterday") : t("board.since.days", { days: d });
}
