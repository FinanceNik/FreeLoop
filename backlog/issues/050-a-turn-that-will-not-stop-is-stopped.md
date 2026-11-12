---
title: A turn that will not stop is stopped
status: done
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

## Done
Reproduced: the reference agent (`HANG` prompt) ignoring a cancel kept running after the turn was recorded as cancelled. After the wait (`cancelWait`, 10 s) the agent process is now killed, a notice says it was stopped, and the prompt returns `ErrAgentExited`, so the engine drops the runtime and the next message starts the agent again and loads the session (or sends the recap). A late permission request was already refused through the turn context; the test keeps that.
