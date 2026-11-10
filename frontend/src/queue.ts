// The queue of tasks (SES-025), as the Queue page shows it. Pure, so queue.test.ts
// covers it.
import { t, type PlainKey } from "./i18n.ts";

export type QueueTask = { id: string; title: string; state: string; note?: string; sessionId?: string; projectRoot: string };

export const TASK_STATE_LABEL: Record<string, PlainKey> = {
  waiting: "queue.state.waiting",
  running: "queue.state.running",
  done: "queue.state.done",
  failed: "queue.state.failed",
};

// queueSummary says in one line what the queue is doing.
export function queueSummary(tasks: readonly QueueTask[], paused: boolean, limit: number): string {
  const n = (state: string) => tasks.filter((task) => task.state === state).length;
  const counts = { running: n("running"), limit, waiting: n("waiting") };
  const line = n("failed") > 0 ? t("queue.summaryFailed", { ...counts, failed: n("failed") }) : t("queue.summary", counts);
  return paused ? `${t("queue.paused")} ${line}` : line;
}

// canMove reports whether a task can move by one place that way: only waiting tasks
// change their turn, and not past the ends.
export function canMove(tasks: readonly QueueTask[], i: number, by: number): boolean {
  const j = i + by;
  return tasks[i]?.state === "waiting" && j >= 0 && j < tasks.length;
}
