import assert from "node:assert/strict";
import { test } from "node:test";
import { added, MCP_SUGGESTIONS, pinned, type McpSuggestion } from "./mcpSuggestions.ts";

test("every suggestion is complete, secure and pinned", () => {
  assert.equal(new Set(MCP_SUGGESTIONS.map((s) => s.id)).size, MCP_SUGGESTIONS.length);
  for (const s of MCP_SUGGESTIONS) {
    assert.ok(s.name && s.description && s.reach, s.id);
    assert.match(s.docs, /^https:\/\//, s.id);
    if (s.transport === "http") {
      assert.match(s.url ?? "", /^https:\/\//, `${s.id} is reached over https`);
      assert.equal(s.command, undefined, s.id);
    } else {
      assert.ok(s.command, s.id);
      assert.ok(pinned(s), `${s.id} runs one exact version`);
    }
    if (s.signIn === "token") assert.ok(s.secretHeader, `${s.id} says where its token goes`);
  }
});

test("pinned refuses latest and ranges", () => {
  const local = (args: string[]): McpSuggestion => ({ id: "x", name: "x", description: "x", reach: "x", transport: "stdio", command: "npx", args, signIn: "none", docs: "https://x" });
  assert.equal(pinned(local(["-y", "@acme/mcp@1.2.3"])), true);
  assert.equal(pinned(local(["-y", "@acme/mcp@latest"])), false);
  assert.equal(pinned(local(["-y", "@acme/mcp"])), false);
  assert.equal(pinned(local(["-y", "@acme/mcp@^1.2.3"])), false);
});

test("a suggestion already set up is known by its ID, address or command", () => {
  const [github] = MCP_SUGGESTIONS;
  const playwright = MCP_SUGGESTIONS.find((s) => s.id === "playwright")!;
  assert.equal(added(github, []), false);
  assert.equal(added(github, [{ id: "github" }]), true);
  assert.equal(added(github, [{ id: "gh", url: "https://api.githubcopilot.com/mcp/" }]), true);
  assert.equal(added(playwright, [{ id: "pw", command: "npx", args: ["-y", "@playwright/mcp@0.0.84"] }]), true);
  assert.equal(added(playwright, [{ id: "pw", command: "npx", args: ["-y", "@playwright/mcp@0.0.80"] }]), false);
});
