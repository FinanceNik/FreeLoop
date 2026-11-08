import assert from "node:assert/strict";
import { test } from "node:test";
import { contextHigh, contextLabel, contextResent, contextTitle, parseLimit, shortTokens } from "./contextMeter.ts";

test("the meter says a share with a window and a size without one", () => {
  assert.equal(contextLabel({ tokens: 62_000, window: 100_000 }), "62k · 62 %");
  assert.equal(contextLabel({ tokens: 100, window: 100_000 }), "100 · <1 %");
  assert.equal(contextLabel({ tokens: 840_000, window: 1_000_000 }), "840k · 84 %");
  assert.equal(contextLabel({ tokens: 140_000, window: 0 }), "140k");
  assert.equal(contextLabel({ tokens: 1_200_000, window: 0 }), "1.2M");
});

test("no figure means no meter, not a zero", () => {
  assert.equal(contextLabel(null), null);
  assert.equal(contextLabel(undefined), null);
  assert.equal(contextLabel({ tokens: 0, window: 100_000 }), null);
});

test("a context is high from 80 % of a known window", () => {
  assert.equal(contextHigh({ tokens: 79_999, window: 100_000 }), false);
  assert.equal(contextHigh({ tokens: 80_000, window: 100_000 }), true);
  assert.equal(contextHigh({ tokens: 900_000, window: 0 }), false);
  assert.equal(contextHigh(null), false);
});

test("the tooltip gives the tokens, and the window when known", () => {
  assert.match(contextTitle({ tokens: 1000, window: 2000 }), /1,000 of 2,000 tokens/);
  assert.match(contextTitle({ tokens: 1000, window: 0 }), /Context: 1,000 tokens/);
});

test("shortTokens rounds up into the next unit at its edge", () => {
  assert.equal(shortTokens(999), "999");
  assert.equal(shortTokens(999_400), "999k");
  assert.equal(shortTokens(999_600), "1.0M");
});

test("a resend is a large context, about the same size, written again", () => {
  // From real sessions: a whole context written again, with little or nothing read.
  assert.equal(contextResent(617_724, 620_812, 620_810), true);
  assert.equal(contextResent(641_717, 641_864, 597_799), true);
});

test("an ordinary call, a first message or a compaction is no resend", () => {
  // Most of it came from the cache.
  assert.equal(contextResent(156_000, 160_000, 3_991), false);
  // Nothing before it: the context is new.
  assert.equal(contextResent(0, 90_000, 90_000), false);
  // A large file was just added.
  assert.equal(contextResent(10_000, 90_000, 80_000), false);
  // Just compacted, from far larger.
  assert.equal(contextResent(965_779, 82_516, 82_514), false);
  // Too small to matter.
  assert.equal(contextResent(20_000, 21_000, 21_000), false);
});

test("a limit can be typed as people write it", () => {
  assert.equal(parseLimit("200k"), 200_000);
  assert.equal(parseLimit("1M"), 1_000_000);
  assert.equal(parseLimit("1.5m"), 1_500_000);
  assert.equal(parseLimit("1,000,000"), 1_000_000);
  assert.equal(parseLimit(" 128000 "), 128_000);
  assert.equal(parseLimit(""), 0);
  assert.equal(parseLimit("lots"), null);
  assert.equal(parseLimit("-5"), null);
});

test("with no limit, the tooltip says where to set one", () => {
  assert.match(contextTitle({ tokens: 1000, window: 0 }), /Context limit/);
  assert.doesNotMatch(contextTitle({ tokens: 1000, window: 2000 }), /Context limit/);
});
