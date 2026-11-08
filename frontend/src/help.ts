// The help pages (GUI-047): which topics there are, in what order, where each one
// leads in the app, and searching their text. The pages themselves are Markdown in
// src/help/, read by Help.tsx; this module has no runtime imports so `node --test`
// can run it.

// A place in the app a topic can open: a screen's kind, or "demo" for the demo.
export type HelpPlace =
  | "demo"
  | "new"
  | "board"
  | "queue"
  | "attention"
  | "providers"
  | "profiles"
  | "rules"
  | "permissions"
  | "skills"
  | "mcp"
  | "environments"
  | "optimizers"
  | "usage"
  | "costs"
  | "keyboard"
  | "network"
  | "data";

export interface HelpTopic {
  slug: string;
  title: string;
  // Where the topic leads: buttons under the page.
  opens: { label: string; place: HelpPlace }[];
}

// The topics in reading order: the first steps, then what a session is made of,
// then the settings, then what to do when something goes wrong.
export const HELP_TOPICS: HelpTopic[] = [
  { slug: "getting-started", title: "Getting started", opens: [{ label: "Try the demo", place: "demo" }, { label: "Open Providers", place: "providers" }] },
  { slug: "sessions", title: "Sessions and panes", opens: [{ label: "New session", place: "new" }, { label: "Open the Board", place: "board" }, { label: "Open the Queue", place: "queue" }] },
  { slug: "agents-and-profiles", title: "Agents and profiles", opens: [{ label: "Open Profiles", place: "profiles" }] },
  { slug: "permissions", title: "Permissions and the policy", opens: [{ label: "Open Policy", place: "rules" }, { label: "Open Permissions settings", place: "permissions" }] },
  { slug: "providers", title: "Providers and models", opens: [{ label: "Open Providers", place: "providers" }] },
  { slug: "skills-and-mcp", title: "Skills and MCP servers", opens: [{ label: "Open Skills", place: "skills" }, { label: "Open MCP servers", place: "mcp" }] },
  { slug: "environments", title: "Environments", opens: [{ label: "Open Environments", place: "environments" }] },
  { slug: "token-optimizers", title: "Token optimizers", opens: [{ label: "Open Token optimizers", place: "optimizers" }] },
  { slug: "usage-and-budgets", title: "Usage, prices and budgets", opens: [{ label: "Open Usage", place: "usage" }, { label: "Open Prices and budgets", place: "costs" }] },
  { slug: "keyboard", title: "Keyboard and the command palette", opens: [{ label: "Open Keyboard shortcuts", place: "keyboard" }] },
  { slug: "privacy", title: "Privacy and your data", opens: [{ label: "Open Network settings", place: "network" }, { label: "Open Data settings", place: "data" }] },
  { slug: "troubleshooting", title: "Troubleshooting", opens: [{ label: "Report a problem", place: "data" }] },
];

// A search result: the topic, and a line of its text around the first match.
export interface HelpHit {
  slug: string;
  title: string;
  snippet: string;
}

// plain turns a page's Markdown into text to search and quote: no markup, one line.
export function plain(markdown: string): string {
  return markdown
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/^#+\s*/gm, "")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/[*_`>|]/g, "")
    .replace(/^\s*[-+]\s+/gm, "")
    .replace(/\s+/g, " ")
    .trim();
}

// searchHelp finds the topics whose title or text holds every word of query, title
// matches first, each with a snippet around the first word found in the text. pages
// maps a slug to its Markdown; topics missing a page are left out.
export function searchHelp(topics: readonly HelpTopic[], pages: Readonly<Record<string, string>>, query: string): HelpHit[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return [];
  const hits: (HelpHit & { rank: number; order: number })[] = [];
  topics.forEach((t, order) => {
    const page = pages[t.slug];
    if (page === undefined) return;
    const text = plain(page);
    const lower = text.toLowerCase();
    const title = t.title.toLowerCase();
    if (!words.every((w) => title.includes(w) || lower.includes(w))) return;
    const inTitle = words.every((w) => title.includes(w));
    const at = Math.max(0, ...words.map((w) => lower.indexOf(w)).filter((i) => i >= 0).slice(0, 1));
    const start = Math.max(0, at - 60);
    const end = Math.min(text.length, at + 100);
    const snippet = (start > 0 ? "…" : "") + text.slice(start, end).trim() + (end < text.length ? "…" : "");
    hits.push({ slug: t.slug, title: t.title, snippet, rank: inTitle ? 0 : 1, order });
  });
  hits.sort((a, b) => a.rank - b.rank || a.order - b.order);
  return hits.map(({ slug, title, snippet }) => ({ slug, title, snippet }));
}
