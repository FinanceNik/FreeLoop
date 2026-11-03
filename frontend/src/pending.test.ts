import { test } from "node:test";
import assert from "node:assert/strict";
import { approveAllQuestion, groupPending, pendingLabel } from "./pending.ts";

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
