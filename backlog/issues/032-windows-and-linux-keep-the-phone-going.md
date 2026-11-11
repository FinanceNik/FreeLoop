---
title: The phone on Windows and Linux
status: open
type: human
blockedBy: [025-try-the-phone-on-a-real-phone]
prd: sessions-from-the-phone
---

## What
On a Windows PC and an Ubuntu 24.04 machine with Tailscale, turn on the phone, run the shown command (with `sudo` or `--operator` on Linux), pair a phone, minimise by closing the window, and leave it.

## Acceptance
- The Tailscale command is found and shown with the right path on each.
- Closing the window minimises the app, and the phone keeps working.
- With "Keep this computer awake" on, the computer does not sleep while a session works (Windows: `powercfg /requests` lists Freeloop; Linux: `systemd-inhibit --list` lists it).
- Removing the phone signs it out.

## Notes
- Built and compiled only on a Mac so far.
