---
title: An AFK run saves a step before it acts on it
status: open
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
