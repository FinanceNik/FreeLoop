---
title: In review shows the agent's review and the user's QA; a stopped issue goes back to Backlog
status: done
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

## Done
- `placeIssues` sets `review` ("reviewing" or "qa") on cards in In review and `stopped` (the step's note, else the run's reason, else "") on a Backlog card whose latest step failed; a newer run has no such step, so the mark goes without anything clearing it. The card shows the review's answer (from the attention list's QA item, plain text cut after four lines) and QA done, Open its session and Open the review, reusing Needs attention's words.
- QA done on the card calls the same `QADone` as the attention view, and the card moves to Done on the next refresh; the check confirms Needs attention loses the item.
- The check stops the run while its first issue waits for the paused queue, sees the red mark, starts a new run, and sees the mark gone. "Agent reviewing" is covered by the unit test only: the demo's review is too quick for a check to catch it.
