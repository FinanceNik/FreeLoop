// What a catalogue is, and the check that holds a translation to its source (GUI-048):
// the same keys, text where the source has text and plural forms where it has those,
// and the same {placeholders}. A mistake fails the type check, at the entry.

// An entry's plural forms, by the categories of Intl.PluralRules. Every language has
// "other"; English, German and Dutch use "one" too.
export interface Plural {
  readonly zero?: string;
  readonly one?: string;
  readonly two?: string;
  readonly few?: string;
  readonly many?: string;
  readonly other: string;
}

export type Entry = string | Plural;

export type Source = { readonly [key: string]: Entry };

// The text of every form of an entry.
export type Forms<E> = E extends string ? E : { [F in keyof E]-?: E[F] }[keyof E] & string;

// The names of the {placeholders} in a text.
export type Holes<S> = S extends `${string}{${infer P}}${infer Rest}` ? P | Holes<Rest> : never;

type Same<A, B> = [A] extends [B] ? ([B] extends [A] ? true : false) : false;

type Shape<E> = E extends string ? string : Plural;

type Fits<E, V> = E extends string ? (V extends string ? true : false) : V extends string ? false : V extends Plural ? true : false;

// Checked is what a translation T of source E must be: each entry as it is when its
// shape and placeholders match, and a type naming the placeholders when they do not.
export type Checked<E extends Source, T> = {
  readonly [K in keyof E]: K extends keyof T
    ? Fits<E[K], T[K]> extends true
      ? Same<Holes<Forms<E[K]>>, Holes<Forms<T[K]>>> extends true
        ? T[K]
        : { placeholders: Holes<Forms<E[K]>> }
      : Shape<E[K]>
    : Shape<E[K]>;
} & { readonly [K in Exclude<keyof T, keyof E>]: never };

// translationOf<E>() returns the function a translation of E is written through.
export function translationOf<E extends Source>() {
  return <const T extends Checked<E, T>>(c: T): { readonly [K in keyof E]: Entry } => c as unknown as { readonly [K in keyof E]: Entry };
}
