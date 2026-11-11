import { test } from "node:test";
import assert from "node:assert/strict";
import { isChanged, lookOf, sameLook, selectedStyle, styleColors, styleFonts } from "./styles.ts";

const paper = { id: "paper", name: "Paper", textFont: "Inter", codeFont: "", light: { accent: "#7c3aed", bg: "#ffffff" }, dark: null };
const night = { id: "night", name: "Night", textFont: "", codeFont: "Menlo", light: {}, dark: { bg: "#000000" } };

test("a look is the colours of both appearances and the two fonts", () => {
  const look = lookOf({ light: { accent: "#7c3aed" } }, { textFont: "Inter", codeFont: undefined, fontSize: 16 });
  assert.deepEqual(look, { light: { accent: "#7c3aed" }, dark: {}, textFont: "Inter", codeFont: "" });
});

test("looks compare by value, whatever the order and however empty is written", () => {
  const a = lookOf({ light: { bg: "#ffffff", accent: "#7c3aed" }, dark: null }, { textFont: "Inter" });
  const b = lookOf({ light: { accent: "#7c3aed", bg: "#ffffff" }, dark: {} }, { textFont: "Inter", codeFont: "" });
  assert.ok(sameLook(a, b));
  assert.ok(!sameLook(a, { ...b, codeFont: "Menlo" }));
  assert.ok(!sameLook(a, { ...b, light: { accent: "#7c3aed" } }));
  assert.ok(!sameLook(a, { ...b, light: { accent: "#7c3aed", bg: "#fffffe" } }));
  // An entry the bindings type as possibly missing counts as not set.
  assert.ok(sameLook(lookOf({ light: { accent: undefined as unknown as string } }, {}), lookOf({}, {})));
});

test("the style chosen is found by its ID, and a missing one is the app's own look", () => {
  assert.equal(selectedStyle([paper, night], "night"), night);
  assert.equal(selectedStyle([paper, night], ""), null);
  assert.equal(selectedStyle([paper, night], undefined), null);
  assert.equal(selectedStyle([paper, night], "gone"), null);
  assert.equal(selectedStyle(null, "paper"), null);
});

test("a style is changed when the look on screen is not what it holds", () => {
  const onScreen = lookOf({ light: { bg: "#ffffff", accent: "#7c3aed" } }, { textFont: "Inter" });
  assert.ok(!isChanged(onScreen, paper));
  assert.ok(isChanged({ ...onScreen, textFont: "" }, paper));
  assert.ok(isChanged(onScreen, night));
  // The app's own look is changed by any colour or font of the user's.
  assert.ok(!isChanged(lookOf({}, {}), null));
  assert.ok(isChanged(lookOf({ dark: { bg: "#000000" } }, {}), null));
  assert.ok(isChanged(lookOf({}, { codeFont: "Menlo" }), null));
});

test("applying a style puts all of its values in place, and the app's own clears them", () => {
  assert.deepEqual(styleColors(paper), { light: { accent: "#7c3aed", bg: "#ffffff" }, dark: {} });
  assert.deepEqual(styleFonts(paper), { textFont: "Inter", codeFont: "" });
  assert.deepEqual(styleColors(null), { light: {}, dark: {} });
  assert.deepEqual(styleFonts(null), { textFont: "", codeFont: "" });
  // What is applied is what the style holds, so it is not shown as changed.
  assert.ok(!isChanged(lookOf(styleColors(night), styleFonts(night)), night));
});
