---
title: Guard against untranslated text and clipped labels
status: done
type: afk
blockedBy: [002-formats-follow-the-language, 003-sidebar-session-and-composer, 004-remaining-views, 005-help-pages]
prd: german-and-dutch-interface
---

## What
Two guards keep the languages whole: a source test that fails on English text written straight into a
component, and German and Dutch passes in the app check that fail when a control or heading is clipped.
Any clipping they find is fixed in the layout.

## Acceptance
- A `node --test` source test fails on literal text in JSX or in user-facing attributes (`aria-label`,
  `title`, `placeholder`) outside the catalogues, with an allow-list for symbols and developer terms.
- `make check-app` switches to Deutsch and then Nederlands and walks the sidebar, a session, the composer
  controls, Settings and the usage pane, failing on any control or heading whose text overflows its box.
- Both pass on the current code.

## Done
- The source test now reads every file in `frontend/src/` but eight that hold no interface text, each listed
  with its reason (CSS, font names, SVG paths, the Markdown parser, languages' own names, the PDF frame, the
  systems' names, the review message to the agent); it found nothing new.
- The clipping step measures the app's own controls and headings (buttons, tabs, headings, labels, table
  headers, and anything set to end in an ellipsis) and leaves out text from users, agents and files.
