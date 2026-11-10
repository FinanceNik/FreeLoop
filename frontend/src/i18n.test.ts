import assert from "node:assert/strict";
import { test } from "node:test";
import { CATALOGUES, LANGUAGES, language, list, locale, resolveLanguage, setLanguage, t, tParts } from "./i18n.ts";

test("t fills placeholders in the language set", () => {
  setLanguage("en");
  assert.equal(t("settings.updates.available", { version: "1.2.0" }), "Version 1.2.0 is available.");
  setLanguage("de");
  assert.equal(t("settings.updates.available", { version: "1.2.0" }), "Version 1.2.0 ist verfügbar.");
  setLanguage("nl");
  assert.equal(t("settings.updates.available", { version: "1.2.0" }), "Versie 1.2.0 is beschikbaar.");
  setLanguage("en");
});

test("t picks the plural form the language's rules give for a count", () => {
  const blocked = (lang: "en" | "de" | "nl", count: number) => {
    setLanguage(lang);
    return t("settings.network.blocked", { count });
  };
  assert.equal(blocked("en", 1), "1 blocked connection");
  assert.equal(blocked("en", 0), "0 blocked connections");
  assert.equal(blocked("en", 3), "3 blocked connections");
  assert.equal(blocked("de", 1), "1 blockierte Verbindung");
  assert.equal(blocked("de", 3), "3 blockierte Verbindungen");
  assert.equal(blocked("nl", 1), "1 geblokkeerde verbinding");
  assert.equal(blocked("nl", 3), "3 geblokkeerde verbindingen");
  // The forms come from Intl.PluralRules, so a count the rules call "other" in a
  // language gets that form, whole number or not.
  assert.equal(blocked("en", 1.5), "1.5 blocked connections");
  setLanguage("en");
});

test("tParts keeps what is not text, such as an element, where its placeholder is", () => {
  setLanguage("de");
  const el = { element: "code" };
  const parts = tParts("settings.web.intro", { tool: el });
  assert.ok(parts.includes(el));
  assert.equal(parts.filter((p) => p === el).length, 1);
  assert.ok(typeof parts[0] === "string" && parts[0].length > 0);
  setLanguage("en");
});

test("list joins names as the language does", () => {
  assert.equal(list([]), "");
  assert.equal(list(["Hooks"]), "Hooks");
  assert.equal(list(["Hooks", "Prices", "Budgets"]), "Hooks, Prices and Budgets");
  setLanguage("de");
  assert.equal(list(["A", "B", "C"]), "A, B und C");
  setLanguage("nl");
  assert.equal(list(["A", "B"]), "A en B");
  setLanguage("en");
});

test("system follows the first preferred language the app has, else English", () => {
  assert.equal(resolveLanguage("system", ["de-AT"]), "de");
  assert.equal(resolveLanguage("system", ["nl-BE"]), "nl");
  assert.equal(resolveLanguage("system", ["fr"]), "en");
  assert.equal(resolveLanguage("system", ["fr-FR", "nl-NL", "de-DE"]), "nl");
  assert.equal(resolveLanguage("system", ["en-GB", "de-DE"]), "en");
  assert.equal(resolveLanguage("system", ["DE_ch"]), "de");
  assert.equal(resolveLanguage("system", []), "en");
  // A missing or unknown setting means system.
  assert.equal(resolveLanguage(undefined, ["de-DE"]), "de");
  assert.equal(resolveLanguage("", ["nl"]), "nl");
  assert.equal(resolveLanguage("klingon", ["de"]), "de");
  // A language chosen wins over the system's.
  assert.equal(resolveLanguage("en", ["de-DE"]), "en");
  assert.equal(resolveLanguage("nl", ["de-DE"]), "nl");
  assert.equal(resolveLanguage("de", []), "de");
});

test("the language set is the one read back", () => {
  setLanguage("nl");
  assert.equal(language(), "nl");
  setLanguage("en");
  assert.equal(language(), "en");
});

test("the locale is the language, in the region the system prefers for it", () => {
  setLanguage("de");
  assert.equal(locale(), "de");
  setLanguage("en", ["en-GB", "de-DE"]);
  assert.equal(locale(), "en-GB");
  setLanguage("de", ["en-GB", "de-AT"]);
  assert.equal(locale(), "de-AT");
  setLanguage("nl", ["nl_BE"]);
  assert.equal(locale(), "nl-BE");
  // A language the system does not list is written as the language writes it.
  setLanguage("nl", ["en-US", "de-DE"]);
  assert.equal(locale(), "nl");
  setLanguage("de", ["not a tag", "de-"]);
  assert.equal(locale(), "de");
  setLanguage("en");
  assert.equal(locale(), "en");
});

test("t writes a number in the locale's style", () => {
  setLanguage("en");
  assert.equal(t("settings.network.blocked", { count: 1250 }), "1,250 blocked connections");
  setLanguage("de");
  assert.equal(t("settings.network.blocked", { count: 1250 }), "1.250 blockierte Verbindungen");
  assert.equal(t("settings.fontSize", { size: 13.5 }), "Schriftgröße: 13,5 pt");
  setLanguage("nl");
  assert.equal(t("settings.network.blocked", { count: 1250 }), "1.250 geblokkeerde verbindingen");
  setLanguage("en");
});

test("the picker names each language in itself", () => {
  assert.deepEqual(
    LANGUAGES.map((l) => [l.id, l.name]),
    [
      ["en", "English"],
      ["de", "Deutsch"],
      ["nl", "Nederlands"],
    ],
  );
});

// The type check holds the catalogues to the English one (i18n.typetest.ts); this
// says the same at run time, for a catalogue built some other way.
const holes = (entry: unknown): string[] =>
  [...new Set(Object.values(typeof entry === "string" ? { text: entry } : (entry as object)).flatMap((s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1])))].sort();

test("each catalogue has the English keys, forms and placeholders", () => {
  const en = CATALOGUES.en;
  for (const lang of ["de", "nl"] as const) {
    const c = CATALOGUES[lang] as Record<string, unknown>;
    assert.deepEqual(Object.keys(c).sort(), Object.keys(en).sort(), lang);
    for (const [key, entry] of Object.entries(en)) {
      assert.equal(typeof c[key], typeof entry, `${lang} ${key}`);
      assert.deepEqual(holes(c[key]), holes(entry), `${lang} ${key}`);
    }
  }
});

// Entries spelt as in English because the word is the language's own too, not English
// left over: German says Agent, Text, Code, Status, Terminal, Branch, Name and Host, Dutch
// Agent, Code, Model, Project, Context, Tokens and Servers, all OK.
const SAME_WORD = {
  de: new Set([
    "afk.cost.issues",
    "afk.cost.reviews",
    "modelSets.row.issues",
    "modelSets.row.reviews",
    "modelSets.name",
    "settings.build.version",
    "common.ok",
    "pending.kind.agent",
    "colors.token.text",
    "fonts.text",
    "fonts.code",
    "sidebar.filter.status",
    "sidebar.filter.tag",
    "sidebar.savedFilter.namePlaceholder",
    "search.hit.agent",
    "tags.label",
    "tags.add",
    "app.board",
    "session.item.agent",
    "plan.versionLabel",
    "panel.tab.plan",
    "panel.tab.diff",
    "panel.tab.terminal",
    "panel.tab.info",
    "panel.info.status",
    "panel.info.worktree",
    "panel.info.branch",
    "panel.info.ports",
    "panel.scope.global",
    "terminal.list",
    "terminal.shell",
    "terminal.label",
    "diff.commit.title",
    "costSummary.agent",
    "liveUsage.filter",
    "limits.col.limit",
    "board.backlog",
    "board.lane.backlog",
    "board.detail.kind.review",
    "backlog.type.afk",
    "schedules.form.name",
    "schedules.form.prompt",
    "newSession.optional",
    "profiles.yaml",
    "profiles.id",
    "profiles.name",
    "profiles.commandPrompt",
    "environments.name",
    "environments.secretName",
    "environments.sandbox",
    "environments.container",
    "environments.host",
    "mcp.form.id",
    "mcp.form.name",
    "mcp.form.url",
    "skills.create.name",
    "providers.name",
    "sessionImport.agent",
  ]),
  nl: new Set([
    "modelSets.row.issues",
    "modelSets.row.reviews",
    "modelSets.model",
    "afk.cost.planning",
    "afk.cost.issues",
    "afk.cost.reviews",
    "afk.cost.commit",
    "settings.build.version",
    "common.ok",
    "pending.kind.agent",
    "fonts.code",
    "settings.defaults.model",
    "prices.model",
    "usage.model",
    "sidebar.filters",
    "sidebar.groupBy.project",
    "sidebar.filter.status",
    "sidebar.filter.tag",
    "search.hit.agent",
    "tags.label",
    "tags.add",
    "app.help",
    "session.item.agent",
    "tool.via",
    "tool.viaCommand",
    "panel.tab.plan",
    "panel.tab.diff",
    "panel.tab.terminal",
    "panel.tab.info",
    "panel.info.status",
    "panel.info.project",
    "panel.info.worktree",
    "panel.info.branch",
    "panel.info.context",
    "panel.info.contextTokens",
    "panel.rules.effect",
    "panel.scope.project",
    "terminal.list",
    "terminal.shell",
    "terminal.label",
    "diff.commit.title",
    "diff.file.agentEdit",
    "pr.title",
    "usagePage.chart.tokensWeek",
    "insights.activity.pullRequests",
    "insights.efficiency.model",
    "insights.efficiency.project",
    "tokenFigures.tokens",
    "costSummary.agent",
    "costSummary.model",
    "costSummary.tokens",
    "usagePane.view.tokens",
    "liveUsage.tokens",
    "liveUsage.filter",
    "liveUsage.strip",
    "board.backlog",
    "board.lane.backlog",
    "board.detail.kind.review",
    "board.issueCount",
    "board.filter.label",
    "backlog.type.afk",
    "queue.project",
    "schedules.form.project",
    "schedules.form.prompt",
    "schedules.form.model",
    "profiles.yaml",
    "profiles.id",
    "profiles.commandPrompt",
    "environments.sandbox",
    "environments.container",
    "environments.host",
    "mcp.form.id",
    "mcp.form.url",
    "mcp.form.headers",
    "mcp.registry.servers",
    "skills.source.project",
    "skills.project",
    "claudeImport.scripts",
    "sessionImport.agent",
  ]),
};

test("German and Dutch translate every entry", () => {
  const en = CATALOGUES.en as Record<string, unknown>;
  for (const lang of ["de", "nl"] as const) {
    const c = CATALOGUES[lang] as Record<string, unknown>;
    const same = Object.keys(en).filter((k) => !SAME_WORD[lang].has(k) && JSON.stringify(c[k]) === JSON.stringify(en[k]));
    assert.deepEqual(same, [], `${lang} entries still in English`);
  }
});

test("German and Dutch text addresses no one", () => {
  const de = Object.values(CATALOGUES.de).flatMap((e) => (typeof e === "string" ? [e] : Object.values(e)));
  const nl = Object.values(CATALOGUES.nl).flatMap((e) => (typeof e === "string" ? [e] : Object.values(e)));
  // Capital Sie and Ihr also start a sentence about "they"; the catalogues word those
  // another way, so the formal you cannot hide behind them.
  const deYou = de.filter((s) => /\b(du|dich|dir|dein\w*|Sie|Ihr\w*|Ihnen)\b/.test(s));
  const nlYou = nl.filter((s) => /\b(je|jij|jou|jouw|u|uw)\b/i.test(s));
  assert.deepEqual(deYou, []);
  assert.deepEqual(nlYou, []);
});
