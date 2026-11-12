import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { pairCode } from "./phone.ts";
import { workerPath, workerScope } from "./register.ts";

const pub = new URL("../../phone/public/", import.meta.url);
const read = (name: string) => readFileSync(new URL(name, pub), "utf8");

test("the manifest makes a standalone app under /phone/", () => {
  const m = JSON.parse(read("manifest.webmanifest"));
  assert.equal(m.display, "standalone");
  assert.equal(m.name, "Freeloop");
  assert.equal(m.start_url, "/phone/");
  assert.equal(m.scope, "/phone/");
  assert.match(m.background_color, /^#[0-9a-f]{6}$/i);
  assert.match(m.theme_color, /^#[0-9a-f]{6}$/i);
  const sizes = m.icons.map((i: { sizes: string }) => i.sizes);
  assert.ok(sizes.includes("192x192") && sizes.includes("512x512"));
  for (const icon of m.icons) {
    assert.equal(icon.type, "image/png");
    assert.ok(icon.src.startsWith("/phone/"));
    assert.ok(statSync(new URL(icon.src.slice("/phone/".length), pub)).size > 0);
  }
});

test("the page links the manifest and Apple's touch icon", () => {
  const html = readFileSync(new URL("../../phone/index.html", import.meta.url), "utf8");
  assert.match(html, /rel="manifest" href="\/phone\/manifest\.webmanifest"/);
  assert.match(html, /rel="apple-touch-icon" href="\/phone\/apple-touch-icon\.png"/);
  assert.ok(statSync(new URL("apple-touch-icon.png", pub)).size > 0);
});

test("the worker answers no fetch and stores nothing", () => {
  const code = read("sw.js")
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");
  assert.doesNotMatch(code, /["']fetch["']|onfetch|\bcaches\b|indexedDB|localStorage|importScripts/);
});

test("the worker is registered at /phone/ by the phone app only", () => {
  assert.equal(workerPath, "/phone/sw.js");
  assert.equal(workerScope, "/phone/");
  const walk = (dir: string): string[] =>
    readdirSync(dir).flatMap((n) => {
      const p = join(dir, n);
      if (statSync(p).isDirectory()) return n === "phone" ? [] : walk(p);
      return /\.(tsx?|html)$/.test(n) && !n.endsWith(".test.ts") ? [p] : [];
    });
  const src = new URL("../", import.meta.url).pathname;
  for (const f of walk(src)) assert.doesNotMatch(readFileSync(f, "utf8"), /serviceWorker/, `${f} registers a worker`);
  assert.doesNotMatch(readFileSync(new URL("../../index.html", import.meta.url), "utf8"), /serviceWorker/);
});

test("opened without a pairing link the app has no code to fill in", () => {
  assert.equal(pairCode(""), "");
  assert.equal(pairCode("#"), "");
});

test("the worker shows a notification from a push's kind and session, and nothing else", () => {
  const listeners: Record<string, (e: unknown) => void> = {};
  const shown: { title: string; options: { body: string; tag: string; data: { url: string } } }[] = [];
  const self = {
    addEventListener: (type: string, fn: (e: unknown) => void) => (listeners[type] = fn),
    skipWaiting: () => {},
    clients: { claim: () => {} },
    navigator: { language: "de-CH" },
    registration: { showNotification: (title: string, options: never) => (shown.push({ title, options }), Promise.resolve()) },
  } as Record<string, unknown>;
  new Function("self", read("sw.js"))(self);
  const push = (json: () => unknown) => listeners.push({ data: { json }, waitUntil: () => {} });
  push(() => ({ kind: "approval", sessionId: "ses_1", secret: "leaks" }));
  assert.equal(shown[0].options.body, "Eine Sitzung braucht dich");
  assert.equal(shown[0].options.data.url, "/phone/#session=ses_1");
  assert.doesNotMatch(JSON.stringify(shown[0]), /leaks/);
  push(() => {
    throw new Error("not json");
  });
  assert.equal(shown.length, 2, "a bad payload still shows a notification");
  push(() => ({ kind: "<b>x</b>", sessionId: "../../x" }));
  assert.equal(shown[2].options.data.url, "/phone/");
  assert.equal(shown[2].options.body, "In einer Sitzung ist etwas passiert");
});

test("the worker's text per kind and language", () => {
  const words: Record<string, Record<string, string>> = {
    en: { approval: "A session needs you", done: "A session finished", failed: "A session stopped with an error" },
    de: { approval: "Eine Sitzung braucht dich", done: "Eine Sitzung ist fertig", failed: "Eine Sitzung wurde mit einem Fehler beendet" },
    nl: { approval: "Een sessie heeft je nodig", done: "Een sessie is klaar", failed: "Een sessie is met een fout gestopt" },
  };
  for (const [lang, kinds] of Object.entries(words)) {
    for (const [kind, body] of Object.entries(kinds)) {
      const shown: { options: { body: string } }[] = [];
      let handler: (e: unknown) => void = () => {};
      const self = {
        addEventListener: (type: string, fn: (e: unknown) => void) => type === "push" && (handler = fn),
        skipWaiting: () => {},
        clients: { claim: () => {} },
        navigator: { language: lang },
        registration: { showNotification: (_t: string, options: never) => (shown.push({ options }), Promise.resolve()) },
      } as Record<string, unknown>;
      new Function("self", read("sw.js"))(self);
      handler({ data: { json: () => ({ kind, sessionId: "s1" }) }, waitUntil: () => {} });
      assert.equal(shown[0].options.body, body, `${lang} ${kind}`);
    }
  }
});
