---
title: The sidebar, the session view and the composer in German and Dutch
status: open
type: afk
blockedBy: [001-language-setting-and-settings-page]
prd: german-and-dutch-interface
---

## What
The parts of the window used all day are translated: sidebar and its footer, the session header, transcript
chrome (tool calls, thinking, permission prompts, queued messages), the composer and its controls, the right
panel, the Diff and Files tabs, and the app's own slash commands and mode names.

## Acceptance
- Every string Freeloop writes in these views comes from the catalogues.
- Text from agents and files is shown as given: modes and commands an agent advertises, workflow names and
  stage titles, skill and MCP descriptions, profile names.
- `make check-app` in Deutsch finds the composer's placeholder and the app's own mode names in German.

## Notes
- The existing `make check-app` steps run in English and must keep passing unchanged.
