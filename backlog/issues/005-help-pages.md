---
title: The help pages in German and Dutch
status: done
type: afk
blockedBy: [001-language-setting-and-settings-page]
prd: german-and-dutch-interface
---

## What
The help pages (GUI-047) are shown in the chosen language. All of them are translated; none falls back to
English.

## Acceptance
- Each help page has German and Dutch text; the existing help tests run for all three languages.
- Keyboard shortcuts, commands and file names in the pages are unchanged across languages.
- Switching the language while a help page is open shows it in the new language.

## Done
- Pages for German and Dutch are in `frontend/src/help/de/` and `help/nl/`; `Help.tsx` picks them by the
  language in force, each falling back to English, and the topic titles and buttons come from the catalogues.
- `help.test.ts` holds every translation to the English page's headings, table rows, code, shortcuts, links
  and per-system choices; a click is written in the page's language (⌘-Klick, ⌘-klik), which `forPlatform`
  turns into Ctrl-Klick and Ctrl-klik on Windows and Linux.
- Names that come from the engine or from files stay English in the pages, as in the interface: the built-in
  environments Local and Local, sandboxed, the built-in workflow, the Default profile, provider kinds.
- The troubleshooting page names macOS's and Windows' own buttons in German and Dutch, as those systems show
  them; worth a look in 008.
