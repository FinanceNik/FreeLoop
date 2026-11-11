---
title: The phone app installs on the home screen
status: open
type: afk
blockedBy: []
prd: sessions-from-the-phone
---

## What
The phone app gets a web app manifest (`display: standalone`, name, colours, icons made from `build/darwin/icons`), Apple's touch icon and a service worker under `/phone/` that caches nothing and only exists for push (issue 029). The phone server serves them with the right types; the CSP allows the worker. The Settings tab of the phone app says how to add it to the home screen, and the app pairs itself when opened from there without a cookie.

## Acceptance
- `/phone/manifest.webmanifest` and the icons are served with their content types, and `/phone/sw.js` registers with scope `/phone/`; Go test of the server, unit test of the manifest's fields.
- The worker answers no fetch and stores nothing; unit test.
- Opened without a cookie, the app shows pairing even with no `#pair=` in the address; existing test still passes.
- `make check-app` still passes; the desktop build does not register the worker.

## Notes
- iOS delivers push only to a web app added to the home screen (iOS 16.4+).
- Decision 10 of the PRD; GUI-052.
