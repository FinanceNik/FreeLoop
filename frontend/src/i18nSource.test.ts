import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { test } from "node:test";

// GUI-048: the interface writes no text of its own; all of it comes from the
// catalogues in src/i18n/. Every source file is read, but for these, which hold no
// interface text: a rough reading of the source, not a parser, finds text between
// tags, text on a line of its own, words in a user-facing attribute, and string
// literals that read like a label or a sentence.
const EXEMPT: Record<string, string> = {
  "colors.ts": "CSS it writes",
  "fonts.ts": "fonts' names",
  "i18n.ts": "each language's name in that language",
  "icons.tsx": "SVG paths",
  "longfixture.ts": "the made-up long session the timings run on (issue 040), words for filler",
  "markdown.ts": "the Markdown parser",
  "pdfview.ts": "the PDF frame's page, whose one word is PDF",
  "platform.ts": "systems' names and their file managers'",
  "review.ts": "the message a review sends the agent, which stays English",
};

// SOURCES are every component and module but the tests and the exempt ones.
const SOURCES = readdirSync(new URL("./", import.meta.url)).filter((f) => /\.tsx?$/.test(f) && !/\.(test|typetest|d)\.ts$/.test(f) && !(f in EXEMPT));

// What is not text to translate: a selector, a media query, the product's name, a key's
// name as a key event gives it.
const NOT_TEXT = new Set([
  'input:not([type="checkbox"]), textarea',
  "(prefers-color-scheme: dark)",
  "(prefers-reduced-motion: reduce)",
  "Freeloop",
  // Examples in a field, patterns, paths, headers and vendors' names: values, not text.
  "mcp__server__tool",
  "mcp__github__*",
  "github",
  "npx",
  "https://example.com/mcp",
  "https://registry.example.com",
  "Authorization",
  "Linear",
  "Sentry",
  "Notion",
  "Playwright",
  "skills/",
  ".freeloop/skills",
  ".agents/skills",
  ".claude/skills",
  ...["Tab", "Escape", "Enter", "Backspace", "Home", "End", "Meta", "Control", "Ctrl", "Alt", "Shift", "Super"],
]);

// A line of code that holds none of < > { } = and so could pass for text.
const CODE_LINE =
  /^(\.|\?|:|!|&&|\|\||\)|\[|\]|\w+\??: |return\b|case\b|default\b|const\b|let\b|if\b|else\b|for\b|while\b|export\b|import\b|type\b|interface\b|function\b|await\b|throw\b|break\b|continue\b|delete\b)|[({[]$|^[\w.?[\]]+\(|^[a-z][A-Za-z]*[,;]?$|^(type )?[A-Za-z_$][\w$]*,$|^[\w.]+(\+\+|--);?$/;

// A capitalized word on its own or starting a phrase, and not a key such as Meta+1.
const LABEL = /^[A-Z][a-z']+(?:$|[\s,.?!:…])/;

function untranslated(source: string): string[] {
  const code = source
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/(^|\s)\/\/.*$/gm, "$1")
    // A regular expression, roughly: a slash where a value starts.
    .replace(/(^|[=(,:!&|?;{[]|\breturn)(\s*)\/(?![/*])(?:[^/\\\n[]|\\.|\[(?:[^\]\\\n]|\\.)*\])+\/[dgimsuvy]*/gm, "$1$2/re/");
  const found: string[] = [];
  // What is said in code: a message for the developer, a palette group's id (palette.ts
  // names the group), a CSS selector a query takes, a grid's columns, a case to match, a
  // dragged item's type.
  const notText = (at: number) =>
    /(?:console\.\w+\(|\bgroup: |querySelector(?:All)?(?:<\w+>)?\(|\.closest\(|\.matches\(|gridTemplate\w*: |\bcase |\btypes\.includes\()$/.test(
      code.slice(Math.max(0, at - 40), at),
    );
  const add = (s: string) => NOT_TEXT.has(s) || found.push(s);
  const words = (s: string) => /[A-Za-z]{2,}/.test(s);
  // Text between tags on one line, or after an expression up to a closing tag.
  for (const m of code.matchAll(/[>}]([^<>{}\n]*)(?=<\/)/g)) if (words(m[1])) add(m[1].trim());
  // A line of text on its own, as between an opening and a closing tag.
  for (const line of code.split("\n")) {
    const s = line.trim();
    if (words(s) && !/[<>{}="`]/.test(s) && !CODE_LINE.test(s)) add(s);
  }
  // A user-facing attribute with words in it.
  for (const m of code.matchAll(/\b(?:aria-label|title|placeholder|label|hint|alt)="([^"]*)"/g)) if (words(m[1])) add(m[1]);
  // A string that starts like a label or reads like a sentence.
  for (const m of code.matchAll(/"((?:[^"\\\n]|\\.)*)"|'((?:[^'\\\n]|\\.)*)'/g)) {
    const s = m[1] ?? m[2];
    if (notText(m.index)) continue;
    if (LABEL.test(s) || (/\s/.test(s) && /[,.?!:…]/.test(s) && words(s))) add(s);
  }
  for (const m of code.matchAll(/`([^`]*)`/g)) {
    // A class list, not text.
    if (/className=\{\s*$/.test(code.slice(Math.max(0, m.index - 20), m.index)) || notText(m.index)) continue;
    const s = m[1].replace(/\$\{[^}]*\}/g, "").replace(/\bFreeloop\b/g, "");
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
  // Names in an import over several lines, and a class list built in a template.
  assert.deepEqual(untranslated("import {\n  CONFIG_EVENT_NAME,\n  EngineService,\n  type Session,\n} from \"./api.ts\";"), []);
  assert.deepEqual(untranslated("<span className={`status-dot status-${status}`} />"), []);
  assert.deepEqual(untranslated("    back?.focus?.();\n    buttons[next].focus();"), []);
  // Loops and counters, a message for the developer, a palette group's id, a key's name.
  assert.deepEqual(untranslated("    for (const rel of open) load(rel);\n    settings++;\n    endA--;"), []);
  assert.deepEqual(untranslated('console.error("markdown: rendering failed", err);'), []);
  assert.deepEqual(untranslated('{ id: "diff", group: "Changes", run }'), []);
  assert.deepEqual(untranslated('if (e.key === "Enter") send();'), []);
  // A regular expression's backticks open no template.
  assert.deepEqual(untranslated("const FENCE = /^\\s*(```|~~~)/;\nconst a = `${x}`;\nconst b = `${y} ${z}`;"), []);
  assert.deepEqual(untranslated("const ok = /^Done/.test(s) ? `Done in ${n}` : x;"), ["Done in ${n}"]);
  assert.deepEqual(untranslated("const where = `${provider}/${model}`;\nconst s = \"Saved.\";"), ["Saved."]);
  assert.deepEqual(untranslated('document.querySelector<HTMLElement>(".pane-active textarea")?.focus();\ncase "Home":\n<h1>\n  Freeloop\n</h1>'), []);
});

test("every string in the interface comes from the catalogues", () => {
  assert.ok(SOURCES.length > 120, `${SOURCES.length} files`);
  assert.deepEqual(Object.keys(EXEMPT).filter((name) => !existsSync(new URL(`./${name}`, import.meta.url))), [], "exempt files that are gone");
  const found = SOURCES.map((name) => [name, untranslated(readFileSync(new URL(`./${name}`, import.meta.url), "utf8"))] as const);
  assert.deepEqual(
    found.filter(([, texts]) => texts.length > 0),
    [],
  );
});

// GUI-048: numbers, dates and times are written in the locale of the language in force,
// so every formatter takes it from the translation module: never the web view's own,
// which a call without a locale (or with []) falls back to, and never a fixed one.
const FORMATTER = /\.toLocale(?:String|DateString|TimeString|UpperCase|LowerCase)\(|\bIntl\.[A-Z]\w*\(/g;

function withoutLocale(source: string): string[] {
  const code = source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|\s)\/\/.*$/gm, "$1");
  return [...code.matchAll(FORMATTER)].filter((m) => !code.startsWith("locale()", m.index + m[0].length)).map((m) => code.slice(m.index, m.index + m[0].length + 12));
}

test("the formatter check finds a call without the locale", () => {
  assert.equal(withoutLocale("n.toLocaleString()").length, 1);
  assert.equal(withoutLocale('d.toLocaleDateString([], { month: "short" })').length, 1);
  assert.equal(withoutLocale('d.toLocaleTimeString("en")').length, 1);
  assert.equal(withoutLocale("new Intl.NumberFormat(undefined, {})").length, 1);
  assert.equal(withoutLocale("n.toLocaleString(locale())\nnew Intl.DateTimeFormat(locale(), {})\n// n.toLocaleString()").length, 0);
});

test("every number, date and time formatter takes the locale from the translation module", () => {
  const dir = new URL("./", import.meta.url);
  const sources = readdirSync(dir).filter((f) => /\.tsx?$/.test(f) && !/\.(test|typetest)\.ts$/.test(f));
  assert.ok(sources.length > 50);
  for (const name of sources) assert.deepEqual(withoutLocale(readFileSync(new URL(name, dir), "utf8")), [], name);
});
