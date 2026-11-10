// The policy page's check (SEC-018): what a person types, as the engine's query,
// and the verdict in words.
import { t, type PlainKey } from "./i18n.ts";

// Each action's label is a catalogue key, translated where it shows; the examples are
// commands and paths, shown as given.
export const CHECK_TOOLS = [
  { id: "shell", label: "policy.tool.shell", placeholder: "git push origin main" },
  { id: "edit", label: "policy.tool.edit", placeholder: ".git/hooks/pre-commit" },
  { id: "read", label: "policy.tool.read", placeholder: ".env" },
  { id: "fetch", label: "policy.tool.fetch", placeholder: "https://example.com" },
] as const satisfies readonly { id: string; label: PlainKey; placeholder: string }[];

export interface CheckInput {
  where: string; // "session:<id>" or "project:<root>"
  mode: string; // "" for the session's or the default
  tool: string;
  what: string;
}

export interface Query {
  sessionId: string;
  projectRoot: string;
  profileId: string;
  environmentId: string;
  mode: string;
  tool: string;
  command: string;
  path: string;
}

export function toQuery(input: CheckInput): Query {
  const [kind, ...rest] = input.where.split(":");
  const at = rest.join(":");
  const what = input.what.trim();
  const command = input.tool === "shell" ? what : "";
  return {
    sessionId: kind === "session" ? at : "",
    projectRoot: kind === "project" ? at : "",
    profileId: "",
    environmentId: "",
    mode: input.mode,
    tool: input.tool,
    command,
    path: input.tool === "shell" ? "" : what,
  };
}

interface Verdict {
  outcome: string;
  reason: string;
  rule?: { scope: string; source: string } | null;
}

// headline is the verdict's first line: what happens, then who decided, in the
// engine's words.
export function headline(v: Verdict): { tone: "ok" | "warn" | "error"; text: string } {
  const why = v.reason.charAt(0).toUpperCase() + v.reason.slice(1);
  switch (v.outcome) {
    case "allowed":
      return { tone: "ok", text: t("policy.allowed", { why }) };
    case "denied":
      return { tone: "error", text: t("policy.denied", { why }) };
    default:
      return { tone: "warn", text: t("policy.asked", { why }) };
  }
}

// Where a rule lives, named as the right panel names it; a scope not listed shows as
// the engine names it.
const SCOPE: Record<string, PlainKey> = {
  global: "panel.scope.global",
  environment: "policy.scope.environment",
  profile: "policy.scope.profile",
  project: "panel.scope.project",
  session: "panel.rules.listed.session",
};

// ruleWhere says where a rule lives, in the words of the page.
export function ruleWhere(r: { scope: string; source: string }): string {
  const key = SCOPE[r.scope];
  const scope = key ? t(key) : r.scope;
  return r.source ? `${scope}, ${r.source}` : scope;
}
