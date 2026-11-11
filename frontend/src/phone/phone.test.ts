import { test } from "node:test";
import assert from "node:assert/strict";
import { deviceName, onceOptions, pairCode, pickLanguage } from "./phone.ts";

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
