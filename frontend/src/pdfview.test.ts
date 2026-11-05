import { readFileSync } from "node:fs";
import { test } from "node:test";
import assert from "node:assert/strict";
import { pdfBytes, viewerDocument, VIEWER_POLICY } from "./pdfview.ts";

const directive = (name: string) => VIEWER_POLICY.split(";").map((d) => d.trim()).find((d) => d.startsWith(`${name} `));

// In a real WebKit view, this policy is what stops a PDF's link: without it a click left
// the file. Loosening it reopens that, and scripts in the viewer would reach the app's page.
test("the viewer's policy lets it load the PDF and nothing else", () => {
  assert.equal(directive("frame-src"), "frame-src blob:");
  assert.equal(directive("default-src"), "default-src 'none'");
  assert.equal(directive("script-src"), undefined);
  assert.equal(directive("connect-src"), undefined);
});

test("the viewer's page carries the policy before anything else, and no script", () => {
  const page = viewerDocument();
  assert.ok(page.indexOf("Content-Security-Policy") < page.indexOf("<iframe"));
  assert.ok(page.includes(`content="${VIEWER_POLICY}"`));
  assert.doesNotMatch(page, /<script/i);
  // A sandbox would leave it blank: WebKit's PDF view counts as a plugin.
  assert.doesNotMatch(page, /sandbox/i);
});

test("the engine's bytes come back as they were", () => {
  assert.deepEqual([...pdfBytes(btoa("%PDF-\x00\xff"))], [37, 80, 68, 70, 45, 0, 255]);
  assert.equal(pdfBytes("").length, 0);
});

// A frame's policies stack, and the app page's would otherwise leave every PDF blank. Only
// frames widen: nothing else in the page may load from a blob's page or the outside.
test("the app's own policy lets the viewer hold a PDF, and no more than it did", () => {
  const config = readFileSync(new URL("../vite.config.ts", import.meta.url), "utf8");
  assert.match(config, /"frame-src blob: /);
  assert.match(config, /"script-src 'self'"/);
  assert.doesNotMatch(config, /script-src[^"]*blob:/);
  assert.match(config, /"object-src 'none'"/);
});
