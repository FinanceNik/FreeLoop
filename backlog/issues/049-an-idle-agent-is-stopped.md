---
title: An idle agent is stopped, and what it held is let go
status: open
type: afk
blockedBy: []
prd: before-going-public
---

## What
A session's runtime stops only on archive, delete, the agent's own exit, a preset change and a subagent's end (`sessions.go:62-99`). Every session that was prompted keeps its agent's process or container until the app quits, and an AFK run adds one per issue and one per review. `make memcheck` runs no external agent, so nothing sees it. A runtime with no turn, no waiting approval and nothing queued is stopped after 30 minutes and starts again on the next message, as it does after a restart of the app; the time is a setting, with "never" as a choice (OQ-053, closed 2026-11-11).


## Acceptance
- Settings has the time, 30 minutes unless changed, and "never"; a file can set it too (SET-001).
- A session idle past the time has no agent process and no container; a message to it starts one and the turn runs with the session's history; Go test with the reference agent.
- A session with a turn running, an approval waiting, a queued message or an open terminal is never stopped; Go tests for each.
- An AFK run's finished issue and review sessions are stopped when their step ends.
- What a stopped session held goes with it: its open transcript file (`store.go:427`) and its `stuck` and `contextSeen` state.
- `make memcheck` gains a run with ten sessions on the reference agent and reports the processes alive before and after the idle time.

## Notes
- `stopIfIdle` already checks and removes under the runtime's lock; this adds the clock.
