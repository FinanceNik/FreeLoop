// The policy page's check (SEC-018): what a person types, as the engine's query,
// and the verdict in words.

export const CHECK_TOOLS = [
  { id: "shell", label: "Run a command", placeholder: "git push origin main" },
  { id: "edit", label: "Edit a file", placeholder: ".git/hooks/pre-commit" },
  { id: "read", label: "Read a file", placeholder: ".env" },
  { id: "fetch", label: "Fetch a URL", placeholder: "https://example.com" },
] as const;

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

// headline is the verdict's first line: what happens, then who decided.
export function headline(v: Verdict): { tone: "ok" | "warn" | "error"; text: string } {
  const why = v.reason.charAt(0).toUpperCase() + v.reason.slice(1);
  switch (v.outcome) {
    case "allowed":
      return { tone: "ok", text: `Allowed. ${why}.` };
    case "denied":
      return { tone: "error", text: `Denied. ${why}.` };
    default:
      return { tone: "warn", text: `You would be asked. ${why}.` };
  }
}

// ruleWhere says where a rule lives, in the words of the page.
export function ruleWhere(r: { scope: string; source: string }): string {
  const scope = { global: "global", environment: "environment", profile: "profile", project: "project", session: "this session" }[r.scope] ?? r.scope;
  return r.source ? `${scope}, ${r.source}` : scope;
}
