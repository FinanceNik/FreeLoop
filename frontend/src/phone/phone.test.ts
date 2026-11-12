import { test } from "node:test";
import assert from "node:assert/strict";
import { applicationServerKey, deviceName, pushBody, onceOptions, pairCode, parseSSE, pickLanguage, pushSupported, reconnectDelay } from "./phone.ts";

test("the phone answers only once", () => {
  const options = [
    { optionId: "a1", name: "Allow", kind: "allow_once" },
    { optionId: "a2", name: "Always", kind: "allow_always" },
    { optionId: "r1", name: "Deny", kind: "reject_once" },
  ];
  assert.deepEqual(onceOptions(options), { allow: options[0], deny: options[2] });
  assert.deepEqual(onceOptions([{ optionId: "x", name: "Always", kind: "allow_always" }]), { allow: undefined, deny: undefined });
  assert.deepEqual(onceOptions(null), { allow: undefined, deny: undefined });
});

test("a pairing link carries its code", () => {
  assert.equal(pairCode("#pair=AB12-CD34"), "AB12-CD34");
  assert.equal(pairCode("#x=1&pair=ZZ"), "ZZ");
  assert.equal(pairCode("#other"), "");
  assert.equal(pairCode(""), "");
});

test("a phone's first name and language", () => {
  assert.equal(deviceName("Mozilla/5.0 (iPhone; CPU iPhone OS 19_0 like Mac OS X)"), "iPhone");
  assert.equal(deviceName("Mozilla/5.0 (Linux; Android 16)"), "Android phone");
  assert.equal(deviceName("curl"), "Phone");
  assert.equal(pickLanguage(["fr-FR", "nl-BE", "en"]), "nl");
  assert.equal(pickLanguage(["fr"]), "en");
});

test("a server-sent stream is read in whole messages, whatever the chunks", () => {
  const first = parseSSE("", "id: 4\nevent: event\ndata: {\"a\":1}\n\n: ping\n\nevent: summ");
  assert.deepEqual(first.messages, [{ id: "4", event: "event", data: '{"a":1}' }]);
  assert.equal(first.rest, "event: summ");
  const second = parseSSE(first.rest, "ary\ndata: x\ndata: y\n\n");
  assert.deepEqual(second.messages, [{ id: "", event: "summary", data: "x\ny" }]);
  assert.equal(second.rest, "");
  // Carriage returns, as some proxies send, and a message with only a comment.
  assert.deepEqual(parseSSE("", "event: event\r\ndata: z\r\n\r\n").messages, [{ id: "", event: "event", data: "z" }]);
  assert.deepEqual(parseSSE("", ": keep\n\n").messages, []);
  // A message with no event name is the default one.
  assert.deepEqual(parseSSE("", "data: m\n\n").messages, [{ id: "", event: "message", data: "m" }]);
});

test("a stream that dropped goes on after a pause that grows to a limit", () => {
  assert.equal(reconnectDelay(0), 500);
  assert.equal(reconnectDelay(1), 1000);
  assert.equal(reconnectDelay(2), 2000);
  assert.equal(reconnectDelay(20), 15000);
});

test("the push key from the server is read as bytes for subscribe", () => {
  // 0x04 then 64 bytes: a P-256 point, as the server sends it in base64url.
  const bytes = Uint8Array.from([4, ...Array.from({ length: 64 }, (_, i) => 250 - i)]);
  const b64 = Buffer.from(bytes).toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  assert.deepEqual(Array.from(applicationServerKey(b64)), Array.from(bytes));
});

test("a subscription is posted as its endpoint and two keys", () => {
  const sub = { toJSON: () => ({ endpoint: "https://fcm.googleapis.com/x", expirationTime: null, keys: { p256dh: "P", auth: "A" } }) };
  assert.deepEqual(pushBody(sub), { endpoint: "https://fcm.googleapis.com/x", keys: { p256dh: "P", auth: "A" } });
  assert.throws(() => pushBody({ toJSON: () => ({ endpoint: "https://x" }) }));
});

test("push needs a service worker, push and notifications in the browser", () => {
  assert.equal(pushSupported({}), false);
  assert.equal(pushSupported({ serviceWorker: {}, PushManager: {}, Notification: {} }), true);
  assert.equal(pushSupported({ serviceWorker: {}, PushManager: {} }), false);
});
