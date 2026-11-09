---
title: Rows collapse, a filter shows one project, and Done shows the last five
status: open
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
