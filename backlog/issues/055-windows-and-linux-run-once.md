---
title: Windows and Linux are run once on a real machine
status: open
type: human
blockedBy: []
prd: before-going-public
---

## What
OQ-045 says macOS, Windows and Linux work essentially the same before the repository goes public. The Windows code was cross-compiled and vetted on a Mac and has never run on Windows; the window has only ever been driven in macOS WebKit. Someone installs the package on a Windows 11 machine and on a Linux desktop and runs the demo and one real session on each.

## Acceptance
- On each: install, first run, the demo to its end, a session on a real agent with one approval, a terminal tab, quit and reopen with the session there.
- The conversation follows its end while streaming and holds its place when scrolled up (Chromium anchors scrolling itself, which `drawOnce` was not written for); a PDF opens; the message box grows with its text.
- What fails becomes an issue; what blocks a first session is fixed before 056.
- If no machine is at hand: the README says in its first screen that Windows and Linux are untested, and OQ-045 gets a note that the repository went public that way.

## Notes
- A virtual machine counts. Needs a person.
