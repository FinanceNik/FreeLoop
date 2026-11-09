---
title: Untrusted projects with a backlog get a row to trust them; an empty board explains itself
status: open
type: afk
blockedBy: [009-swimlanes-of-workflow-issues]
prd: workflow-board
---

## What
An untrusted project that has a backlog folder gets a collapsed row reading "Not trusted: trust it to see its backlog", with a Trust project button. With no backlog in any project, the board says what it is for in two lines, with Start a workflow… and a link to the Workflows help.

## Acceptance
- The engine's list of backlogs includes untrusted projects whose backlog folder exists, marked untrusted, with no issues: it checks that the folder exists and reads nothing in it. An engine test covers both.
- Trust project on the row trusts the project (as the session view's trust does, asking first when the project has hooks) and the row fills in.
- The empty board shows Start a workflow…, which opens a new session with the workflow at its first stage, and the help link.
- `make check-app` sees the empty board before the workflow step slices anything.

## Notes
- Reading an untrusted project's files is not allowed (SEC-006); a stat of the folder is.
- PRD decision 10.
