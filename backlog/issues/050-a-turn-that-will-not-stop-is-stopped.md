---
title: A turn that will not stop is stopped
status: open
type: afk
blockedBy: []
prd: before-going-public
---

## What
Suspected, to be shown first: when an external agent does not answer a cancel within 10 seconds, the turn is recorded as cancelled while the agent goes on working (`internal/harness/acp/acp.go:581`), spending and perhaps asking for permissions under a turn the user believes is over.

## Acceptance
- A test with the reference agent ignoring cancel shows what happens today; if the agent does stop, the issue is closed as not reproduced, with the test kept.
- Otherwise: after the wait, the agent's process group is ended, the transcript says the agent was stopped and not that it stopped, and the next message starts it again with the session's history.
- A permission request arriving after the turn was recorded as ended is refused, not shown.

## Notes
- Process groups are already ended on a session's cancel (`internal/engine/runtime.go`); this is the same for one turn.
