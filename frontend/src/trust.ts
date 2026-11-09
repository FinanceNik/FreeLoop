import { t } from "./i18n.ts";

// What the "Trust project" actions do next. The hooks a project would run are
// shown before it is trusted (EXT-005), so not knowing them is not the same as
// there being none: null means the lookup has not answered or failed.
export type TrustStep = "lookup" | "confirm" | "trust";

export function trustStep(hooks: readonly unknown[] | null, ask: boolean): TrustStep {
  if (hooks === null) return "lookup";
  return ask && hooks.length > 0 ? "confirm" : "trust";
}

type HookLine = { event: string; match?: string; command: string };
type AgentFile = { path: string; hooks?: string[] | null; allows?: boolean };

// trustLines says what trusting a project lets run or try (SEC-006, SEC-015): its
// .freeloop/ hooks, the hooks Claude Code would run from the repository's own
// settings, and agent settings that tell the agent not to ask, which the app's policy
// still overrules.
export function trustLines(s: { hooks?: HookLine[] | null; agentConfig?: AgentFile[] | null }): string[] {
  const out = (s.hooks ?? []).map((h) =>
    h.match ? t("trust.hookMatching", { event: h.event, match: h.match, command: h.command }) : t("trust.hook", { event: h.event, command: h.command }),
  );
  for (const f of s.agentConfig ?? []) {
    if ((f.hooks ?? []).length > 0) out.push(t("trust.claudeHooks", { path: f.path, hooks: (f.hooks ?? []).join(", ") }));
    if (f.allows) out.push(t("trust.agentAllows", { path: f.path }));
  }
  return out;
}

