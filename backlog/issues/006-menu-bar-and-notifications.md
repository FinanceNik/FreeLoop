---
title: The menu bar and system notifications in German and Dutch
status: open
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
