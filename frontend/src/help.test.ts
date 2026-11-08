import assert from "node:assert/strict";
import { test } from "node:test";
import { HELP_TOPICS, plain, searchHelp } from "./help.ts";

const pages: Record<string, string> = {
  "getting-started": "# Getting started\n\nTry the **demo** first: it needs no key.\n\n- Add a provider\n- Start a session",
  permissions: "# Permissions and the policy\n\nA deny rule always wins. See [the docs](https://example.com).\n\n```\nallow shell\n```",
};

test("plain strips the markup to one line of text", () => {
  assert.equal(plain(pages["getting-started"]), "Getting started Try the demo first: it needs no key. Add a provider Start a session");
  assert.equal(plain(pages.permissions), "Permissions and the policy A deny rule always wins. See the docs.");
});

test("search needs every word, puts title matches first and quotes the text", () => {
  assert.deepEqual(searchHelp(HELP_TOPICS, pages, ""), []);
  const deny = searchHelp(HELP_TOPICS, pages, "deny rule");
  assert.deepEqual(deny.map((h) => h.slug), ["permissions"]);
  assert.match(deny[0].snippet, /A deny rule always wins/);
  assert.deepEqual(searchHelp(HELP_TOPICS, pages, "deny demo"), []);
  // "policy" is in one title; "a" in both texts: the title match comes first.
  assert.deepEqual(searchHelp(HELP_TOPICS, pages, "policy").map((h) => h.slug), ["permissions"]);
  assert.deepEqual(searchHelp(HELP_TOPICS, pages, "start").map((h) => h.slug), ["getting-started"]);
  // A topic without a page is not offered.
  assert.ok(!searchHelp(HELP_TOPICS, pages, "a").some((h) => h.slug === "sessions"));
});

test("every topic has a unique slug and somewhere to go", () => {
  assert.equal(new Set(HELP_TOPICS.map((t) => t.slug)).size, HELP_TOPICS.length);
  assert.ok(HELP_TOPICS.every((t) => t.opens.length > 0));
});
