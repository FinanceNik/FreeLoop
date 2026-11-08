---
title: The interface in German and Dutch, chosen in Settings
status: open
---

## Problem
Freeloop speaks only English. Every label, button, setting, help page, menu item and notification is
hard-coded English text, and numbers and dates follow whatever locale WebKit happens to report. German- and
Dutch-speaking users who would rather work in their own language cannot, and there is no translation layer
to add a language to: every new string is written straight into a component.

## Solution
Settings gets a **Language** picker beside Theme: System, English, Deutsch, Nederlands. System follows the
languages macOS prefers and falls back to English. A choice applies at once, without a restart, and is kept
in `config.yaml` as `ui.language`. In German or Dutch, everything Freeloop itself writes is in that language:
the interface, the help pages, the app's own mode names, the macOS menu bar and system notifications.
Numbers, dates and times follow the language too. What comes from models, agents and files is shown as given.

## User stories
- As a German-speaking developer, I want the whole interface in German, so that I read the app in my own
  language.
- As a Dutch-speaking developer, I want the same in Dutch.
- As a user whose Mac is set to German, I want Freeloop in German from its first start, without looking for
  a setting.
- As a user who prefers English tools on a German Mac, I want to pick English and have it stay English.
- As a user, I want numbers, dates and times written the way my language writes them (1.250, 18:30, 8. Nov.).
- As a user, I want notifications and the menu bar in the same language as the window.
- As a contributor, I want the build to fail when a string is missing from a language or typed into a
  component untranslated, so that no language falls behind silently.

## Decisions
1. **Interface only.** Engine error details, agent output, workflow prompts and skills stay English: models
   work best in English, and three copies of every prompt would drift.
2. **The setting.** `ui.language` in `config.yaml`, one of `system`, `en`, `de`, `nl`, validated like
   `ui.theme`; picked beside Theme in Settings; default `system`, resolved from macOS's preferred languages,
   falling back to English. A change applies immediately, like Theme.
3. **Our own small translation module, no dependency.** An English catalogue is the source; the German and
   Dutch catalogues are typed against it, so a missing or extra key or a mismatched `{placeholder}` fails the
   type check. One function, `t(key, vars)`, with plurals from the platform's `Intl.PluralRules`. No i18n
   library (the project keeps npm dependencies few).
4. **Neutral tone.** German and Dutch text addresses no one (neither du/Sie nor je/u). Terms developers say
   in English stay English: Commit, Branch, Token, Prompt, Worktree.
5. **Formats follow the language.** The translation module owns the locale, and every number, date and time
   formatter takes it from there, including the app's own token and chart-axis formatters. Costs stay in US
   dollars, since provider prices are in dollars, written in the locale's style (146,51 $).
6. **Native surfaces too.** The menu bar and system notifications are translated from a small catalogue in
   the desktop layer. Notifications are translated by their notice kind, so the engine's notices stay
   language-neutral. The menu is rebuilt when the language changes.
7. **Only Freeloop's own text.** The help pages and the app's own mode names are translated. Text from files
   or agents (workflow files including the built-in one, modes and commands an agent advertises, skill and
   MCP descriptions, profile names) is shown as given, so the interface never special-cases a harness
   (HRN-001).

## How it is tested
- **Type check:** the German and Dutch catalogues have exactly the English keys and placeholders.
- **Unit tests** of the translation module from its interface: `t` fills placeholders and picks plural
  forms per language; `system` resolves to German, Dutch or English from a list of preferred languages and
  falls back to English for anything else; number, date, time and cost formatting per language.
- **Config tests:** `ui.language` accepts the four values and reports any other as an issue, as `ui.theme`
  does.
- **Source test:** fails on English text written directly in a component instead of through the catalogue.
- **Desktop tests:** a notice of each kind gets its title and body in the chosen language; the menu's
  titles change with the language.
- **App check:** `make check-app` runs in English as today, then switches to Deutsch and to Nederlands and
  checks that the page's `lang` changes, that landmark labels appear translated without a restart, and that
  no control or heading in the main views (sidebar, session, composer controls, Settings, usage pane) is
  clipped, since German labels run about a third longer.

## Out of scope
- Languages other than German and Dutch; each later one is a catalogue and a picker entry.
- Right-to-left layouts.
- Translation files supplied by users in the data folder; the catalogues ship inside the app.
- Translating engine errors, model or agent output, prompts, skills, or anything from files.
- Translated README, docs or website.

## Open questions
None.
