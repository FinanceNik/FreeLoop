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
// the project's checks, which an unattended session otherwise waits on.
export function defaultAllow(cmds: CommandLike[]): string[] {
  return cmds.filter((c) => c.outcome === "ask").map((c) => c.command);
}

const STEP: Record<string, string> = {
  working: "working",
  reviewing: "in review",
  qa: "ready for QA",
  qa_done: "QA'd",
  failed: "stopped here",
};

export function stepLabel(s: StepLike): string {
  return STEP[s.state] ?? s.state;
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
