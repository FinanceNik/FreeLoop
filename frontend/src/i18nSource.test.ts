import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

// GUI-048: the Settings page and what it is made of write no text of their own; all of
// it comes from the catalogues in src/i18n/. A rough reading of the source, not a
// parser: text between tags, text on a line of its own, words in a user-facing
// attribute, and string literals that read like a label or a sentence.
const SETTINGS_PAGE = [
  "Settings.tsx",
  "SettingsNav.tsx",
  "ShortcutsEditor.tsx",
  "ColorEditor.tsx",
  "SourceMarks.tsx",
  "SourceMark.tsx",
  "FontEditor.tsx",
  "Costs.tsx",
  "BudgetTable.tsx",
  "Hooks.tsx",
];

// What is not text to translate: a selector, a media query, a key's name.
const NOT_TEXT = new Set(['input:not([type="checkbox"]), textarea', "(prefers-color-scheme: dark)", "Tab", "Escape"]);

// A line of code that holds none of < > { } = and so could pass for text.
const CODE_LINE = /^(\.|\?|:|&&|\|\||\)|\[|\]|\w+\??: |return\b|case\b|default\b|const\b|let\b|if\b|else\b|export\b|import\b|type\b|interface\b|function\b|await\b|throw\b|break\b|continue\b|delete\b)|[({[]$|^[\w.]+\(|^[a-z][A-Za-z]*[,;]?$/;

// A capitalized word on its own or starting a phrase, and not a key such as Meta+1.
const LABEL = /^[A-Z][a-z']+(?:$|[\s,.?!:…])/;

function untranslated(source: string): string[] {
  const code = source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|\s)\/\/.*$/gm, "$1");
  const found: string[] = [];
  const words = (s: string) => /[A-Za-z]{2,}/.test(s);
  // Text between tags on one line, or after an expression up to a closing tag.
  for (const m of code.matchAll(/[>}]([^<>{}\n]*)(?=<\/)/g)) if (words(m[1])) found.push(m[1].trim());
  // A line of text on its own, as between an opening and a closing tag.
  for (const line of code.split("\n")) {
    const s = line.trim();
    if (words(s) && !/[<>{}="`]/.test(s) && !CODE_LINE.test(s)) found.push(s);
  }
  // A user-facing attribute with words in it.
  for (const m of code.matchAll(/\b(?:aria-label|title|placeholder|label|hint|alt)="([^"]*)"/g)) if (words(m[1])) found.push(m[1]);
  // A string that starts like a label or reads like a sentence.
  for (const m of code.matchAll(/"((?:[^"\\\n]|\\.)*)"|'((?:[^'\\\n]|\\.)*)'/g)) {
    const s = m[1] ?? m[2];
    if (NOT_TEXT.has(s)) continue;
    if (LABEL.test(s) || (/\s/.test(s) && /[,.?!:…]/.test(s) && words(s))) found.push(s);
  }
  for (const m of code.matchAll(/`([^`]*)`/g)) {
    const s = m[1].replace(/\$\{[^}]*\}/g, "");
    if (words(s) && (/\s/.test(s.trim()) || LABEL.test(s))) found.push(m[1]);
  }
  return [...new Set(found)];
}

test("the source reading finds English written into a component", () => {
  assert.deepEqual(untranslated('<h4>Restore this backup?</h4>'), ["Restore this backup?"]);
  assert.deepEqual(untranslated("<button>\n  Cancel\n</button>"), ["Cancel"]);
  assert.deepEqual(untranslated('<input aria-label="Model" />'), ["Model"]);
  assert.deepEqual(untranslated('const x = ok ? "Checking…" : t("a");'), ["Checking…"]);
  assert.deepEqual(untranslated("const name = `Provider: ${source.name}`;"), ["Provider: ${source.name}"]);
  assert.deepEqual(untranslated('<span className="row settings-save">{t("a")}</span>\n  required\n  const [a, b] = useState<string | null>(null);'), []);
});

test("every string on the Settings page comes from the catalogues", () => {
  for (const name of SETTINGS_PAGE) {
    const source = readFileSync(new URL(`./${name}`, import.meta.url), "utf8");
    assert.deepEqual(untranslated(source), [], name);
  }
});
