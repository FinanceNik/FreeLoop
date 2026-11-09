import assert from "node:assert/strict";
import { test } from "node:test";
import { prepareImage } from "./imageprep.ts";
import { setLanguage } from "./i18n.ts";

// The canvas work needs a page; what a file that is no picture gets does not.
test("a file that is no picture is refused with the reason, in the language set", async () => {
  await assert.rejects(prepareImage(new Blob(["x"], { type: "image/svg+xml" })), { message: "image/svg+xml is not a picture type agents read; use PNG, JPEG, GIF or WebP." });
  await assert.rejects(prepareImage(new Blob(["x"])), { message: "This file is not a picture type agents read; use PNG, JPEG, GIF or WebP." });
  setLanguage("de");
  await assert.rejects(prepareImage(new Blob(["x"], { type: "image/svg+xml" })), {
    message: "image/svg+xml ist kein Bildformat, das Agenten lesen; geeignet sind PNG, JPEG, GIF und WebP.",
  });
  setLanguage("nl");
  await assert.rejects(prepareImage(new Blob(["x"])), { message: "Dit bestand is geen afbeeldingstype dat agents lezen; geschikt zijn PNG, JPEG, GIF en WebP." });
  setLanguage("en");
});
