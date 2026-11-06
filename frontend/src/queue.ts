// The queue of tasks (SES-025), as the Queue page shows it. Pure, so queue.test.ts
// covers it.

export type QueueTask = { id: string; title: string; state: string; note?: string; sessionId?: string; projectRoot: string };

export const TASK_STATE_LABEL: Record<string, string> = {
  waiting: "Waiting",
  running: "Running",
  done: "Done",
  failed: "Failed",
};

// queueSummary says in one line what the queue is doing.
export function queueSummary(tasks: readonly QueueTask[], paused: boolean, limit: number): string {
  const n = (state: string) => tasks.filter((t) => t.state === state).length;
  const parts = [`${n("running")} of ${limit} running`, `${n("waiting")} waiting`];
  if (n("failed") > 0) parts.push(`${n("failed")} failed`);
  return (paused ? "Paused. " : "") + parts.join(", ") + ".";
}

// canMove reports whether a task can move by one place that way: only waiting tasks
// change their turn, and not past the ends.
export function canMove(tasks: readonly QueueTask[], i: number, by: number): boolean {
  const j = i + by;
  return tasks[i]?.state === "waiting" && j >= 0 && j < tasks.length;
}
