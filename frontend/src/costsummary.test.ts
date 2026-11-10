import { test } from "node:test";
import assert from "node:assert/strict";
import { costSummary, summaryText } from "./costsummary.ts";
import { setLanguage } from "./i18n.ts";

const row = (agent: string, model: string, input: number, cost: number, priced = true) => ({
  day: "2026-11-01",
  sessionId: "secret-session",
  providerId: agent,
  model,
  inputTokens: input,
  outputTokens: 0,
  cacheReadTokens: 0,
  cacheWriteTokens: 0,
  calls: 2,
  cost,
  priced,
  projectRoot: "/Users/me/secret-project",
});
const keys = { agent: (r: ReturnType<typeof row>) => r.providerId, model: (r: ReturnType<typeof row>) => r.model };
const range = { label: "30 days", since: "2026-10-05", until: "2026-11-03", pricesAsOf: "2026-11-03", filtered: false, cacheReads: true };

test("the summary pastes as tables, by agent and by model, with a total", () => {
  const text = summaryText(costSummary([row("Claude Code", "opus", 2000000, 30), row("OpenRouter", "qwen", 1000000, 0.15)], keys, range));
  const lines = text.split("\n");
  assert.equal(lines[0], "**Freeloop usage, last 30 days** (2026-10-05 to 2026-11-03)");
  assert.ok(lines.includes("| Agent | Tokens | Estimated cost |"));
  assert.ok(lines.includes("| Claude Code | 2M | $30.00 |"));
  assert.ok(lines.includes("| Model | Tokens | Estimated cost |"));
  assert.ok(lines.includes("| qwen | 1M | $0.15 |"));
  assert.ok(lines.includes("| **Total** | **3M** | **$30.15** |"));
  // Every table row has the same number of columns, or GitHub will not draw it.
  for (const l of lines.filter((x) => x.startsWith("|"))) assert.equal(l.split("|").length, 5, l);
});

test("it says what the numbers are, and names no session or project", () => {
  const text = summaryText(costSummary([row("Claude Code", "opus", 10, 1)], keys, range));
  assert.match(text, /list prices as of 2026-11-03/);
  assert.match(text, /subscription is shown at what the same work costs on the API/);
  assert.doesNotMatch(text, /secret/);
});

test("a model without a price says so, and the cost leaves it out", () => {
  const s = costSummary([row("A", "priced", 10, 1), row("A", "free-form", 10, 0, false)], keys, range);
  assert.equal(s.models.find((m) => m.name === "free-form")?.cost, "no price");
  assert.match(summaryText(s), /2 model calls on free-form have no price/);
  assert.equal(s.total.cost, "$1.00");
});

test("nothing priced says so rather than claiming nothing was spent", () => {
  const s = costSummary([row("A", "x", 10, 0, false)], keys, range);
  assert.equal(s.total.cost, "no price");
});

test("a pipe in a name cannot break the table, and long lists fold", () => {
  const text = summaryText(costSummary([row("A", "a|b", 10, 1)], keys, range));
  assert.ok(text.includes("| a\\|b |"));
  const many = Array.from({ length: 12 }, (_, i) => row("A", `m${i}`, 1000 - i, 1));
  const s = costSummary(many, keys, range);
  assert.equal(s.models.length, 9);
  assert.equal(s.models[8].name, "4 more");
  assert.equal(s.models[8].cost, "$4.00");
});

test("a filtered view and left-out cache reads are stated", () => {
  const text = summaryText(costSummary([row("A", "m", 10, 1)], keys, { ...range, filtered: true, cacheReads: false }));
  assert.match(text, /filtered part/);
  assert.match(text, /leave out cache reads/);
});

test("an empty range still makes a summary", () => {
  const s = costSummary([], keys, range);
  assert.equal(s.total.tokens, "0");
  assert.equal(s.total.cost, "no price");
});

// GUI-048: the summary is written in the language in force when it is made; names and
// the dates it is given are shown as they are.
test("the summary is in the language set, names as given", () => {
  setLanguage("de");
  const rows = [row("Claude Code", "opus", 2000000, 30), row("A", "free-form", 10, 0, false), row("A", "local", 10, 0, false)];
  const text = summaryText(costSummary(rows, keys, { ...range, label: "30 Tage", filtered: true, cacheReads: false }));
  const lines = text.split("\n");
  assert.equal(lines[0], "**Freeloop-Nutzung, letzte 30 Tage** (2026-10-05 bis 2026-11-03)");
  assert.ok(lines.includes("| Agent | Token | Geschätzte Kosten |"), text);
  assert.ok(lines.includes("| Modell | Token | Geschätzte Kosten |"), text);
  assert.ok(lines.includes("| Claude Code | 2\u00a0Mio. | 30,00\u00a0$ |"), text);
  assert.ok(lines.includes("| free-form | 10 | kein Preis |"), text);
  assert.ok(lines.includes("| **Gesamt** | **2\u00a0Mio.** | **30,00\u00a0$** |"), text);
  assert.ok(lines.includes("4 Aufrufe von free-form und local haben keinen Preis und fehlen daher in den Kosten."), text);
  assert.match(text, /zu Listenpreisen, Stand 2026-11-03\./);
  assert.match(text, /keine Lesezugriffe auf den Cache/);
  assert.match(text, /gefilterter Teil der Nutzung/);
  const many = Array.from({ length: 12 }, (_, i) => row("A", `m${i}`, 1000 - i, 1));
  assert.equal(costSummary(many, keys, range).models[8].name, "4 weitere");

  setLanguage("nl");
  const s = costSummary([{ ...row("A", "x", 10, 0, false), calls: 1 }], keys, range);
  assert.equal(s.total.name, "Totaal");
  assert.equal(s.total.cost, "geen prijs");
  assert.ok(s.notes.includes("1 aanroep van x heeft geen prijs en telt dus niet mee in de kosten."), s.notes.join("\n"));
  assert.ok(summaryText(s).split("\n").includes("| Model | Tokens | Geschatte kosten |"));
  assert.equal(costSummary(many, keys, range).models[8].name, "nog 4");
  setLanguage("en");
});
