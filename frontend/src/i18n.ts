// The interface's language (GUI-048): which one is in force, and the text for a key in
// it. English (src/i18n/en.ts) is the source; the German and Dutch catalogues are
// typed against it, so a key or a {placeholder} out of step fails the type check.
// Text from models, agents and files is never looked up here: it is shown as given.
// No runtime imports but the catalogues, so `node --test` can run it.
import type { Forms, Holes, Plural } from "./i18n/catalogue.ts";
import { en } from "./i18n/en.ts";
import { de } from "./i18n/de.ts";
import { nl } from "./i18n/nl.ts";

export type Language = "en" | "de" | "nl";

// The picker's choices besides "system", each named in itself.
export const LANGUAGES: readonly { id: Language; name: string }[] = [
  { id: "en", name: "English" },
  { id: "de", name: "Deutsch" },
  { id: "nl", name: "Nederlands" },
];

export const CATALOGUES = { en, de, nl } as const;

type English = typeof en;
export type Key = keyof English & string;

type Text<K extends Key> = Forms<English[K]>;
type IsPlural<K extends Key> = English[K] extends string ? false : true;

// A key whose text needs no values, as a table of labels holds.
export type PlainKey = { [K in Key]: [Holes<Text<K>>] extends [never] ? (IsPlural<K> extends true ? never : K) : never }[Key];

// The values a key's text needs: one per placeholder, and a count for plural forms.
export type Vars<K extends Key, V = never> = { readonly [P in Holes<Text<K>>]: P extends "count" ? number : string | number | V } & (IsPlural<K> extends true
  ? { readonly count: number }
  : unknown);

type Args<K extends Key, V = never> = [Holes<Text<K>>] extends [never] ? (IsPlural<K> extends true ? [vars: Vars<K, V>] : []) : [vars: Vars<K, V>];

function isLanguage(s: string): s is Language {
  return s === "en" || s === "de" || s === "nl";
}

// resolveLanguage is the language for a setting of ui.language: the one chosen, or for
// "system" (and anything else) the first of the computer's preferred languages the app
// has, as a Mac app picks its localization; English when it has none of them.
export function resolveLanguage(setting: string | null | undefined, preferred: readonly string[]): Language {
  if (setting && isLanguage(setting)) return setting;
  for (const tag of preferred) {
    const base = tag.toLowerCase().split(/[-_]/)[0];
    if (isLanguage(base)) return base;
  }
  return "en";
}

// preferredLanguages are the languages the system prefers, as the web view reports them.
export function preferredLanguages(): readonly string[] {
  if (typeof navigator === "undefined") return [];
  return navigator.languages?.length ? navigator.languages : navigator.language ? [navigator.language] : [];
}

let current: Language = "en";
const plurals = new Map<Language, Intl.PluralRules>();

export function setLanguage(lang: Language) {
  current = lang;
}

export function language(): Language {
  return current;
}

function entry(key: Key, count: unknown): string {
  const e = (CATALOGUES[current] as Record<string, string | Plural>)[key] ?? (en as Record<string, string | Plural>)[key] ?? key;
  if (typeof e === "string") return e;
  if (typeof count !== "number") return e.other;
  let rules = plurals.get(current);
  if (!rules) plurals.set(current, (rules = new Intl.PluralRules(current)));
  return e[rules.select(count)] ?? e.other;
}

// tParts is a key's text in pieces, with each placeholder's value in its place, for
// values that are not text, such as an element. A placeholder without a value stays.
export function tParts<K extends Key, V>(key: K, ...args: Args<K, V>): (string | V)[] {
  const vars = (args[0] ?? {}) as Record<string, string | number | V>;
  const out: (string | V)[] = [];
  let text = "";
  for (const piece of entry(key, vars.count).split(/(\{\w+\})/)) {
    const name = /^\{(\w+)\}$/.exec(piece)?.[1];
    const value = name !== undefined && Object.hasOwn(vars, name) ? vars[name] : piece;
    if (typeof value === "string" || typeof value === "number") text += String(value);
    else {
      if (text) out.push(text);
      out.push(value);
      text = "";
    }
  }
  if (text || out.length === 0) out.push(text);
  return out;
}

// t is a key's text in the language in force, its placeholders filled.
export function t<K extends Key>(key: K, ...args: Args<K>): string {
  return tParts(key, ...args).join("");
}

// list joins names into one: "A, B and C".
export function list(items: readonly string[]): string {
  if (items.length < 2) return items[0] ?? "";
  return t("common.and", { rest: items.slice(0, -1).join(", "), last: items[items.length - 1] });
}
