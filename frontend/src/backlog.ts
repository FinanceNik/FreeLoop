// A workflow's backlog on the Board (WFL-004): what each issue waits on and what it
// holds up, said in words. Pure, so backlog.test.ts covers it.
import { t, type PlainKey } from "./i18n.ts";

export interface BacklogItem {
  id: string;
  title: string;
  state: string;
  type: string;
  waitsOn: string[] | null;
  blocks: string[] | null;
  cycle?: boolean;
  problem?: string;
}

const STATE: Record<string, PlainKey> = { ready: "backlog.state.ready", blocked: "backlog.state.blocked", done: "backlog.state.done" };

// stateLabel is an issue's state in words; one the app does not know, as the file says it.
export function stateLabel(it: BacklogItem): string {
  return STATE[it.state] ? t(STATE[it.state]) : it.state;
}

// typeLabel says who does an issue: an agent unattended, or a person.
export function typeLabel(it: BacklogItem): string {
  return t(it.type === "afk" ? "backlog.type.afk" : "backlog.type.human");
}

// relations says what an issue waits on and what it holds up, in one line.
export function relations(it: BacklogItem): string {
  const parts: string[] = [];
  if (it.cycle) parts.push(t("backlog.cycle"));
  else if ((it.waitsOn ?? []).length) parts.push(t("backlog.after", { issues: (it.waitsOn ?? []).join(", ") }));
  if ((it.blocks ?? []).length) parts.push(t("backlog.then", { issues: (it.blocks ?? []).join(", ") }));
  if (it.problem) parts.push(t("backlog.unreadable", { problem: it.problem }));
  return parts.join(" · ");
}

// counts sums up a backlog for its heading: how many AFK issues are ready to run,
// and how many are open in all.
export function counts(items: BacklogItem[] | null): { open: number; afkReady: number; done: number } {
  const list = items ?? [];
  return {
    open: list.filter((i) => i.state !== "done").length,
    afkReady: list.filter((i) => i.state === "ready" && i.type === "afk" && !i.problem).length,
    done: list.filter((i) => i.state === "done").length,
  };
}
