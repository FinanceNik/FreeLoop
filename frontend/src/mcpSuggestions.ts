// Suggested MCP servers (MCP-010): a short list built into the app, so nothing is
// fetched until the user adds one. Adding one opens the usual form filled in, and the
// server runs only once saved and approved, as any other. Remote servers run at their
// vendor and sign in with OAuth or a token; a local one runs on this computer with the
// user's rights, pinned to one version. Addresses and versions were checked against
// each vendor's documentation and npm on 2026-11-08; keep them that way when changing
// one. No server that reads files directly is suggested: it would go around the
// protected paths the agent's own file tools keep to. The descriptions are the app's own
// words, held as catalogue keys and translated where they show (GUI-048); names, header
// names and addresses are shown as given.
import type { PlainKey } from "./i18n.ts";

// AUTH_HEADER is the header a token goes in: a protocol value, never translated.
export const AUTH_HEADER = "Authorization";

export interface McpSuggestion {
  id: string;
  name: string;
  // What it is for, in a line.
  description: PlainKey;
  // Where it runs and what it can reach, said plainly.
  reach: PlainKey;
  transport: "http" | "stdio";
  url?: string;
  command?: string;
  args?: string[];
  // A header holding a secret, filled in before saving; it goes to the keyring.
  secretHeader?: { name: string; hint: PlainKey };
  // How it signs in: OAuth in the browser, a token in a header, or not at all.
  signIn: "oauth" | "token" | "none";
  docs: string;
}

export const MCP_SUGGESTIONS: McpSuggestion[] = [
  {
    id: "github",
    name: "GitHub",
    description: "mcp.suggest.github.description",
    reach: "mcp.suggest.github.reach",
    transport: "http",
    url: "https://api.githubcopilot.com/mcp/",
    secretHeader: { name: AUTH_HEADER, hint: "mcp.suggest.github.hint" },
    signIn: "token",
    docs: "https://github.com/github/github-mcp-server",
  },
  {
    id: "linear",
    name: "Linear",
    description: "mcp.suggest.linear.description",
    reach: "mcp.suggest.linear.reach",
    transport: "http",
    url: "https://mcp.linear.app/mcp",
    signIn: "oauth",
    docs: "https://linear.app/docs/mcp",
  },
  {
    id: "sentry",
    name: "Sentry",
    description: "mcp.suggest.sentry.description",
    reach: "mcp.suggest.sentry.reach",
    transport: "http",
    url: "https://mcp.sentry.dev/mcp",
    signIn: "oauth",
    docs: "https://docs.sentry.io/ai/mcp/",
  },
  {
    id: "notion",
    name: "Notion",
    description: "mcp.suggest.notion.description",
    reach: "mcp.suggest.notion.reach",
    transport: "http",
    url: "https://mcp.notion.com/mcp",
    signIn: "oauth",
    docs: "https://developers.notion.com/guides/mcp/get-started-with-mcp",
  },
  {
    id: "context7",
    name: "Context7",
    description: "mcp.suggest.context7.description",
    reach: "mcp.suggest.context7.reach",
    transport: "http",
    url: "https://mcp.context7.com/mcp",
    signIn: "none",
    docs: "https://github.com/upstash/context7",
  },
  {
    id: "playwright",
    name: "Playwright",
    description: "mcp.suggest.playwright.description",
    reach: "mcp.suggest.playwright.reach",
    transport: "stdio",
    command: "npx",
    args: ["-y", "@playwright/mcp@0.0.84"],
    signIn: "none",
    docs: "https://github.com/microsoft/playwright-mcp",
  },
];

// pinned reports whether a local suggestion runs one exact version, never "latest"
// or a range: what runs changes only when the app's list does.
export function pinned(s: McpSuggestion): boolean {
  if (s.transport !== "stdio") return true;
  const pkg = (s.args ?? []).find((a) => a.includes("@") && !a.startsWith("-"));
  return !!pkg && /@\d+\.\d+\.\d+$/.test(pkg);
}

// added reports whether a server like the suggestion is already set up, by ID, by
// address or by command, so it is offered only once.
export function added(s: McpSuggestion, servers: { id: string; url?: string; command?: string; args?: string[] | null }[]): boolean {
  return servers.some(
    (x) =>
      x.id === s.id ||
      (s.url !== undefined && x.url === s.url) ||
      (s.command !== undefined && x.command === s.command && JSON.stringify(x.args ?? []) === JSON.stringify(s.args ?? [])),
  );
}
