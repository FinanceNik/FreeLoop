---
title: The sidebar, the session view and the composer in German and Dutch
status: done
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

## Done
- The app's own modes moved from `api.ts` to `commands.ts` (`modeName`, `modeHint`, `modeCommands()`; `api.ts`
  re-exports them). A mode the app does not know, such as an agent's own, shows as given.
- Also translated, as these views use them: `shortcuts.ts` (the approval buttons, a form's Send and Skip, the
  key hint), so the attention view's buttons follow too; `a11y.ts` (the window title's screen names, the spoken
  tool status, a picker's place); and the palette entries `App.tsx` builds. The headings in `palette.ts` and the
  other views' palette entries are left to 004.
- `i18nSource.test.ts` reads every file of these views. `review.ts` is left out because its message goes to the
  agent; `pdfview.ts` because it builds the PDF frame's page.
- English is the same as before, with two small exceptions: "1 character" no longer reads "1 characters", and
  counts from 1,000 get digit grouping. Some English slips stay because the one and other forms are equal
  ("2 error", "Archive 1 finished backlog files"); German and Dutch have both forms.
- A decision tag still reads "denied by protected_path" in English, since the app check looks for it; German and
  Dutch name who decided. The Info tab's Status shows the engine's id (`waiting_permission`), as before.
- A collapsed date group in the sidebar is now remembered by id (`date:today`), so one collapsed before opens once.
