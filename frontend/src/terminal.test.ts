import { test } from "node:test";
import assert from "node:assert/strict";
import { after, bytesOf, countWide, fitSize } from "./terminal.ts";

test("chunks decode to bytes, a split character included", () => {
  assert.deepEqual([...bytesOf(Buffer.from("hi\r\n").toString("base64"))], [104, 105, 13, 10]);
  const euro = Buffer.from("€");
  assert.deepEqual([...bytesOf(euro.subarray(0, 2).toString("base64"))], [0xe2, 0x82]);
  assert.equal(bytesOf(null).length, 0);
});

test("the size fits the box", () => {
  assert.deepEqual(fitSize(800, 400, 8, 17), { cols: 100, rows: 23 });
  assert.deepEqual(fitSize(5, 5, 8, 17), { cols: 2, rows: 1 });
  assert.deepEqual(fitSize(800, 400, 0, 17), { cols: 80, rows: 24 });
});

test("a chunk joins the scrollback without gaps or repeats", () => {
  const chunk = new Uint8Array([1, 2, 3, 4]);
  assert.deepEqual([...after(chunk, 10, 10)], [1, 2, 3, 4]);
  assert.deepEqual([...after(chunk, 10, 12)], [3, 4]);
  assert.deepEqual([...after(chunk, 10, 20)], []);
  assert.deepEqual([...after(chunk, 12, 10)], [1, 2, 3, 4]);
});

// GUI-054: an emoji takes two cells, as the shell that printed it counts.
test("a terminal counts an emoji as two cells wide", () => {
  let widths: { version: string; wcwidth(code: number): number } | undefined;
  const term = {
    unicode: { activeVersion: "6", register: (p: typeof widths) => (widths = p) },
    loadAddon(addon: { activate(term: unknown): void }) {
      addon.activate(this);
    },
  };
  countWide(term);
  assert.equal(term.unicode.activeVersion, "11");
  assert.equal(widths?.version, "11");
  assert.equal(widths?.wcwidth(0x1f642), 2);
  assert.equal(widths?.wcwidth(0x41), 1);
  // A Nerd Font's icons are in the private use area and stay one cell.
  assert.equal(widths?.wcwidth(0xe0b0), 1);
});
