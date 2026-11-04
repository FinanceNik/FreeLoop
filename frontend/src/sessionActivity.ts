import { quietCommand, visibleCommand } from "./a11y.ts";
import { since } from "./board.ts";

// What a session is doing right now, in a line (GUI-030), so the user can see without
// opening it. Folded from the events every window receives, with no model call and
// the same for every harness. A session not touched since the app opened has none.

export interface Activity {
  seq: number;
  // The tool call or kind of work under way, and when the turn began.
  doing: string;
  turnStart: string | null;
  // What each call of this turn is called, and the requests waiting for the user.
  tools: Record<string, string>;
  waiting: { requestId: string; what: string }[];
  finishedAt: string | null;
  stopReason: string | null;
  changed: number;
}

export const NO_ACTIVITY: Activity = { seq: 0, doing: "", turnStart: null, tools: {}, waiting: [], finishedAt: null, stopReason: null, changed: 0 };

type ActivityEvent = { seq: number; ts: string; type: string; data?: unknown };

// foldActivity takes one event. Replayed events are older than what it has seen.
export function foldActivity(a: Activity, ev: ActivityEvent): Activity {
  if (ev.seq <= a.seq) return a;
  const d = (ev.data ?? {}) as { toolCallId?: string; title?: string; tool?: string; requestId?: string; command?: string; stopReason?: string; dirty?: number };
  const next = { ...a, seq: ev.seq };
  switch (ev.type) {
    case "turn_started":
      return { ...next, doing: "", turnStart: ev.ts, tools: {}, waiting: [], finishedAt: null, stopReason: null };
    case "agent_thought_chunk":
      return { ...next, doing: "Thinking" };
    case "agent_message_chunk":
      return { ...next, doing: "Writing" };
    case "tool_call": {
      const title = d.title || d.tool || "Tool call";
      return { ...next, doing: title, tools: d.toolCallId ? { ...a.tools, [d.toolCallId]: title } : a.tools };
    }
    case "tool_call_update":
      if (!d.title || !d.toolCallId) return next;
      return { ...next, doing: a.doing === a.tools[d.toolCallId] ? d.title : a.doing, tools: { ...a.tools, [d.toolCallId]: d.title } };
    case "permission_requested":
      return { ...next, waiting: [...a.waiting, { requestId: d.requestId ?? "", what: d.command || a.tools[d.toolCallId ?? ""] || d.title || "" }] };
    case "permission_decided":
      return { ...next, waiting: a.waiting.filter((w) => w.requestId !== d.requestId) };
    case "turn_ended":
      return { ...next, doing: "", waiting: [], finishedAt: ev.ts, stopReason: d.stopReason ?? "" };
    case "git_state":
      return { ...next, changed: d.dirty ?? 0 };
    default:
      return next;
  }
}

// elapsed says how long something has been going, shortly.
export function elapsed(from: string, now: Date): string {
  const s = Math.max(0, Math.round((now.getTime() - new Date(from).getTime()) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${m % 60}m`;
}

// describeActivity writes the line, or "" when it would add nothing to the status.
export function describeActivity(status: string, a: Activity | undefined, roots: string[], now: Date): string {
  const shown = (what: string) => visibleCommand(quietCommand(what, roots));
  const first = a?.waiting[0]?.what;
  if (status === "waiting_permission") return first ? `Needs you: ${shown(first)}` : "";
  if (status === "running" || status === "queued") {
    const doing = status === "queued" ? "Waiting for a slot" : a?.doing ? shown(a.doing) : "Working";
    return a?.turnStart ? `${doing} · ${elapsed(a.turnStart, now)}` : doing;
  }
  if (!a?.finishedAt || status !== "idle") return "";
  const changed = a.changed > 0 ? ` · ${a.changed} ${a.changed === 1 ? "file" : "files"} changed` : "";
  return `${a.stopReason === "end_turn" ? "Finished" : "Stopped"}${changed} · ${since(a.finishedAt, now)}`;
}
