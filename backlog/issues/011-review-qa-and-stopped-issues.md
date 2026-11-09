---
title: In review shows the agent's review and the user's QA; a stopped issue goes back to Backlog
status: open
type: afk
blockedBy: [009-swimlanes-of-workflow-issues]
prd: workflow-board
---

## What
In review shows each issue in one of two states: "Agent reviewing", muted, and "Ready for your QA", highlighted, with the start of the review's answer and the buttons QA done, Open session and Open the review. QA done moves the card to Done. An issue a run stopped at goes back to Backlog with a red "The run stopped here: <reason>" mark and Open session, until the next run starts.

## Acceptance
- The placement function tells the two review states apart and puts a failed step's issue in Backlog with the run's reason; unit-tested for both, and for the mark going once a newer run exists.
- QA done on a card moves it to Done and takes it out of Needs attention.
- The review's answer is shown as plain text, cut to a few lines.
- `make check-app`: during the run a card shows "Agent reviewing" or "Ready for your QA"; QA done on the card moves it to Done.

## Notes
- The engine's QA item already carries the review's reply (`QAItem.Reply`); the run's steps carry `reviewId`, `note` and the run's `reason`.
- PRD decisions 5 and 6.
