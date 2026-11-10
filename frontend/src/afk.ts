// An AFK run in words (WFL-005, WFL-006): what the policy says of a command it will
// want, which to offer allowed, and how far a run has got. Pure, so afk.test.ts
// covers it.
import { t, type PlainKey } from "./i18n.ts";

export interface CommandLike {
  command: string;
  from: string;
  outcome: string;
}

export interface StepLike {
  issue: string;
  title: string;
  state: string;
  // Why its session has not started yet, such as an agent's limit.
  waiting?: string;
  sessionId?: string;
  // When the run began on the issue.
  startedAt?: string;
  // Its review's session, and what the run says of it (why it stopped there).
  reviewId?: string;
  note?: string;
  // When the run finished with the issue (its review ended).
  endedAt?: string;
}

export interface RunLike {
  state: string;
  reason?: string;
  steps: StepLike[] | null;
  branch?: string;
  // The model its issues run on, named for a person, and their effort (WFL-014).
  modelLabel?: string;
  effort?: string;
}

// commandOutcome says what a session of the run would meet now.
export function commandOutcome(c: CommandLike): string {
  switch (c.outcome) {
    case "allowed":
      return t("afk.outcome.allowed");
    case "denied":
      return t("afk.outcome.denied");
    case "ask":
      return t("afk.outcome.ask");
    default:
      return t("afk.outcome.unknown");
  }
}

// defaultAllow ticks what would ask and is not denied: the run's own commands and
// the project's checks, which an unattended session otherwise waits on. The
// project's other check targets are offered unticked: what they touch is not known.
export function defaultAllow(cmds: CommandLike[]): string[] {
  return cmds.filter((c) => c.outcome === "ask" && c.from !== "project-more").map((c) => c.command);
}

// commandFrom says where an offered command comes from.
export function commandFrom(from: string): string {
  switch (from) {
    case "project":
      return t("afk.from.project");
    case "project-more":
      return t("afk.from.projectMore");
    default:
      return t("afk.from.workflow");
  }
}

const STEP: Record<string, PlainKey> = {
  working: "afk.step.working",
  reviewing: "afk.step.reviewing",
  qa: "afk.step.qa",
  qa_done: "afk.step.qaDone",
  failed: "afk.step.failed",
};

export function stepLabel(s: StepLike): string {
  if (s.state === "working" && s.waiting) return t("afk.step.waiting", { reason: s.waiting });
  return STEP[s.state] ? t(STEP[s.state]) : s.state;
}

// waitingStep is the run's issue whose session has not started, if any.
export function waitingStep(r: RunLike): StepLike | undefined {
  if (r.state !== "running" && r.state !== "paused") return undefined;
  return (r.steps ?? []).find((s) => s.state === "working" && !!s.waiting);
}

// issueStep is what the run has done with an issue, for its card on the Board.
export function issueStep(r: RunLike | null | undefined, issue: string): StepLike | undefined {
  return (r?.steps ?? []).filter((s) => s.issue === issue).pop();
}

// The run's line by its state; a state the app does not know reads as stopped.
const LINE = {
  running: "afk.line.running",
  paused: "afk.line.paused",
  finished: "afk.line.finished",
  stopped: "afk.line.stopped",
} as const;

// runLine sums a run up in one line.
export function runLine(r: RunLike): string {
  const steps = r.steps ?? [];
  const done = steps.filter((s) => s.state === "qa" || s.state === "qa_done").length;
  const now = steps.find((s) => s.state === "working" || s.state === "reviewing");
  const line = r.state === "running" || r.state === "paused" || r.state === "finished" ? LINE[r.state] : LINE.stopped;
  const parts = [t(line, { done, count: steps.length })];
  if (now) parts.push(`${now.issue} ${stepLabel(now)}`);
  if (r.reason && r.state !== "running") parts.push(r.reason);
  if (r.modelLabel) parts.push(t("afk.line.issuesOn", { model: [r.modelLabel, r.effort].filter(Boolean).join(" · ") }));
  return parts.join(" · ");
}
