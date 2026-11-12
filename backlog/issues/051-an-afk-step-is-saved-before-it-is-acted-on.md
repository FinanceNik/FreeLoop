---
title: An AFK run saves a step before it acts on it
status: done
type: afk
blockedBy: []
prd: before-going-public
---

## What
`nextIssue` queues the work (`afk.go:931`) and then records the step, ignoring the save's error; `finishIssue` starts a review (`afk.go:746`) and then records it the same way. A failed save or a crash in between makes the next tick start the same issue or review again: a second paid session, unattended (WFL-005). The step is recorded as "starting" first, the save must succeed, and a tick that finds "starting" looks for the session before it makes one.

## Acceptance
- A save that fails stops the run with the reason; nothing is queued; Go test with a store that fails on demand, written first.
- A crash between the record and the start (the run reloaded from `afkruns.json` mid-step) continues with the session that exists or starts exactly one; Go test.
- No `changeRun` or `setStep` error is dropped anywhere in `afk.go`.
- A run's cost for an issue counts one session per step in these tests.

## Notes
- `internal/engine/afk.go:746`, `:931`.

## Done
- `AFKStep.Starting` (`internal/config/afkruns.go`): `nextIssue` saves the step as starting before it queues the task, `finishIssue` saves the step as reviewing and starting before it makes the review; a save that fails stops the run with the reason and makes nothing. A tick that finds `Starting` takes the run's task (`recoverStarting`) or the issue session's review (`startReview`) if one exists, else makes exactly one; `StopAFK` waits for the pass in progress and removes a task whose record was lost.
- Every `changeRun` and `setStep` error in `afk.go` is handled: the two saves above stop the run, the others are logged and the next tick looks again (`afkWarn`, `trySetStep`, `endRunNow`).
- `Engine.afkSave` is the test's way to fail a save. `internal/engine/afk_starting_test.go`: both stops on a failed save, and a run reloaded before and after the task, and before and after the review, was made, each ending with one session and one review per step.
