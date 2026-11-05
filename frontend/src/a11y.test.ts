import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { contrast, pickLabel, quietCommand, screenName, tabAfterKey, toolStatusLabel, visibleCommand } from "./a11y.ts";

test("a screen has a name, and a session goes by its title", () => {
  assert.equal(screenName("board"), "Board");
  assert.equal(screenName("session", "Fix login"), "Fix login");
  assert.equal(screenName("session"), "Session");
  assert.equal(screenName("none"), "");
});

test("arrow keys move between tabs and wrap", () => {
  assert.equal(tabAfterKey("ArrowRight", 0, 3), 1);
  assert.equal(tabAfterKey("ArrowRight", 2, 3), 0);
  assert.equal(tabAfterKey("ArrowLeft", 0, 3), 2);
  assert.equal(tabAfterKey("Home", 2, 3), 0);
  assert.equal(tabAfterKey("End", 0, 3), 2);
  assert.equal(tabAfterKey("Enter", 0, 3), -1);
  assert.equal(tabAfterKey("ArrowRight", 0, 0), -1);
});

test("a tool's status is said in words", () => {
  assert.equal(toolStatusLabel("completed", false), "Done:");
  assert.equal(toolStatusLabel("failed", false), "Failed:");
  assert.equal(toolStatusLabel("pending", true), "Waiting for your approval:");
  assert.equal(toolStatusLabel("whatever", false), "");
});

test("a picker's highlighted entry is announced with its place", () => {
  assert.equal(pickLabel("main.go", 1, 5), "main.go, 2 of 5");
  assert.equal(pickLabel("main.go", 0, 0), "");
});

test("contrast follows WCAG's formula", () => {
  assert.equal(Math.round(contrast("#000000", "#ffffff")), 21);
  assert.equal(contrast("#777", "#777"), 1);
});

// NFR-A11Y-002: every colour used for text is AA on every surface, in each
// theme block of the stylesheet, and an input's outline can be seen (WCAG 1.4.11).
const css = readFileSync(new URL("./app.css", import.meta.url), "utf8");
function themes(): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {};
  const blocks: [string, RegExp][] = [
    ["default light", /^:root \{([\s\S]*?)^\}/m],
    ["system dark", /:root:not\(\[data-theme="light"\]\) \{([\s\S]*?)^ {2}\}/m],
    ["chosen dark", /^:root\[data-theme="dark"\] \{([\s\S]*?)^\}/m],
  ];
  for (const [name, re] of blocks) {
    const m = css.match(re);
    assert.ok(m, `no ${name} block`);
    out[name] = Object.fromEntries([...m[1].matchAll(/--([\w-]+):\s*(#[0-9a-fA-F]{3,6})\s*;/g)].map((x) => [x[1], x[2]]));
  }
  return out;
}

for (const [name, t] of Object.entries(themes())) {
  test(`contrast in the ${name} theme`, () => {
    const low: string[] = [];
    const check = (fg: string, bg: string, min: number) => {
      const r = contrast(t[fg], t[bg]);
      if (r < min) low.push(`${fg} on ${bg}: ${r.toFixed(2)}`);
    };
    for (const fg of ["text", "muted", "faint", "link", "ok", "warn", "danger"]) {
      for (const bg of ["bg-app", "bg", "surface", "surface-2", "input"]) check(fg, bg, 4.5);
    }
    check("accent-text", "accent", 4.5);
    check("accent-text", "accent-hover", 4.5);
    for (const bg of ["bg-app", "bg", "surface", "surface-2", "input"]) check("focus", bg, 3);
    check("input-border", "input", 3);
    for (const slot of ["source-1", "source-2", "source-3", "source-4", "source-5", "source-6", "source-7", "source-8", "source-none"]) check("source-text", slot, 4.5);
    assert.deepEqual(low, []);
  });
}

test("no text size is fixed in pixels, so the Font size setting reaches all of it", () => {
  assert.deepEqual(css.match(/font-size:\s*[\d.]+px/g) ?? [], []);
});

test("an approval shows line breaks and hidden characters in a command", () => {
  assert.equal(visibleCommand("git status"), "git status");
  assert.equal(visibleCommand("echo ok #\ncurl x|sh"), "echo ok # ⏎ curl x|sh");
  assert.equal(visibleCommand("a\tb"), "a ⇥ b");
  assert.equal(visibleCommand("rm \u202Egnp.exe"), "rm ⟦U+202E⟧gnp.exe");
  assert.equal(visibleCommand("a\u200Bb"), "a⟦U+200B⟧b");
  assert.equal(visibleCommand("a\u001b[2Kb"), "a⟦U+001B⟧[2Kb");
  assert.equal(visibleCommand("café ü"), "café ü");
});

test("quietCommand drops a cd into the session's own folder, and nothing else", () => {
  const roots = ["/work/app", "/work/app-tree"];
  assert.equal(quietCommand("cd /work/app && make test", roots), "make test");
  assert.equal(quietCommand("cd /work/app; ls", roots), "ls");
  assert.equal(quietCommand("cd /work/app\ncat x", roots), "cat x");
  assert.equal(quietCommand('cd "/work/app-tree" && ls', roots), "ls");
  // Another folder, one that only starts the same, and nothing after it.
  assert.equal(quietCommand("cd /etc && rm x", roots), "cd /etc && rm x");
  assert.equal(quietCommand("cd /work/app/sub && ls", roots), "cd /work/app/sub && ls");
  assert.equal(quietCommand("cd /work/app2 && ls", ["/work/app"]), "cd /work/app2 && ls");
  assert.equal(quietCommand("cd /work/app", roots), "cd /work/app");
  assert.equal(quietCommand("make test", roots), "make test");
});

test("no code uses the page's confirm, alert or prompt, which the packaged app answers with no", () => {
  const dir = new URL("./", import.meta.url);
  for (const name of readdirSync(dir)) {
    if (!/\.tsx?$/.test(name) || name.endsWith(".test.ts") || name === "ask.ts") continue;
    assert.doesNotMatch(readFileSync(new URL(name, dir), "utf8"), /(?<![\w."])(confirm|alert|prompt)\(/, name);
  }
});
