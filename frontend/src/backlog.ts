// A workflow's backlog on the Board (WFL-004): what each issue waits on and what it
// holds up, said in words. Pure, so backlog.test.ts covers it.

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

const STATE: Record<string, string> = { ready: "Ready", blocked: "Blocked", done: "Done" };

export function stateLabel(it: BacklogItem): string {
  return STATE[it.state] ?? it.state;
}

export function typeLabel(it: BacklogItem): string {
  return it.type === "afk" ? "AFK" : "Human";
}

// relations says what an issue waits on and what it holds up, in one line.
export function relations(it: BacklogItem): string {
  const parts: string[] = [];
  if (it.cycle) parts.push("waits on itself through others");
  else if ((it.waitsOn ?? []).length) parts.push(`after ${(it.waitsOn ?? []).join(", ")}`);
  if ((it.blocks ?? []).length) parts.push(`then ${(it.blocks ?? []).join(", ")}`);
  if (it.problem) parts.push(`can't be read: ${it.problem}`);
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
