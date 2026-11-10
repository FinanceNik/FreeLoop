---
title: Rows collapse, a filter shows one project, and Done shows the last five
status: done
type: afk
blockedBy: [010-waiting-for-you-on-the-card]
prd: workflow-board
---

## What
Each project's row can be collapsed from its header and stays so; a filter at the top shows all projects or one. A collapsed row opens by itself when one of its issues waits for the user. Done shows the last five issues of each row, newest first, with "Show all N".

## Acceptance
- Done's order: issues a run did, by when the run finished them, newest first, then the rest by ID; capped at five with "Show all N", which shows them all. Unit-tested.
- Collapsing a row hides its columns and keeps its header with the counts; the choice survives a restart (browser storage, per project).
- The project filter shows one row or all; it lists only projects with a row.
- A collapsed row with a waiting issue is shown open, with a note why.
- `make check-app` collapses a row and finds its cards hidden, and shows the Done cap with "Show all".

## Notes
- PRD decisions 3 and 9.

## Done
- Done's order: a card with a run step, newest finish first (the step's `endedAt`, which the run sets when the review ends), then the rest by id; `capDone` keeps five and says how many it left out; a row has "Show all N" and then "Show fewer". A row's choice to show all is not kept.
- A row collapses under its header with a chevron button (`aria-expanded`); the choice is kept in the browser's storage under `freeloop:board-collapsed`, by project, and unreadable or blocked storage is no choice. With an issue waiting for the user the row is shown open with a note and the button is disabled, and it folds again once nothing waits.
- The filter is a list in the header, shown when there are several rows; it lists the projects with a row, and a project that has no row any more filters nothing. Its choice is not kept.
- The check's setup now makes a project with seven finished issues (the oldest session, so the empty pane's short list of recent sessions still holds the session the panes step looks for), and trusts it from its session.
