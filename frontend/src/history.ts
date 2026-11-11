// Earlier prompts (GUI-049): what the user typed and sent, for the message box to
// complete from, recall with ↑ and search. Only the box records here, so a stage's
// expanded prompt, a queued task or anything the engine sends by itself is not
// history. Kept as drafts are (drafts.ts): in the page's storage, redacted by the
// engine first, and removed with the session it was typed in. It imports nothing, so
// `node --test` can run it.

export interface PastPrompt {
  text: string;
  sessionId: string;
  projectRoot: string;
  at: number;
}

const KEY = "freeloop.history";
export const MAX_ENTRIES = 500;
// A longer prompt, a pasted log say, is left out rather than cut short.
export const MAX_TEXT = 4 * 1024;

type HistoryStorage = Pick<Storage, "getItem" | "setItem">;

let storage: HistoryStorage | null = null;
function store(): HistoryStorage | null {
  if (storage) return storage;
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}

// Newest first; null until read from storage.
let entries: PastPrompt[] | null = null;
const listeners = new Set<() => void>();
const changed = () => listeners.forEach((f) => f());

// useHistoryStorage replaces the page's storage, for tests; what was read goes too.
export function useHistoryStorage(s: HistoryStorage | null) {
  storage = s;
  entries = null;
}

function valid(x: unknown): x is PastPrompt {
  const p = x as PastPrompt;
  return !!p && typeof p.text === "string" && typeof p.sessionId === "string" && typeof p.projectRoot === "string" && typeof p.at === "number";
}

function read(): PastPrompt[] {
  try {
    const raw = store()?.getItem(KEY);
    const list: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(list) ? list.filter(valid).slice(0, MAX_ENTRIES) : [];
  } catch {
    return [];
  }
}

function write(list: PastPrompt[]) {
  entries = list;
  try {
    store()?.setItem(KEY, JSON.stringify(list));
  } catch {
    // Kept in memory for now.
  }
  changed();
}

// historyEntries is every earlier prompt, newest first. The same list comes back until
// it changes, for React's useSyncExternalStore.
export function historyEntries(): readonly PastPrompt[] {
  if (entries === null) entries = read();
  return entries;
}

// subscribeHistory calls f when the history changes, here or in another window.
export function subscribeHistory(f: () => void): () => void {
  listeners.add(f);
  return () => listeners.delete(f);
}

// Another window of the app wrote it.
try {
  globalThis.addEventListener?.("storage", (e: StorageEvent) => {
    if (e.key !== KEY && e.key !== null) return;
    entries = null;
    changed();
  });
} catch {
  // No window: tests.
}

const tooLong = (text: string) => new TextEncoder().encode(text).length > MAX_TEXT;

// rememberPrompt records a sent prompt, redacted. If redacting fails, nothing is
// recorded. The same text sent again in the same project moves to the top.
export async function rememberPrompt(p: { text: string; sessionId: string; projectRoot: string }, redact: (s: string) => Promise<string>, now = Date.now()) {
  const typed = p.text.trim();
  if (!typed || tooLong(typed)) return;
  let text: string;
  try {
    text = (await redact(typed)).trim();
  } catch {
    return;
  }
  if (!text || tooLong(text)) return;
  // Read again: another window may have added to it.
  const list = read().filter((x) => !(x.text === text && x.projectRoot === p.projectRoot));
  write([{ text, sessionId: p.sessionId, projectRoot: p.projectRoot, at: now }, ...list].slice(0, MAX_ENTRIES));
}

// pruneHistory removes what was typed in sessions that no longer exist.
export function pruneHistory(exists: (sid: string) => boolean) {
  const list = historyEntries();
  const kept = list.filter((x) => exists(x.sessionId));
  if (kept.length !== list.length) write(kept);
}

// suggestion is the rest of the newest earlier prompt of this project that starts with
// the draft, or "" for none. A draft of fewer than two characters, or of more than one
// line, gets none.
export function suggestion(draft: string, list: readonly PastPrompt[], root: string): string {
  if (draft.includes("\n") || draft.replace(/\s/g, "").length < 2) return "";
  for (const x of list) {
    if (x.projectRoot === root && x.text.length > draft.length && x.text.startsWith(draft)) return x.text.slice(draft.length);
  }
  return "";
}

// nextWord is the part of a suggestion's rest that one word takes: the spaces before
// the next word, and the word.
export function nextWord(rest: string): string {
  return rest.match(/^\s*\S+/)?.[0] ?? rest;
}

// recallList is what ↑ steps through: this project's earlier prompts that start with
// prefix and are more than it, newest first, each once.
export function recallList(list: readonly PastPrompt[], root: string, prefix: string): string[] {
  const out: string[] = [];
  for (const x of list) {
    if (x.projectRoot !== root || x.text === prefix || !x.text.startsWith(prefix) || out.includes(x.text)) continue;
    out.push(x.text);
  }
  return out;
}

// searchPrompts finds the earlier prompts holding every word of query, whatever the
// case: this project's first, then other projects', newest first in each.
export function searchPrompts(query: string, list: readonly PastPrompt[], root: string, limit = 8): PastPrompt[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  const hits = list.filter((x) => {
    const text = x.text.toLowerCase();
    return words.every((w) => text.includes(w));
  });
  return [...hits.filter((x) => x.projectRoot === root), ...hits.filter((x) => x.projectRoot !== root)].slice(0, limit);
}

// onFirstLine and onLastLine say whether the caret can leave the text upward or
// downward, so ↑ and ↓ move it between a recalled prompt's lines first.
export function onFirstLine(text: string, caret: number): boolean {
  return !text.slice(0, caret).includes("\n");
}

export function onLastLine(text: string, caret: number): boolean {
  return !text.slice(caret).includes("\n");
}

// Right-to-left text is drawn in an order the grey suggestion would not follow.
export function rightToLeft(text: string): boolean {
  return /[֐-ࣿיִ-﷿ﹰ-﻿]/.test(text);
}
