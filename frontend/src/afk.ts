// An AFK run in words (WFL-005, WFL-006): what the policy says of a command it will
// want, which to offer allowed, and how far a run has got. Pure, so afk.test.ts
// covers it.

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
}

export interface RunLike {
  state: string;
  reason?: string;
  steps: StepLike[] | null;
  branch?: string;
}

// commandOutcome says what a session of the run would meet now.
export function commandOutcome(c: CommandLike): string {
  switch (c.outcome) {
    case "allowed":
      return "already allowed";
    case "denied":
      return "denied by your policy";
    case "ask":
      return "would ask";
    default:
      return "not known";
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
      return "the project's checks";
    case "project-more":
      return "another of the project's checks";
    default:
      return "the workflow";
  }
}

const STEP: Record<string, string> = {
  working: "working",
  reviewing: "in review",
  qa: "ready for QA",
  qa_done: "QA'd",
  failed: "stopped here",
};

export function stepLabel(s: StepLike): string {
  if (s.state === "working" && s.waiting) return `waiting to start: ${s.waiting}`;
  return STEP[s.state] ?? s.state;
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

// runLine sums a run up in one line.
export function runLine(r: RunLike): string {
  const steps = r.steps ?? [];
  const done = steps.filter((s) => s.state === "qa" || s.state === "qa_done").length;
  const now = steps.find((s) => s.state === "working" || s.state === "reviewing");
  const head =
    r.state === "running" ? "AFK run going on" : r.state === "paused" ? "AFK run paused" : r.state === "finished" ? "AFK run finished" : "AFK run stopped";
  const parts = [`${head}: ${done} of ${steps.length} ${steps.length === 1 ? "issue" : "issues"} done`];
  if (now) parts.push(`${now.issue} ${stepLabel(now)}`);
  if (r.reason && r.state !== "running") parts.push(r.reason);
  return parts.join(" · ");
}
