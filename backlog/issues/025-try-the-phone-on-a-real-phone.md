---
title: Try the phone on a real iPhone over Tailscale
status: open
type: human
blockedBy: []
prd: sessions-from-the-phone
---

## What
In a rebuilt app, with Tailscale on the Mac and the iPhone, set up the phone as Settings → Phone says, pair the iPhone over cellular, add the app to the home screen, close Freeloop's window and work a real session from the phone.

## Acceptance
- The shown `tailscale serve` command works as given, and the page offers the computer's tailnet name.
- Pairing from the link works, and the computer asks before the phone gets in.
- In the app added to the home screen the phone is still paired, or pairs again there; which it was is written into this issue.
- A Claude Code request is allowed from the phone and the agent goes on; the desktop shows "From the phone".
- A message sent from the phone reaches the session; Stop stops a running turn.
- With the window closed the app keeps answering; with "Keep this computer awake" on, the Mac does not sleep while the session works.
- Removing the phone in Settings signs it out at its next request.

## Notes
- Needs a person, an iPhone and Tailscale on both.
- Anything that goes wrong becomes an issue of its own.
