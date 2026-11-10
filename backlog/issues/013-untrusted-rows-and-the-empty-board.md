---
title: Untrusted projects with a backlog get a row to trust them; an empty board explains itself
status: done
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

## Done
- `Engine.Backlogs` lists an untrusted project whose backlog folder exists as a row with `Trusted: false` and no issues or PRDs; `TestBacklogNeedsTrust` checks it, that a project without the folder has no row trusted or not, and that a trusted project's row says it is trusted. The existence check is `environment.IsDir`, which stats the folder and reads nothing in it.
- Trust project on the row asks for the same confirmation as the session view when the project's settings would run hooks (`ProjectSettings`, `trustLines`), then trusts it and reloads the rows.
- The fresh app check now always has an untrusted row (its second project), so the empty Board is checked in the lived-in pass, where no project has a backlog; the fresh pass's Dutch Board check reads the untrusted row's words instead.
