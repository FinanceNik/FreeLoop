import { test } from "node:test";
import assert from "node:assert/strict";
import { keyMoves, plainRemote } from "./addresses.ts";

test("a change of host, scheme or port moves a saved key", () => {
  assert.equal(keyMoves("https://a.example/v1", "https://a.example/v2"), false);
  assert.equal(keyMoves("https://a.example/v1", "https://b.example/v1"), true);
  assert.equal(keyMoves("https://a.example", "http://a.example"), true);
  assert.equal(keyMoves("https://a.example", "https://a.example:8443"), true);
  assert.equal(keyMoves("", ""), false);
  assert.equal(keyMoves("", "https://b.example"), true);
  assert.equal(keyMoves("https://a.example", ""), true);
});

test("plain http is called out only off this Mac", () => {
  assert.equal(plainRemote("http://llm.example.com/v1"), true);
  assert.equal(plainRemote("https://llm.example.com/v1"), false);
  assert.equal(plainRemote("http://localhost:8080/v1"), false);
  assert.equal(plainRemote("http://127.0.0.1:8080"), false);
  assert.equal(plainRemote("http://[::1]:8080"), false);
  assert.equal(plainRemote("not a url"), false);
});
