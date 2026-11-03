// What the attention view needs to know about requests (GUI-002, GUI-006). Pure,
// so attentionview.test.ts covers it.

type Timed = { requestId: string; createdAt?: string | Date };

// oldestFirst lists every waiting request across sessions, longest wait first.
export function oldestFirst<T extends Timed>(perSession: readonly (readonly T[] | null | undefined)[]): T[] {
  const time = (r: T) => new Date(r.createdAt ?? 0).getTime() || 0;
  return perSession.flatMap((rs) => rs ?? []).sort((a, b) => time(a) - time(b));
}

// currentOf is the request the keys act on: the one chosen while it still waits,
// else the longest wait, so answering one moves on without losing the place.
export function currentOf(list: readonly Timed[], chosen: string | null): string | null {
  if (chosen && list.some((r) => r.requestId === chosen)) return chosen;
  return list[0]?.requestId ?? null;
}

export function moved(list: readonly Timed[], current: string | null, by: number): string | null {
  if (list.length === 0) return null;
  const i = list.findIndex((r) => r.requestId === current);
  return list[Math.min(list.length - 1, Math.max(0, (i < 0 ? 0 : i) + by))].requestId;
}

export type EditPreview = { oldText: string; newText: string };

// editPreview finds the change in a tool call's input, in the spellings the
// built-in harness and agents use, so an edit can be read before it is allowed.
export function editPreview(input: unknown): EditPreview | null {
  if (!input || typeof input !== "object") return null;
  const o = input as Record<string, unknown>;
  const text = (...names: string[]) => {
    for (const n of names) if (typeof o[n] === "string") return o[n] as string;
    return null;
  };
  const after = text("new_string", "new_text", "newText", "newString");
  const before = text("old_string", "old_text", "oldText", "oldString");
  if (after !== null && before !== null) return { oldText: before, newText: after };
  const written = text("content", "contents");
  return written !== null ? { oldText: "", newText: written } : null;
}
