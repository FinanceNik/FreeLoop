// What waits for approval in the app's files (SET-005), for reading: by what it
// does, what runs first.

export const EVENTS = [
  { id: "session_start", name: "Session start" },
  { id: "pre_tool_use", name: "Before a tool call" },
  { id: "permission_request", name: "Before asking you" },
  { id: "post_tool_use", name: "After a tool call" },
];

const GROUPS: { title: string; kinds: string[] }[] = [
  { title: "Commands that would run", kinds: ["hook", "project_hooks", "agent", "optimizer", "mcp"] },
  { title: "Rules that would allow without asking", kinds: ["rule"] },
  { title: "Environments", kinds: ["environment"] },
  { title: "Settings", kinds: ["setting"] },
  { title: "Where things would be sent", kinds: ["address"] },
];

export function pendingLabel(c: { kind: string; event: string }): string {
  switch (c.kind) {
    case "hook":
      return `Hook (${EVENTS.find((e) => e.id === c.event)?.name ?? c.event})`;
    case "project_hooks":
      return "A trusted project's changed hooks";
    case "agent":
      return "Agent";
    case "optimizer":
      return "Token optimizer";
    case "mcp":
      return "MCP server";
    case "rule":
      return "Rule";
    case "environment":
      return "Environment";
    case "address":
      return "Address";
    default:
      return "Setting";
  }
}

// groupPending keeps every item: one of a kind no group names goes under Settings
// rather than out of sight.
export function groupPending<T extends { kind: string }>(items: T[]): { title: string; items: T[] }[] {
  const known = GROUPS.flatMap((g) => g.kinds);
  return GROUPS.map((g) => ({
    title: g.title,
    items: items.filter((c) => g.kinds.includes(c.kind) || (g.title === "Settings" && !known.includes(c.kind))),
  })).filter((g) => g.items.length > 0);
}

// approveAllQuestion is what the user agrees to: how many, of what.
export function approveAllQuestion(items: { kind: string }[]): string {
  const counts = groupPending(items).map((g) => `${g.title}: ${g.items.length}`);
  return `Approve all ${items.length}?\n\n${counts.join("\n")}\n\nThey take effect at once, and a backup can hold anything. Approve all only if you know where these came from, such as your own backup from another computer.`;
}
