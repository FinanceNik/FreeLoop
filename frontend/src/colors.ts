// The user's colors over the look's tokens (GUI-022), from colors.json or the
// editor in Settings. Kept apart from the components so it can be tested.
import { numberFormat, t, type PlainKey } from "./i18n.ts";

export type ColorMode = "light" | "dark";
export type ColorMap = Record<string, string>;
export interface ColorSet {
  light?: ColorMap | null;
  dark?: ColorMap | null;
}

// What the editor offers; colors.json can also set the tokens these derive
// from below, and the chart series.
// Titles and labels are catalogue keys (GUI-048).
export const COLOR_GROUPS: { title: PlainKey; tokens: { id: string; label: PlainKey }[] }[] = [
  {
    title: "colors.group.backgrounds",
    tokens: [
      { id: "bg-app", label: "colors.token.bgApp" },
      { id: "bg", label: "colors.token.bg" },
      { id: "surface", label: "colors.token.surface" },
      { id: "surface-2", label: "colors.token.surface2" },
      { id: "input", label: "colors.token.input" },
    ],
  },
  {
    title: "colors.group.text",
    tokens: [
      { id: "text", label: "colors.token.text" },
      { id: "muted", label: "colors.token.muted" },
      { id: "faint", label: "colors.token.faint" },
      { id: "border", label: "colors.token.border" },
      { id: "border-strong", label: "colors.token.borderStrong" },
      { id: "input-border", label: "colors.token.inputBorder" },
    ],
  },
  {
    title: "colors.group.accent",
    tokens: [
      { id: "accent", label: "colors.token.accent" },
      { id: "accent-text", label: "colors.token.accentText" },
      { id: "link", label: "colors.token.link" },
      { id: "focus", label: "colors.token.focus" },
    ],
  },
  {
    title: "colors.group.status",
    tokens: [
      { id: "ok", label: "colors.token.ok" },
      { id: "warn", label: "colors.token.warn" },
      { id: "danger", label: "colors.token.danger" },
    ],
  },
  {
    title: "colors.group.marks",
    tokens: [{ id: "git", label: "colors.token.git" }],
  },
];

// The same check as config.ValidColor. A value only reaches the style sheet
// after this, so nothing but a color gets in.
export function isColor(v: string): boolean {
  return /^(#[0-9a-fA-F]{3,4}|#[0-9a-fA-F]{6}|#[0-9a-fA-F]{8}|rgba?\([0-9., %/]+\))$/.test(v);
}

// Tokens that follow another one unless set themselves, so changing the
// accent does not leave blue hovers and blue links behind.
function derived(c: ColorMap, mode: ColorMode): ColorMap {
  const lift = (token: string, pct: number) => `color-mix(in srgb, var(--${token}) ${pct}%, white)`;
  const soft = (token: string, pct: number) => `color-mix(in srgb, var(--${token}) ${pct}%, transparent)`;
  const out: ColorMap = {};
  if (c.accent) {
    out["accent-hover"] = "color-mix(in srgb, var(--accent) 88%, black)";
    out["accent-soft"] = soft("accent", 14);
    out.link = mode === "dark" ? lift("accent", 65) : "var(--accent)";
    out.focus = out.link;
  }
  if (c.danger) {
    out["danger-soft"] = soft("danger", 10);
    out.del = soft("danger", 12);
  }
  if (c.ok) out.add = soft("ok", 13);
  return out;
}

function declarations(c: ColorMap, mode: ColorMode): string {
  const set: ColorMap = {};
  for (const [k, v] of Object.entries(c)) {
    if (/^[a-z0-9-]+$/.test(k) && isColor(v)) set[k] = v;
  }
  return Object.entries({ ...derived(set, mode), ...set })
    .map(([k, v]) => `--${k}: ${v};`)
    .join(" ");
}

// colorSheet is the style sheet for a set of colors. Its selectors match the
// ones in app.css, and it comes after that file, so it wins.
export function colorSheet(colors: ColorSet): string {
  const light = declarations(colors.light ?? {}, "light");
  const dark = declarations(colors.dark ?? {}, "dark");
  const rules: string[] = [];
  if (light) rules.push(`:root { ${light} }`);
  if (dark) {
    rules.push(`@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { ${dark} } }`);
    rules.push(`:root[data-theme="dark"] { ${dark} }`);
  }
  return rules.join("\n");
}

// pickerValue is the #rrggbb form a color input needs, or "" when v has none.
export function pickerValue(v: string): string {
  const s = v.trim();
  if (/^#[0-9a-fA-F]{6}$/.test(s)) return s.toLowerCase();
  if (/^#[0-9a-fA-F]{8}$/.test(s)) return s.slice(0, 7).toLowerCase();
  if (/^#[0-9a-fA-F]{3,4}$/.test(s)) return `#${s[1]}${s[1]}${s[2]}${s[2]}${s[3]}${s[3]}`.toLowerCase();
  const m = /^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)/.exec(s);
  if (!m) return "";
  return `#${m.slice(1, 4).map((n) => Math.min(Number(n), 255).toString(16).padStart(2, "0")).join("")}`;
}

// contrast is the WCAG contrast ratio of two colors, or null when one of them
// is not a color it can read.
export function contrast(a: string, b: string): number | null {
  const lum = (v: string) => {
    const hex = pickerValue(v);
    if (!hex) return null;
    const [r, g, b] = [1, 3, 5].map((i) => {
      const c = parseInt(hex.slice(i, i + 2), 16) / 255;
      return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const x = lum(a);
  const y = lum(b);
  if (x === null || y === null) return null;
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

// The pairs people read most, with what WCAG AA asks of body text.
const READABLE: [string, string, PlainKey][] = [
  ["text", "bg", "colors.pair.textOnConversation"],
  ["text", "bg-app", "colors.pair.textOnSidebar"],
  ["muted", "bg", "colors.pair.mutedOnConversation"],
  ["accent-text", "accent", "colors.token.accentText"],
];

// hardToRead names the pairs under 4.5:1, for colors read through get.
export function hardToRead(get: (token: string) => string): string[] {
  const out: string[] = [];
  for (const [fg, bg, label] of READABLE) {
    const ratio = contrast(get(fg), get(bg));
    if (ratio !== null && ratio < 4.5) out.push(`${t(label)}: ${numberFormat({ minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(ratio)}:1`);
  }
  return out;
}

let sheet: CSSStyleSheet | null = null;

// applyColors puts a set of colors on the page.
export function applyColors(colors: ColorSet) {
  if (!sheet) {
    sheet = new CSSStyleSheet();
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
  }
  sheet.replaceSync(colorSheet(colors));
}

// tokenDefaults reads the app's own colors for a mode from app.css, so the
// editor can show them for the mode that is not on screen. The sheet set by
// applyColors is not in document.styleSheets, so it does not get in the way.
export function tokenDefaults(mode: ColorMode): ColorMap {
  const selector = mode === "dark" ? ':root[data-theme="dark"]' : ":root";
  const out: ColorMap = {};
  for (const s of Array.from(document.styleSheets)) {
    let rules: CSSRuleList;
    try {
      rules = s.cssRules;
    } catch {
      continue;
    }
    for (const r of Array.from(rules)) {
      if (!(r instanceof CSSStyleRule) || r.selectorText !== selector) continue;
      for (const g of COLOR_GROUPS) {
        for (const t of g.tokens) {
          const v = r.style.getPropertyValue(`--${t.id}`).trim();
          if (v) out[t.id] = v;
        }
      }
    }
  }
  return out;
}
