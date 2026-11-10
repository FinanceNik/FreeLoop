---
title: The menu bar and system notifications in German and Dutch
status: done
type: afk
blockedBy: [001-language-setting-and-settings-page]
prd: german-and-dutch-interface
---

## What
The macOS menu bar and the system notifications follow the chosen language. Notifications are translated
in the desktop layer by their notice kind, so the engine stays language-neutral; the menu is rebuilt when
the language changes.

## Acceptance
- A notice of each kind gets its title and body in English, German and Dutch (Go test).
- The menu's own titles (such as View) change with `ui.language` without a restart (Go test).
- An unknown notice kind falls back to the engine's English text rather than showing nothing.
- No macOS-only code outside `internal/platform/darwin` (the architecture test passes).

## Notes
- The menu is built in the desktop layer of `cmd/freeloop`; system resolution of `system` on the Go side
  must agree with the frontend's.

## Done
- The window resolves `ui.language` (system included) and tells the desktop layer the result
  (`EngineService.SetInterfaceLanguage`), so the two cannot disagree; the menu is built again when it changes.
- Notices now carry what happened (`Session`, `Detail`, `StopReason`, `Budget`) beside the engine's English
  `Title` and `Body`; `cmd/freeloop/words.go` writes them per language, and an unknown kind, reason or
  language shows the English.
- Wails' standard menus are relabelled after building (`localizeMenu`); a Mac's own names (Ablage, Archief)
  are in `internal/platform/darwin/menu.go`.
- Found on the way: Wails' Help menu had one item, Learn More, which loaded wails.io into the app's window.
  The Help menu now has Freeloop Help, which opens the app's own help.
- Not checked by a test: the native menu's look on a Mac, Windows and Linux; that is for the review in 008.
