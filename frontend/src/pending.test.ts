import { test } from "node:test";
import assert from "node:assert/strict";
import { approveAllQuestion, groupPending, pendingLabel } from "./pending.ts";
import { setLanguage } from "./i18n.ts";

test("groups keep every item, commands first", () => {
  const items = [{ kind: "address" }, { kind: "rule" }, { kind: "hook" }, { kind: "agent" }, { kind: "something-new" }];
  const groups = groupPending(items);
  assert.deepEqual(
    groups.map((g) => g.title),
    ["Commands that would run", "Rules that would allow without asking", "Settings", "Where things would be sent"],
  );
  assert.equal(groups.flatMap((g) => g.items).length, items.length);
  assert.deepEqual(groups[2].items, [{ kind: "something-new" }]);
});

test("labels name a hook's event", () => {
  assert.equal(pendingLabel({ kind: "hook", event: "pre_tool_use" }), "Hook (Before a tool call)");
  assert.equal(pendingLabel({ kind: "hook", event: "later" }), "Hook (later)");
  assert.equal(pendingLabel({ kind: "something-new", event: "" }), "Setting");
  assert.equal(pendingLabel({ kind: "project_hooks", event: "" }), "A trusted project's changed hooks");
  assert.equal(groupPending([{ kind: "project_hooks" }])[0].title, "Commands that would run");
});

test("approving all says how many of what", () => {
  const q = approveAllQuestion([
    { kind: "hook" },
    { kind: "agent" },
    { kind: "rule" },
    { kind: "address" },
  ]);
  assert.match(q, /Approve all 4\?/);
  assert.match(q, /Commands that would run: 2/);
  assert.match(q, /Rules that would allow without asking: 1/);
  assert.match(q, /Where things would be sent: 1/);
  assert.doesNotMatch(q, /Environments/);
});

test("what waits is told in the language in force (GUI-048)", () => {
  setLanguage("de");
  assert.equal(pendingLabel({ kind: "hook", event: "pre_tool_use" }), "Hook-Befehl (Vor einem Werkzeugaufruf)");
  assert.deepEqual(
    groupPending([{ kind: "rule" }, { kind: "something-new" }]).map((g) => g.title),
    ["Regeln, die ohne Rückfrage erlauben würden", "Einstellungen"],
  );
  assert.match(approveAllQuestion([{ kind: "hook" }, { kind: "agent" }]), /^Alle 2 freigeben\?\n\nBefehle, die laufen würden: 2\n\n/);
  setLanguage("en");
});
