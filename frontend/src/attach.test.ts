import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { addText, isPicture, MAX_FILE_BYTES, MAX_FILES, textOf } from "./attach.ts";
import { setLanguage } from "./i18n.ts";

const bytes = (s: string) => new TextEncoder().encode(s);

test("pictures are the types the model takes; other images are files", () => {
  assert.ok(isPicture("image/png"));
  assert.ok(!isPicture("image/svg+xml"));
  assert.ok(!isPicture("application/pdf"));
});

test("only text reads as text", () => {
  assert.equal(textOf(bytes("héllo")), "héllo");
  assert.equal(textOf(new Uint8Array([0x25, 0x50, 0x00])), null);
  assert.equal(textOf(new Uint8Array([0xff, 0xfe, 0x41])), null);
});

test("a file is added, replaces one of its name, and is refused with a reason", () => {
  let r = addText([], "a.md", bytes("one"));
  assert.deepEqual(r.list, [{ name: "a.md", text: "one" }]);
  r = addText(r.list, "a.md", bytes("two"));
  assert.deepEqual(r.list, [{ name: "a.md", text: "two" }]);

  const binary = addText(r.list, "a.pdf", new Uint8Array([0, 1, 2]));
  assert.equal(binary.list, r.list);
  assert.match(binary.error ?? "", /a\.pdf is not text/);
  const big = addText([], "big.log", new Uint8Array(MAX_FILE_BYTES + 1).fill(97));
  assert.match(big.error ?? "", /larger than 256 KB/);
  const many = Array.from({ length: MAX_FILES }, (_, i) => ({ name: `f${i}`, text: "x" }));
  assert.match(addText(many, "one-more", bytes("x")).error ?? "", /at most/);
  const full = Array.from({ length: 5 }, (_, i) => ({ name: `f${i}`, text: "x".repeat(MAX_FILE_BYTES) }));
  assert.match(addText(full, "last", bytes("x")).error ?? "", /1 MB/);
});

// GUI-048: the reason a file is refused is in the language set.
test("a refused file says why in the language set", () => {
  setLanguage("de");
  assert.equal(
    addText([], "a.pdf", new Uint8Array([0, 1, 2])).error,
    "a.pdf ist weder Text noch ein Bild und kann daher nicht in eine Nachricht. Besser ins Projekt legen und den Pfad nennen.",
  );
  const many = Array.from({ length: MAX_FILES }, (_, i) => ({ name: `f${i}`, text: "x" }));
  assert.equal(addText(many, "one-more", bytes("x")).error, "Eine Nachricht fasst höchstens 20 Dateien.");
  setLanguage("en");
  assert.equal(addText(many, "one-more", bytes("x")).error, "A message takes at most 20 files.");
  assert.equal(addText([], "big.log", new Uint8Array(MAX_FILE_BYTES + 1).fill(97)).error, "big.log is larger than 256 KB. Put it in the project and ask for the part you need.");
});

// A pasted picture once landed at the far edge of a wide window: its own margin
// rule replaced the auto side margins that keep the composer's children in line.
test("what shows above the message box stays in the composer's column", () => {
  const css = readFileSync(new URL("./app.css", import.meta.url), "utf8");
  for (const name of ["picture-strip", "attachments", "todo-list"]) {
    const rule = new RegExp(`^\\.${name} \\{([^}]*)\\}`, "m").exec(css);
    assert.ok(rule, name);
    assert.doesNotMatch(rule[1], /^\s*margin(-left|-right|-inline)?:/m, name);
  }
});
