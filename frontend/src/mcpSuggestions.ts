// Suggested MCP servers (MCP-010): a short list built into the app, so nothing is
// fetched until the user adds one. Adding one opens the usual form filled in, and the
// server runs only once saved and approved, as any other. Remote servers run at their
// vendor and sign in with OAuth or a token; a local one runs on this computer with the
// user's rights, pinned to one version. Addresses and versions were checked against
// each vendor's documentation and npm on 2026-11-08; keep them that way when changing
// one. No server that reads files directly is suggested: it would go around the
// protected paths the agent's own file tools keep to.

export interface McpSuggestion {
  id: string;
  name: string;
  // What it is for, in a line.
  description: string;
  // Where it runs and what it can reach, said plainly.
  reach: string;
  transport: "http" | "stdio";
  url?: string;
  command?: string;
  args?: string[];
  // A header holding a secret, filled in before saving; it goes to the keyring.
  secretHeader?: { name: string; hint: string };
  // How it signs in: OAuth in the browser, a token in a header, or not at all.
  signIn: "oauth" | "token" | "none";
  docs: string;
}

export const MCP_SUGGESTIONS: McpSuggestion[] = [
  {
    id: "github",
    name: "GitHub",
    description: "Issues, pull requests, code search and Actions in your repositories.",
    reach: "Runs at GitHub. It can do what the personal access token you give it allows.",
    transport: "http",
    url: "https://api.githubcopilot.com/mcp/",
    secretHeader: { name: "Authorization", hint: "Bearer followed by a GitHub personal access token" },
    signIn: "token",
    docs: "https://github.com/github/github-mcp-server",
  },
  {
    id: "linear",
    name: "Linear",
    description: "Issues, projects and comments in your Linear workspace.",
    reach: "Runs at Linear, as you, once you sign in.",
    transport: "http",
    url: "https://mcp.linear.app/mcp",
    signIn: "oauth",
    docs: "https://linear.app/docs/mcp",
  },
  {
    id: "sentry",
    name: "Sentry",
    description: "Errors, issues and releases in your Sentry organization.",
    reach: "Runs at Sentry, as you, once you sign in.",
    transport: "http",
    url: "https://mcp.sentry.dev/mcp",
    signIn: "oauth",
    docs: "https://docs.sentry.io/ai/mcp/",
  },
  {
    id: "notion",
    name: "Notion",
    description: "Pages and databases in your Notion workspace.",
    reach: "Runs at Notion, as you, once you sign in.",
    transport: "http",
    url: "https://mcp.notion.com/mcp",
    signIn: "oauth",
    docs: "https://developers.notion.com/guides/mcp/get-started-with-mcp",
  },
  {
    id: "context7",
    name: "Context7",
    description: "Current documentation and code examples for libraries.",
    reach: "Runs at Context7, which receives the library names and questions the agent asks about.",
    transport: "http",
    url: "https://mcp.context7.com/mcp",
    signIn: "none",
    docs: "https://github.com/upstash/context7",
  },
  {
    id: "playwright",
    name: "Playwright",
    description: "A browser the agent drives, to test web apps.",
    reach: "Runs on this computer with your rights: it opens a browser and visits the pages the agent chooses. Needs Node.js.",
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
