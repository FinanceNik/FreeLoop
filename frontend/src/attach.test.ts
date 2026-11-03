import assert from "node:assert/strict";
import { test } from "node:test";
import { addText, isPicture, MAX_FILE_BYTES, MAX_FILES, textOf } from "./attach.ts";

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
