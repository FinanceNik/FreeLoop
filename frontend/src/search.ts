// Full-text search results (SES-007): the engine wraps matches in snippets
// with control characters, which become marked text here, never markup.
import { t, type PlainKey } from "./i18n.ts";

export const SNIPPET_OPEN = "\x02";
export const SNIPPET_CLOSE = "\x03";

export interface SnippetPart {
  text: string;
  mark: boolean;
}

// snippetParts splits a snippet into plain and matched runs; newlines and
// runs of spaces fold into one space.
export function snippetParts(snippet: string): SnippetPart[] {
  const parts: SnippetPart[] = [];
  let mark = false;
  let text = "";
  const flush = () => {
    if (text) parts.push({ text: text.replace(/\s+/g, " "), mark });
    text = "";
  };
  for (const ch of snippet) {
    if (ch === SNIPPET_OPEN || ch === SNIPPET_CLOSE) {
      flush();
      mark = ch === SNIPPET_OPEN;
    } else {
      text += ch;
    }
  }
  flush();
  return parts;
}

// itemForSeq returns the key of the conversation item a hit points at: the
// item that starts at seq, or else the last one starting before it.
export function itemForSeq(items: readonly { key: string }[], seq: number): string | null {
  let best: string | null = null;
  for (const it of items) {
    const k = Number(it.key);
    if (k === seq) return it.key;
    if (k < seq) best = it.key;
  }
  return best;
}

// Where a hit was found, by its kind.
const HIT_LABEL: Record<string, PlainKey> = {
  user: "search.hit.user",
  agent: "search.hit.agent",
  thought: "search.hit.thought",
  tool: "search.hit.tool",
  summary: "search.hit.summary",
};

// hitLabel names where a hit was found, in the language in force; a kind the app has
// no word for is shown as the engine gave it.
export function hitLabel(kind: string): string {
  return Object.hasOwn(HIT_LABEL, kind) ? t(HIT_LABEL[kind]) : kind;
}
