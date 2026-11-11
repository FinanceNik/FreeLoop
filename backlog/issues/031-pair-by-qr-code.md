---
title: Pair a phone by scanning a QR code
status: open
type: human
blockedBy: []
prd: sessions-from-the-phone
---

## What
Settings → Phone shows the pairing link as a QR code beside the code, so the phone's camera opens it. The engine renders it as a PNG with `rsc.io/qr` (BSD-3, about 1,000 lines, no further dependencies), passed to the page as a data URL; the typed code stays.

## Acceptance
- The QR decodes to the shown link; Go test decoding the PNG.
- `make check-app` finds the QR image with an accessible name while a pairing window is open.

## Notes
- **Waits for the user to agree to `rsc.io/qr`** (CLAUDE.md: ask before adding a dependency); asked on 2026-11-11, no answer yet. Once they agree, an agent can do it; if they decline, close it as not done.
