import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { COLOR_GROUPS, colorSheet, contrast, hardToRead, isColor, pickerValue } from "./colors.ts";
import { setLanguage, t } from "./i18n.ts";

test("only plain colors pass", () => {
  for (const v of ["#fff", "#2563eb", "#2563eb80", "rgb(1, 2, 3)", "rgba(37, 99, 235, 0.1)"]) assert.equal(isColor(v), true, v);
  for (const v of ["", "red", "#12", "url(https://example.com/x)", "#fff; background: url(x)", "rgb(1,2,3);}", "var(--text)"]) assert.equal(isColor(v), false, v);
});

test("the sheet has one rule per appearance, in the selectors of app.css", () => {
  assert.equal(colorSheet({}), "");
  assert.equal(colorSheet({ light: { text: "#111111" } }), ":root { --text: #111111; }");
  const dark = colorSheet({ dark: { text: "#eeeeee" } });
  assert.match(dark, /@media \(prefers-color-scheme: dark\) \{ :root:not\(\[data-theme="light"\]\) \{ --text: #eeeeee; \} \}/);
  assert.match(dark, /:root\[data-theme="dark"\] \{ --text: #eeeeee; \}/);
  assert.doesNotMatch(dark, /^:root \{/);
});

test("a value that is not a color never reaches the sheet", () => {
  const sheet = colorSheet({ light: { bg: "url(https://example.com/x)", "x;}body{": "#fff", text: "#111" } });
  assert.equal(sheet, ":root { --text: #111; }");
});

test("the accent carries its hover, soft and link colors with it", () => {
  const light = colorSheet({ light: { accent: "#0a7" } });
  assert.match(light, /--accent: #0a7;/);
  assert.match(light, /--accent-hover: color-mix\(/);
  assert.match(light, /--accent-soft: color-mix\(/);
  assert.match(light, /--link: var\(--accent\);/);
  assert.match(colorSheet({ dark: { accent: "#0a7" } }), /--link: color-mix\(in srgb, var\(--accent\) 65%, white\);/);
});

test("a token set by hand beats the derived one", () => {
  const sheet = colorSheet({ light: { accent: "#0a7", link: "#123456" } });
  assert.match(sheet, /--link: #123456;/);
  assert.doesNotMatch(sheet, /--link: var/);
});

test("status colors carry their soft forms", () => {
  assert.match(colorSheet({ light: { danger: "#f00" } }), /--danger-soft: color-mix[^;]+;.*--del: color-mix/);
  assert.match(colorSheet({ light: { ok: "#0f0" } }), /--add: color-mix/);
});

test("picker values", () => {
  assert.equal(pickerValue("#ABC"), "#aabbcc");
  assert.equal(pickerValue("#abcd"), "#aabbcc");
  assert.equal(pickerValue("#2563EB"), "#2563eb");
  assert.equal(pickerValue("#2563eb80"), "#2563eb");
  assert.equal(pickerValue("rgb(37, 99, 235)"), "#2563eb");
  assert.equal(pickerValue("rgba(37 99 235 / 10%)"), "#2563eb");
  assert.equal(pickerValue("color-mix(in srgb, red, blue)"), "");
});

test("contrast", () => {
  assert.equal(Math.round(contrast("#000", "#fff")!), 21);
  assert.equal(contrast("#123456", "#123456"), 1);
  assert.equal(contrast("#000", "var(--x)"), null);
});

test("hard to read pairs are named", () => {
  const colors: Record<string, string> = { text: "#777777", bg: "#888888", "bg-app": "#000000", muted: "#000000", "accent-text": "#ffffff", accent: "#000000" };
  const out = hardToRead((t) => colors[t]);
  assert.equal(out.length, 1);
  assert.match(out[0], /^Text on the conversation: 1\.\d:1$/);
});

test("the editor's tokens exist in the style sheet", () => {
  const css = readFileSync(new URL("./app.css", import.meta.url), "utf8");
  for (const g of COLOR_GROUPS) {
    for (const t of g.tokens) assert.ok(css.includes(`--${t.id}:`), t.id);
  }
});

test("the editor's names are in the language in force (GUI-048)", () => {
  setLanguage("de");
  const colors: Record<string, string> = { text: "#777777", bg: "#888888", "bg-app": "#000000", muted: "#000000", "accent-text": "#ffffff", accent: "#000000" };
  assert.match(hardToRead((t) => colors[t])[0], /^Text auf der Unterhaltung: /);
  assert.equal(t(COLOR_GROUPS[0].title), "Hintergründe");
  assert.equal(t(COLOR_GROUPS[0].tokens[1].label), "Unterhaltung");
  setLanguage("en");
});
