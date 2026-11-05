// What is typed in a session's composer stays until it is sent or deleted (SES-023). A
// view that goes away, for Settings say, takes its state with it, so the text and what is
// attached are kept here. The text also goes to the page's storage, for a restart, redacted
// by the engine as transcripts are: a key pasted into a draft must not reach the disk.
// Attachments stay in memory only: pictures are large, and files may be anything.

const PREFIX = "freeloop.draft.";
// Longer than this stays in memory only, so one huge paste cannot fill the storage.
const MAX_STORED = 64 * 1024;

type DraftStorage = Pick<Storage, "getItem" | "setItem" | "removeItem" | "key" | "length">;

let storage: DraftStorage | null = null;
function store(): DraftStorage | null {
  if (storage) return storage;
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}

// useDraftStorage replaces the page's storage, for tests.
export function useDraftStorage(s: DraftStorage | null) {
  storage = s;
}

const texts = new Map<string, string>();
const extras = new Map<string, unknown>();

const attempt = (act: (s: DraftStorage) => void) => {
  try {
    const s = store();
    if (s) act(s);
  } catch {
    // Only for a restart: it is still kept in memory.
  }
};

// draftOf returns what was being typed in a session.
export function draftOf(sid: string): string {
  const kept = texts.get(sid);
  if (kept !== undefined) return kept;
  try {
    return store()?.getItem(PREFIX + sid) ?? "";
  } catch {
    return "";
  }
}

// rememberDraft keeps the text in memory. An empty one is deleted, from storage as well.
export function rememberDraft(sid: string, text: string) {
  if (text) {
    texts.set(sid, text);
    return;
  }
  texts.set(sid, "");
  attempt((s) => s.removeItem(PREFIX + sid));
}

// storeDraft writes the text for a restart, if it is still the text once redacted.
export async function storeDraft(sid: string, text: string, redact: (s: string) => Promise<string>) {
  if (!text || text.length > MAX_STORED) {
    if (text) attempt((s) => s.removeItem(PREFIX + sid));
    return;
  }
  let safe: string;
  try {
    safe = await redact(text);
  } catch {
    return; // Not redacted means not written.
  }
  if (texts.get(sid) === text) attempt((s) => s.setItem(PREFIX + sid, safe));
}

export function extrasOf<T>(sid: string): T | undefined {
  return extras.get(sid) as T | undefined;
}

export function rememberExtras(sid: string, value: unknown) {
  extras.set(sid, value);
}

// forgetDraft is for a message sent or a session deleted.
export function forgetDraft(sid: string) {
  rememberDraft(sid, "");
  extras.delete(sid);
}

// pruneDrafts removes what is stored for sessions that no longer exist.
export function pruneDrafts(exists: (sid: string) => boolean) {
  attempt((s) => {
    const gone: string[] = [];
    for (let i = 0; i < s.length; i++) {
      const key = s.key(i);
      if (key?.startsWith(PREFIX) && !exists(key.slice(PREFIX.length))) gone.push(key);
    }
    gone.forEach((key) => s.removeItem(key));
  });
}
