// What a session runs on, and the mark that shows it at a glance (GUI-034): an external
// agent if it has one, else its provider. Colour alone would fail anyone who cannot tell
// the colours apart, and vendor logos are not ours to use, so a mark is a colour and two
// letters. Pure, so source.test.ts covers it.
import { locale } from "./i18n.ts";

export const SLOTS = 8;

export type Named = { id: string; name: string };
type Runs = { harnessId?: string | null; providerId?: string | null };

export interface SourceChoice {
  slot?: number;
  letters?: string;
}
export type SourceChoices = Record<string, SourceChoice> | null | undefined;

export interface Source {
  id: string;
  name: string;
  letters: string;
  // 1 to SLOTS; 0 is the neutral colour, for what no longer exists.
  slot: number;
}

const external = (r: Runs) => !!r.harnessId && r.harnessId !== "builtin";

// sourceName is the one rule for what a session or a row of usage runs on.
export function sourceName(r: Runs, providers: readonly Named[], harnesses: readonly Named[]): string {
  return external(r)
    ? (harnesses.find((h) => h.id === r.harnessId)?.name ?? r.harnessId!)
    : (providers.find((p) => p.id === r.providerId)?.name ?? (r.providerId || "Unknown provider"));
}

export function sourceId(r: Runs): string {
  return (external(r) ? r.harnessId : r.providerId) || "";
}

// defaultLetters are a name's initials, or the start of one word.
export function defaultLetters(name: string): string {
  const words = name.split(/[\s._/-]+/u).filter(Boolean);
  const letters = words.length >= 2 ? words[0][0] + words[1][0] : Array.from(words[0] ?? "?").slice(0, 2).join("");
  return letters.toLocaleUpperCase(locale());
}

// validLetters is config.validSourceLetters: at most two letters or digits.
export function validLetters(s: string): boolean {
  return /^[\p{L}\p{N}]{1,2}$/u.test(s);
}

const validSlot = (n: unknown): n is number => typeof n === "number" && Number.isInteger(n) && n >= 1 && n <= SLOTS;

// A provider's own slot comes from its ID, so a colour does not depend on what else there
// is or how much it is used.
function hash(id: string): number {
  let h = 2166136261;
  for (const c of id) h = Math.imul(h ^ c.codePointAt(0)!, 16777619);
  return (h >>> 0) % SLOTS;
}

// assignSlots gives each ID a slot: the user's choice first, then its own, moving on to the
// next free one when taken, in the order of the IDs. With more IDs than slots, they share.
export function assignSlots(ids: readonly string[], choices: SourceChoices): Map<string, number> {
  const out = new Map<string, number>();
  const taken = new Set<number>();
  const sorted = [...new Set(ids)].filter(Boolean).sort();
  for (const id of sorted) {
    const chosen = choices?.[id]?.slot;
    if (validSlot(chosen)) {
      out.set(id, chosen);
      taken.add(chosen);
    }
  }
  for (const id of sorted) {
    if (out.has(id)) continue;
    let at = hash(id);
    for (let tried = 0; tried < SLOTS && taken.has(at + 1); tried++) at = (at + 1) % SLOTS;
    out.set(id, at + 1);
    taken.add(at + 1);
  }
  return out;
}

// sourceIndex makes the function the views ask. What no longer exists is neutral, and says
// its ID, since its name is gone with it. A list that has not arrived yet says nothing of
// the kind: the agents take a moment, each being looked for through the login shell, and a
// session must not look deleted until they have.
export function sourceIndex(providers: readonly Named[], harnesses: readonly Named[], sessions: readonly Runs[], choices: SourceChoices): (r: Runs) => Source {
  const known = [...providers.map((p) => p.id), ...harnesses.filter((h) => h.id !== "builtin").map((h) => h.id)];
  const waiting = sessions.filter((r) => (external(r) ? harnesses.length === 0 : providers.length === 0)).map(sourceId);
  const slots = assignSlots([...known, ...waiting], choices);
  return (r) => {
    const id = sourceId(r);
    const name = sourceName(r, providers, harnesses);
    const chosen = choices?.[id]?.letters;
    return { id, name, letters: chosen && validLetters(chosen) ? chosen : defaultLetters(name), slot: slots.get(id) ?? 0 };
  };
}

// sourceColor is a slot's colour, as a style sheet value.
export function sourceColor(slot: number): string {
  return slot >= 1 && slot <= SLOTS ? `var(--source-${slot})` : "var(--source-none)";
}
