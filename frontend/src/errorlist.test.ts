import { test } from "node:test";
import assert from "node:assert/strict";
import { addError, errorHeading, type ShownError } from "./errorlist.ts";
import { setLanguage } from "./i18n.ts";

const e = (detail: string, session?: string): ShownError => ({ detail, session });

test("an error names its session in its heading", () => {
  assert.equal(errorHeading(e("no key", "Fix login")), "Error in Fix login:");
  assert.equal(errorHeading(e("no key")), "Error:");
});

test("the heading follows the language, and the engine's words stay as given", () => {
  const err = e("open /tmp/x: permission denied", "Fix login");
  setLanguage("de");
  assert.equal(errorHeading(err), "Fehler in „Fix login“:");
  assert.equal(errorHeading(e("no key")), "Fehler:");
  setLanguage("nl");
  assert.equal(errorHeading(err), "Fout in ‘Fix login’:");
  assert.equal(err.detail, "open /tmp/x: permission denied");
  setLanguage("en");
});

test("a new error does not replace an unread one, and repeats do not pile up", () => {
  assert.deepEqual(addError([e("a")], e("b")), [e("a"), e("b")]);
  assert.deepEqual(addError([e("a"), e("b")], e("a")), [e("b"), e("a")]);
  // The same words from another session are another error.
  assert.deepEqual(addError([e("a", "One")], e("a", "Two")), [e("a", "One"), e("a", "Two")]);
});

test("the list stays short, keeping the newest", () => {
  assert.deepEqual(addError([e("a"), e("b"), e("c"), e("d")], e("e")), [e("b"), e("c"), e("d"), e("e")]);
});
