import assert from "node:assert/strict";
import { test } from "node:test";
import { headline, ruleWhere, toQuery } from "./policy.ts";

test("a command goes in command, anything else in path", () => {
  const shell = toQuery({ where: "project:/Users/me/app", mode: "", tool: "shell", what: " git push " });
  assert.equal(shell.projectRoot, "/Users/me/app");
  assert.equal(shell.sessionId, "");
  assert.equal(shell.command, "git push");
  assert.equal(shell.path, "");
  const edit = toQuery({ where: "session:ses_1", mode: "plan", tool: "edit", what: "a.go" });
  assert.equal(edit.sessionId, "ses_1");
  assert.equal(edit.mode, "plan");
  assert.equal(edit.path, "a.go");
  assert.equal(edit.command, "");
});

test("a root with a colon stays whole", () => {
  assert.equal(toQuery({ where: "project:/a:b", mode: "", tool: "read", what: "x" }).projectRoot, "/a:b");
});

test("the verdict says what happens, then why", () => {
  assert.deepEqual(headline({ outcome: "denied", reason: "deny shell(git push *) (global rule)" }), { tone: "error", text: "Denied. Deny shell(git push *) (global rule)." });
  assert.equal(headline({ outcome: "ask", reason: "no rule or mode decides it, so the user is asked" }).tone, "warn");
  assert.equal(headline({ outcome: "allowed", reason: "bypass mode" }).text, "Allowed. Bypass mode.");
});

test("a rule says where it lives", () => {
  assert.equal(ruleWhere({ scope: "global", source: "/x/rules.yaml" }), "global, /x/rules.yaml");
  assert.equal(ruleWhere({ scope: "session", source: "" }), "this session");
});
