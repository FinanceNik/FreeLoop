import { quietCommand, visibleCommand } from "./a11y.ts";
import { since } from "./board.ts";
import { t, type PlainKey } from "./i18n.ts";

// What a session is doing right now, in a line (GUI-030), so the user can see without
// opening it. Folded from the events every window receives, with no model call and
// the same for every harness. A session not touched since the app opened has none.

export interface Activity {
  seq: number;
  // The tool call or kind of work under way, and when the turn began. A tool call is
  // named by the title the agent gave it; the app's own words (OWN_WORDS) are kept as
  // catalogue keys, so the line is written in the language in force when it is shown.
  doing: string;
  turnStart: string | null;
  // What each call of this turn is called, and the requests waiting for the user.
  tools: Record<string, string>;
  waiting: { requestId: string; what: string }[];
  finishedAt: string | null;
  stopReason: string | null;
  changed: number;
  // How full the context is, for a row in the sidebar (GUI-036).
  context: { tokens: number; window: number } | null;
}

export const NO_ACTIVITY: Activity = { seq: 0, doing: "", turnStart: null, tools: {}, waiting: [], finishedAt: null, stopReason: null, changed: 0, context: null };

type ActivityEvent = { seq: number; ts: string; type: string; data?: unknown };

// The app's own words for the work under way, held in an Activity by their keys.
const THINKING: PlainKey = "status.activity.thinking";
const WRITING: PlainKey = "status.activity.writing";
const TOOL_CALL: PlainKey = "status.activity.toolCall";
const OWN_WORDS: readonly string[] = [THINKING, WRITING, TOOL_CALL];

// foldActivity takes one event. Replayed events are older than what it has seen.
export function foldActivity(a: Activity, ev: ActivityEvent): Activity {
  if (ev.seq <= a.seq) return a;
  const d = (ev.data ?? {}) as { contextTokens?: number; contextWindow?: number; afterTokens?: number; status?: string; toolCallId?: string; title?: string; tool?: string; requestId?: string; command?: string; stopReason?: string; dirty?: number };
  const next = { ...a, seq: ev.seq };
  switch (ev.type) {
    case "turn_started":
      return { ...next, doing: "", turnStart: ev.ts, tools: {}, waiting: [], finishedAt: null, stopReason: null };
    case "agent_thought_chunk":
      return { ...next, doing: THINKING };
    case "agent_message_chunk":
      return { ...next, doing: WRITING };
    case "tool_call": {
      const title = d.title || d.tool || TOOL_CALL;
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
    case "usage_update":
      return d.contextTokens ? { ...next, context: { tokens: d.contextTokens, window: d.contextWindow ?? a.context?.window ?? 0 } } : next;
    case "compaction_update":
      return d.status === "completed" && a.context ? { ...next, context: { ...a.context, tokens: d.afterTokens ?? 0 } } : next;
    default:
      return next;
  }
}

// elapsed says how long something has been going, shortly.
export function elapsed(from: string, now: Date): string {
  const s = Math.max(0, Math.round((now.getTime() - new Date(from).getTime()) / 1000));
  if (s < 60) return t("status.elapsed.seconds", { seconds: s });
  const m = Math.floor(s / 60);
  return m < 60 ? t("status.elapsed.minutes", { minutes: m }) : t("status.elapsed.hours", { hours: Math.floor(m / 60), minutes: m % 60 });
}

// describeActivity writes the line, or "" when it would add nothing to the status. Its
// parts are facts set side by side with a dot, each written whole.
export function describeActivity(status: string, a: Activity | undefined, roots: string[], now: Date): string {
  const shown = (what: string) => (OWN_WORDS.includes(what) ? t(what as PlainKey) : visibleCommand(quietCommand(what, roots)));
  const first = a?.waiting[0]?.what;
  if (status === "waiting_permission") return first ? t("status.activity.needsYou", { what: shown(first) }) : "";
  if (status === "running" || status === "queued") {
    const doing = status === "queued" ? t("status.activity.queued") : a?.doing ? shown(a.doing) : t("status.activity.working");
    return a?.turnStart ? [doing, elapsed(a.turnStart, now)].join(" · ") : doing;
  }
  if (!a?.finishedAt || status !== "idle") return "";
  const ended = a.stopReason === "end_turn" ? t("status.activity.finished") : t("status.activity.stopped");
  const changed = a.changed > 0 ? t("status.activity.changed", { count: a.changed }) : "";
  return [ended, changed, since(a.finishedAt, now)].filter(Boolean).join(" · ");
}
