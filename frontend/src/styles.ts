// The user's saved styles (GUI-053): each a name, the colours of both
// appearances and the two fonts. Choosing one copies its values over the colours
// and fonts in force, so colors.json and config.yaml stay what the window shows;
// a style is "changed" when the look on screen is no longer what it holds. Kept
// apart from the components so it can be tested.
import type { ColorMap } from "./colors.ts";
import type { FontChoice } from "./fonts.ts";

type Entries = { [token: string]: string | undefined } | null | undefined;

// A style as the engine sends it; the bindings type a map's entries as optional.
export interface StyleValues {
  id: string;
  name: string;
  textFont: string;
  codeFont: string;
  light: Entries;
  dark: Entries;
}

export interface Look {
  light: ColorMap;
  dark: ColorMap;
  textFont: string;
  codeFont: string;
}

function colorMap(m: Entries): ColorMap {
  const out: ColorMap = {};
  for (const [k, v] of Object.entries(m ?? {})) if (v) out[k] = v;
  return out;
}

// lookOf is the look on screen: the colours in force and the fonts in config.ui.
export function lookOf(colors: { light?: Entries; dark?: Entries }, ui: { textFont?: string; codeFont?: string }): Look {
  return { light: colorMap(colors.light), dark: colorMap(colors.dark), textFont: ui.textFont ?? "", codeFont: ui.codeFont ?? "" };
}

function sameMap(a: ColorMap, b: ColorMap): boolean {
  const keys = Object.keys(a);
  return keys.length === Object.keys(b).length && keys.every((k) => a[k] === b[k]);
}

export function sameLook(a: Look, b: Look): boolean {
  return a.textFont === b.textFont && a.codeFont === b.codeFont && sameMap(a.light, b.light) && sameMap(a.dark, b.dark);
}

// selectedStyle is the style ui.style names, or null for the app's own look,
// also when its file is gone.
export function selectedStyle<T extends { id: string }>(styles: T[] | null | undefined, id: string | undefined): T | null {
  return (id && styles?.find((s) => s.id === id)) || null;
}

export function styleColors(s: StyleValues | null): { light: ColorMap; dark: ColorMap } {
  return { light: colorMap(s?.light), dark: colorMap(s?.dark) };
}

export function styleFonts(s: StyleValues | null): FontChoice {
  return { textFont: s?.textFont ?? "", codeFont: s?.codeFont ?? "" };
}

// isChanged says whether the look on screen differs from the style chosen, or,
// with none chosen, from the app's own look.
export function isChanged(look: Look, style: StyleValues | null): boolean {
  return !sameLook(look, lookOf(styleColors(style), styleFonts(style)));
}
