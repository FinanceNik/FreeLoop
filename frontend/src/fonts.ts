// The user's fonts (GUI-025), from config.yaml or the editor in Settings. Kept
// apart from the components so it can be tested.

export interface FontChoice {
  textFont: string;
  codeFont: string;
}

// What the editor offers, as far as they are on this Mac: what macOS comes with,
// then fonts people commonly install. Any other name can be typed.
export const TEXT_FONTS = [
  "Helvetica Neue",
  "Avenir Next",
  "Arial",
  "Verdana",
  "Gill Sans",
  "Optima",
  "Charter",
  "Georgia",
  "Iowan Old Style",
  "Palatino",
  "Inter",
  "IBM Plex Sans",
  "Manrope",
  "Open Sans",
  "Roboto",
  "Source Sans 3",
];

export const CODE_FONTS = [
  "Menlo",
  "Monaco",
  "SF Mono",
  "Courier New",
  "Andale Mono",
  "PT Mono",
  "Cascadia Code",
  "Fira Code",
  "Hack",
  "IBM Plex Mono",
  "Inconsolata",
  "Iosevka",
  "JetBrains Mono",
  "Source Code Pro",
  "Ubuntu Mono",
  "Victor Mono",
];

// The same check as config.ValidFontName. A name only reaches the page after
// this, so nothing but a family name gets in.
export function isFontName(v: string): boolean {
  return [...v].length <= 64 && /^[\p{L}\p{N}](?:[\p{L}\p{N}\p{M} ._+-]*[\p{L}\p{N}\p{M}._+-])?$/u.test(v);
}

// fontStack is the value for a font's variable: the name, then the app's own
// fonts for a Mac without it. Null when there is no name to use.
export function fontStack(name: string | undefined, fallback: string): string | null {
  return name && isFontName(name) ? `"${name}", var(--${fallback})` : null;
}

// applyFonts puts the fonts on the page. Without a name the variable goes back
// to what app.css gives it.
export function applyFonts(style: Pick<CSSStyleDeclaration, "setProperty" | "removeProperty">, fonts: Partial<FontChoice>) {
  for (const [property, name, fallback] of [
    ["--font-text", fonts.textFont, "font-text-default"],
    ["--mono", fonts.codeFont, "mono-default"],
  ] as const) {
    const stack = fontStack(name, fallback);
    if (stack) style.setProperty(property, stack);
    else style.removeProperty(property);
  }
}

const GENERIC = ["monospace", "serif", "sans-serif"];

// fontPresent tells whether a family is on this Mac: a page cannot list fonts
// or ask, but text in a missing family measures the same as its fallback, for
// every fallback. measure gives the width of a sample in a CSS font. It says
// yes when nothing can be measured, so a name is never held back on a guess.
export function fontPresent(name: string, measure: (font: string) => number): boolean {
  if (!measure(`72px ${GENERIC[0]}`)) return true;
  return GENERIC.some((g) => measure(`72px "${name}", ${g}`) !== measure(`72px ${g}`));
}

// fontOnThisMac is fontPresent on the page. It measures text in the page, not
// on a canvas: WebKit's canvas does not see fonts the user installed, though
// the page draws them.
export function fontOnThisMac(name: string): boolean {
  if (!isFontName(name)) return false;
  const probe = document.createElement("span");
  probe.textContent = "mmmmmmmmmmlli 0123456789 WwQq{}";
  probe.setAttribute("aria-hidden", "true");
  probe.style.cssText = "position: absolute; visibility: hidden; white-space: nowrap";
  document.body.append(probe);
  const present = fontPresent(name, (font) => {
    probe.style.font = font;
    return probe.getBoundingClientRect().width;
  });
  probe.remove();
  return present;
}
