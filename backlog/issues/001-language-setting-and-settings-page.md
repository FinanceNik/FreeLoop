---
title: Choose a language in Settings; the Settings page follows it
status: done
type: afk
blockedBy: []
prd: german-and-dutch-interface
---

## What
Settings has a Language picker beside Theme (System, English, Deutsch, Nederlands). Picking one changes the
Settings page itself into that language at once, without a restart, and the choice survives a restart.
System follows macOS's preferred languages and falls back to English. This is the tracer bullet: setting,
translation module, catalogues and one fully translated page.

## Acceptance
- `ui.language` in `config.yaml` accepts `system`, `en`, `de`, `nl`; any other value is reported as an
  issue on `ui.language`, as `ui.theme` is; a missing value means `system`.
- The translation module offers `t(key, vars)` over an English catalogue; the German and Dutch catalogues
  are typed against it, so a missing or extra key or a mismatched `{placeholder}` fails `tsc`.
- `t` fills placeholders and picks plural forms per language with `Intl.PluralRules` (unit test).
- `system` resolves to `de`, `nl` or `en` from a list of preferred languages (`de-AT` is German,
  `nl-BE` Dutch, `fr` English) (unit test).
- Every string on the Settings page comes from the catalogues; in Deutsch and Nederlands none is English
  apart from the developer terms the PRD keeps (Commit, Branch, Token, Prompt, Worktree).
- The page's `<html lang>` follows the language.
- `make check-app` picks Deutsch, sees the Settings heading in German without a reload, picks English again.

## Notes
- No new npm dependency. Neutral tone: German and Dutch text addresses no one.
- Add the requirement to docs/REQUIREMENTS.md under the next free GUI ID (GUI-048 when this was written) and
  use that ID in commits.
- The setting sits beside Theme in Settings ("Theme and text size").

## Done
- `system` reads `navigator.languages`, and WebKit gives only the first preferred language there (a Mac
  set to French, then Dutch, gets English, where a native app would pick Dutch). Issue 006 resolves
  `system` on the Go side anyway; passing the full list from there would close the gap.
- `secretStore()` and `inFileManager()` are translated at the source, so the views still in English
  show those few words in German or Dutch inside English sentences until 003 and 004.
- The app check pins `language: en` in its scratch data, so it reads English on any Mac.
- `i18nSource.test.ts` lists the components already translated; later issues add theirs to it.
