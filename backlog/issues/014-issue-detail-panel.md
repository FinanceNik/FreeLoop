---
title: Clicking a card opens a read-only panel with the issue's text and sessions
status: done
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

## Done
- `Engine.IssueDetail` finds the issue by its id among the issues `projectBacklog` lists (so a path, `..` or an empty id reads nothing), reads its file through the environment and returns the body after the frontmatter, what it waits on and holds up, and the sessions of the project's runs for it: each step's session, then its review, newest step first. `TestIssueDetail` covers an untrusted project, the ids, and the order.
- The panel stands beside the Board (`IssuePanel.tsx`), takes the keyboard on its Close button, closes on Escape or the button and gives the focus back to what had it. A card opens it from its title, which is a button, or from a click on the card that is not on another button. The text goes through `Markdown`; the check puts `<img onerror>` in an issue file and sees it as text.
- Not in the panel, by design: Mark done and Run this issue.
