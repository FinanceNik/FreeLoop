// The help pages (GUI-047): which topics there are, in what order, where each one
// leads in the app, and searching their text. The pages themselves are Markdown in
// src/help/, English there and German and Dutch in src/help/de/ and src/help/nl/
// (GUI-048), read by Help.tsx; this module imports only the catalogues, so `node
// --test` can run it.
import { t, type PlainKey } from "./i18n.ts";

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
  | "workflows"
  | "usage"
  | "costs"
  | "keyboard"
  | "voice"
  | "network"
  | "data";

// A topic in the language in force.
export interface HelpTopic {
  slug: string;
  title: string;
  // Where the topic leads: buttons under the page.
  opens: { label: string; place: HelpPlace }[];
}

// The topics in reading order: the first steps, then what a session is made of,
// then the settings, then what to do when something goes wrong.
const TOPICS: { slug: string; title: PlainKey; opens: { label: PlainKey; place: HelpPlace }[] }[] = [
  { slug: "getting-started", title: "help.topic.gettingStarted", opens: [{ label: "help.open.demo", place: "demo" }, { label: "help.open.providers", place: "providers" }] },
  { slug: "sessions", title: "help.topic.sessions", opens: [{ label: "help.open.new", place: "new" }, { label: "help.open.board", place: "board" }, { label: "help.open.queue", place: "queue" }] },
  { slug: "agents-and-profiles", title: "help.topic.agentsAndProfiles", opens: [{ label: "help.open.profiles", place: "profiles" }] },
  { slug: "permissions", title: "help.topic.permissions", opens: [{ label: "help.open.rules", place: "rules" }, { label: "help.open.permissions", place: "permissions" }] },
  { slug: "providers", title: "help.topic.providers", opens: [{ label: "help.open.providers", place: "providers" }] },
  { slug: "skills-and-mcp", title: "help.topic.skillsAndMcp", opens: [{ label: "help.open.skills", place: "skills" }, { label: "help.open.mcp", place: "mcp" }] },
  { slug: "environments", title: "help.topic.environments", opens: [{ label: "help.open.environments", place: "environments" }] },
  { slug: "token-optimizers", title: "help.topic.tokenOptimizers", opens: [{ label: "help.open.optimizers", place: "optimizers" }] },
  { slug: "workflows", title: "help.topic.workflows", opens: [{ label: "help.open.workflows", place: "workflows" }, { label: "help.open.board", place: "board" }, { label: "help.open.attention", place: "attention" }] },
  { slug: "usage-and-budgets", title: "help.topic.usageAndBudgets", opens: [{ label: "help.open.usage", place: "usage" }, { label: "help.open.costs", place: "costs" }] },
  { slug: "keyboard", title: "help.topic.keyboard", opens: [{ label: "help.open.keyboard", place: "keyboard" }] },
  { slug: "voice-input", title: "help.topic.voiceInput", opens: [{ label: "help.open.voice", place: "voice" }] },
  { slug: "privacy", title: "help.topic.privacy", opens: [{ label: "help.open.network", place: "network" }, { label: "help.open.data", place: "data" }] },
  { slug: "troubleshooting", title: "help.topic.troubleshooting", opens: [{ label: "help.open.report", place: "data" }] },
];

// The topics' slugs, in reading order.
export const HELP_SLUGS: readonly string[] = TOPICS.map((x) => x.slug);

// helpTopics is the topics with their titles and buttons in the language in force.
export function helpTopics(): HelpTopic[] {
  return TOPICS.map((x) => ({ slug: x.slug, title: t(x.title), opens: x.opens.map((o) => ({ label: t(o.label), place: o.place })) }));
}

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
