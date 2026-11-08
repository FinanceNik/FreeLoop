import { t, type PlainKey } from "./i18n.ts";

// What waits for approval in the app's files (SET-005), for reading: by what it
// does, what runs first.

export const EVENTS: { id: string; label: PlainKey }[] = [
  { id: "session_start", label: "hooks.event.sessionStart" },
  { id: "pre_tool_use", label: "hooks.event.preToolUse" },
  { id: "permission_request", label: "hooks.event.permissionRequest" },
  { id: "post_tool_use", label: "hooks.event.postToolUse" },
];

// Settings is where one of a kind no group names goes.
const SETTINGS_GROUP: PlainKey = "pending.group.settings";

const GROUPS: { title: PlainKey; kinds: string[] }[] = [
  { title: "pending.group.commands", kinds: ["hook", "project_hooks", "agent", "optimizer", "mcp"] },
  { title: "pending.group.rules", kinds: ["rule"] },
  { title: "pending.group.environments", kinds: ["environment"] },
  { title: SETTINGS_GROUP, kinds: ["setting"] },
  { title: "pending.group.addresses", kinds: ["address"] },
];

const KINDS: Record<string, PlainKey> = {
  project_hooks: "pending.kind.projectHooks",
  agent: "pending.kind.agent",
  optimizer: "pending.kind.optimizer",
  mcp: "pending.kind.mcp",
  rule: "pending.kind.rule",
  environment: "pending.kind.environment",
  address: "pending.kind.address",
};

export function pendingLabel(c: { kind: string; event: string }): string {
  if (c.kind === "hook") {
    const event = EVENTS.find((e) => e.id === c.event);
    return t("pending.kind.hook", { event: event ? t(event.label) : c.event });
  }
  return t(KINDS[c.kind] ?? "pending.kind.setting");
}

// groupPending keeps every item: one of a kind no group names goes under Settings
// rather than out of sight.
export function groupPending<T extends { kind: string }>(items: T[]): { title: string; items: T[] }[] {
  const known = GROUPS.flatMap((g) => g.kinds);
  return GROUPS.map((g) => ({
    title: t(g.title),
    items: items.filter((c) => g.kinds.includes(c.kind) || (g.title === SETTINGS_GROUP && !known.includes(c.kind))),
  })).filter((g) => g.items.length > 0);
}

// approveAllQuestion is what the user agrees to: how many, of what.
export function approveAllQuestion(items: { kind: string }[]): string {
  const counts = groupPending(items).map((g) => `${g.title}: ${g.items.length}`);
  return t("pending.approveAllQuestion", { count: items.length, counts: counts.join("\n") });
}
