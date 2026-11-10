import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { HELP_SLUGS, helpTopics, plain, searchHelp } from "./help.ts";
import { setLanguage, type Language } from "./i18n.ts";

const pages: Record<string, string> = {
  "getting-started": "# Getting started\n\nTry the **demo** first: it needs no key.\n\n- Add a provider\n- Start a session",
  permissions: "# Permissions and the policy\n\nA deny rule always wins. See [the docs](https://example.com).\n\n```\nallow shell\n```",
};

test("plain strips the markup to one line of text", () => {
  assert.equal(plain(pages["getting-started"]), "Getting started Try the demo first: it needs no key. Add a provider Start a session");
  assert.equal(plain(pages.permissions), "Permissions and the policy A deny rule always wins. See the docs.");
});

test("search needs every word, puts title matches first and quotes the text", () => {
  setLanguage("en");
  const topics = helpTopics();
  assert.deepEqual(searchHelp(topics, pages, ""), []);
  const deny = searchHelp(topics, pages, "deny rule");
  assert.deepEqual(deny.map((h) => h.slug), ["permissions"]);
  assert.match(deny[0].snippet, /A deny rule always wins/);
  assert.deepEqual(searchHelp(topics, pages, "deny demo"), []);
  // "policy" is in one title; "a" in both texts: the title match comes first.
  assert.deepEqual(searchHelp(topics, pages, "policy").map((h) => h.slug), ["permissions"]);
  assert.deepEqual(searchHelp(topics, pages, "start").map((h) => h.slug), ["getting-started"]);
  // A topic without a page is not offered.
  assert.ok(!searchHelp(topics, pages, "a").some((h) => h.slug === "sessions"));
});

test("every topic has a unique slug and somewhere to go", () => {
  const topics = helpTopics();
  assert.equal(new Set(topics.map((x) => x.slug)).size, topics.length);
  assert.ok(topics.every((x) => x.opens.length > 0));
});

// GUI-048: every page is in German and Dutch too, none falling back to English, and
// a translation changes only the words: headings, tables, code, shortcuts, links and
// the per-system choices are those of the English page.
const LANGS: Language[] = ["en", "de", "nl"];
const file = (lang: Language, slug: string) => new URL(lang === "en" ? `./help/${slug}.md` : `./help/${lang}/${slug}.md`, import.meta.url);
const read = (lang: Language, slug: string) => readFileSync(file(lang, slug), "utf8");

// shape is what a translation keeps of a page.
function shape(page: string) {
  const fences = [...page.matchAll(/```[\s\S]*?```/g)].map((m) => m[0]);
  const prose = page.replace(/```[\s\S]*?```/g, "");
  const sorted = (xs: Iterable<string>) => [...xs].sort();
  return {
    headings: [...prose.matchAll(/^(#+) /gm)].map((m) => m[1]),
    tableRows: [...prose.matchAll(/^\|.*\|\s*$/gm)].length,
    fences,
    // What is typed or read as written: commands, file names, keys, settings.
    code: sorted([...prose.matchAll(/`([^`\n]+)`/g)].map((m) => m[1])),
    // A click is written in the page's language: ⌘-click, ⌘-Klick, ⌘-klik.
    shortcuts: sorted([...prose.replace(/`[^`\n]+`/g, "").matchAll(/[⌘⌃⌥⇧]+(?:-(?:click|Klick|klik)|[^\s⌘⌃⌥⇧,.;:)]+)/g)].map((m) => m[0].replace(/-(?:Klick|klik)$/, "-click"))),
    links: sorted([...prose.matchAll(/\]\(([^)]+)\)/g)].map((m) => m[1])),
    // {{mac|windows|linux}}: how many choices each has; the words are translated.
    choices: [...prose.matchAll(/\{\{([^{}]*)\}\}/g)].map((m) => m[1].split("|").length),
  };
}

test("every help page is in English, German and Dutch", () => {
  for (const lang of LANGS) {
    const missing = HELP_SLUGS.filter((slug) => !existsSync(file(lang, slug)));
    assert.deepEqual(missing, [], `${lang} pages missing`);
  }
});

test("a translated page keeps the English page's shape, code and shortcuts", () => {
  for (const slug of HELP_SLUGS) {
    const en = shape(read("en", slug));
    for (const lang of ["de", "nl"] as const) {
      if (!existsSync(file(lang, slug))) continue;
      assert.deepEqual(shape(read(lang, slug)), en, `${lang}/${slug}.md`);
    }
  }
});

test("the help is searched in the language it is read in", () => {
  setLanguage("de");
  const de = Object.fromEntries(HELP_SLUGS.filter((slug) => existsSync(file("de", slug))).map((slug) => [slug, read("de", slug)]));
  const hits = searchHelp(helpTopics(), de, "Berechtigungen");
  assert.equal(hits[0]?.slug, "permissions");
  assert.equal(hits[0]?.title, "Berechtigungen und die Richtlinie");
  setLanguage("en");
});
