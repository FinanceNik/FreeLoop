---
title: The remaining views in German and Dutch
status: done
type: afk
blockedBy: [001-language-setting-and-settings-page]
prd: german-and-dutch-interface
---

## What
Everything else Freeloop writes is translated: the Board and its Backlog column, the attention view, the
usage pages and pane, Workflows, Profiles, environments, MCP and skills pages, the new-session dialog,
onboarding, the AFK dialog, the command palette's own entries, error banners' headings, and empty states.

## Acceptance
- Every string Freeloop writes in these views comes from the catalogues.
- Engine error details are shown as given, under a translated heading.
- `make check-app` in Nederlands finds the attention view's and the Board's headings in Dutch.

## Done
- Every view in the issue reads from the catalogues; `i18nSource.test.ts` lists them all. Shown as given: engine
  and file text, names, example values in fields (`npx`, `mcp__github__*`, URLs), paths and vendors' names.
- Error banners say "Error:" or "Error in {title}:" in the language, then the engine's words as they came
  (`errorlist.ts`). `ErrorBoundary` and the "Could not … : {error}" lines of the views work the same way.
- `make check-app` has a step that picks Nederlands and finds "Vraagt aandacht" and "Bord" and the Board's columns.
- `WEEKDAYS` in `activity.ts` is a proxy over Intl's names for the locale, so views reading it follow the language;
  `weekdayName(i)` is for new code.
- Usage filters hold the text of a group's name, so they clear when the language changes.
- English grammar slips fixed by plural entries and `list()` ("1 tool", "A and B"); other English is unchanged.
- The palette's group ids stay English ids (`Approval`, `Action`, ...); `palette.ts` maps them to headings.
- `afk.step.reviewing` in Dutch reads "wordt beoordeeld", not "in review"; wording is for issue 008 to review.
