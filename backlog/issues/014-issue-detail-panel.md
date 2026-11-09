---
title: Clicking a card opens a read-only panel with the issue's text and sessions
status: open
type: afk
blockedBy: [009-swimlanes-of-workflow-issues]
prd: workflow-board
---

## What
Clicking an issue's card opens a panel beside the board with the issue's title, its full text rendered as Markdown, its file, what it waits on and what it holds up, and the sessions that worked on and reviewed it, each opening that session. The panel has no actions that change the issue.

## Acceptance
- An engine call returns one issue's text from a trusted project's backlog, through the environment; refused for an untrusted project. Engine test.
- The text goes through the app's Markdown renderer, never as HTML.
- The panel lists the issue's sessions from the project's runs, newest first, each opening its session.
- Escape and a close button close it; it is reachable and closable with the keyboard alone.
- `make check-app` opens an issue's panel and finds its text.

## Notes
- No Mark done and no Run this issue: the board indicates only (PRD decisions 11 and 12).
