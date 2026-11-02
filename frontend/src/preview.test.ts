import { test } from "node:test";
import assert from "node:assert/strict";
import { framed, parseAddress } from "./preview.ts";

const app = "wails://wails.localhost";

test("ports and localhost addresses load", () => {
  assert.deepEqual(parseAddress("3000", app), { url: "http://localhost:3000/", port: 3000 });
  assert.deepEqual(parseAddress(" localhost:5173/docs?x=1 ", app), { url: "http://localhost:5173/docs?x=1", port: 5173 });
  assert.deepEqual(parseAddress("https://127.0.0.1:8443", app), { url: "https://127.0.0.1:8443/", port: 8443 });
  assert.deepEqual(parseAddress("http://[::1]:8080/", app), { url: "http://[::1]:8080/", port: 8080 });
  assert.deepEqual(parseAddress("http://localhost", app), { url: "http://localhost/", port: 80 });
});

test("anything else does not", () => {
  for (const bad of ["", "https://example.com", "example.com:3000", "file:///etc/passwd", "javascript:alert(1)", "data:text/html,x", "http://user:pw@localhost:3000", "http://localhost.evil.com:3000", "http://127.0.0.1.nip.io/", "99999", "http://169.254.169.254/"]) {
    assert.ok("error" in parseAddress(bad, app), bad);
  }
  assert.ok("error" in parseAddress("34117", "http://localhost:34117"), "the app's own page");
});

test("the app's page knows it is in a frame", () => {
  const w = { top: null as unknown, self: null as unknown };
  w.top = w;
  w.self = w;
  assert.equal(framed(w), false);
  assert.equal(framed({ top: {}, self: {} }), true);
});
