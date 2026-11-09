import assert from "node:assert/strict";
import { test } from "node:test";
import { placeholder } from "./composer.ts";
import { setLanguage } from "./i18n.ts";

test("a pane the user is not in gets the short placeholder", () => {
  assert.equal(placeholder(false, false), "Ask for a change…");
  assert.equal(placeholder(true, false), "Queue a message");
  assert.ok(placeholder(false, true).includes("Type @"));
});

// GUI-048: the placeholder follows the language, and still names @ and /.
test("the placeholder is in the language set", () => {
  setLanguage("de");
  assert.equal(placeholder(false, true), "Um eine Änderung, eine Korrektur, eine Erklärung bitten… @ hängt eine Datei an, / zeigt die Befehle.");
  assert.equal(placeholder(false, false), "Um eine Änderung bitten…");
  assert.equal(placeholder(true, false), "Nachricht einreihen");
  setLanguage("nl");
  assert.equal(placeholder(true, true), "Bericht in de wachtrij zetten voor het einde van deze beurt");
  setLanguage("en");
  assert.equal(placeholder(false, true), "Ask for a change, a fix, an explanation… Type @ to attach a file, / for commands.");
  assert.equal(placeholder(true, true), "Queue a message for when this turn ends");
});
