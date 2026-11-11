---
title: The engine's small questions stop reading whole transcripts
status: open
type: afk
blockedBy: [041-the-store-reads-from-where-it-is-asked]
prd: before-going-public
---

## What
Eighteen places call `store.Events(ctx, sid, 0)`, most to answer something small: the last error (`attention.go:89`), what is queued (`queue.go:193`), how a turn ended (`afk.go:765`), a run's cost (`runcost.go:110`), activity (`activity.go:256`), and others in `options.go`, `stuck.go`, `afkmodels.go`, `worktreescripts.go`, `pullrequest.go`, `compare.go`, `subagents.go`, `runtime.go` and `store/work.go:184`. Each is given the read it needs: the tail, a range from a seq it already knows, or a value the index already holds. Export, fork and handoff need the whole transcript and keep it.

## Acceptance
- A test wraps the store and fails when attention, activity, live usage, the queue or the AFK tick reads from seq 0 for a session whose index is current.
- Each changed caller returns what it did before; its existing tests pass unchanged.
- `recentusage.go`'s 60-second poll reads from its last seq at the cost of that tail, on the benchmark.
- The list of callers left reading everything is in DEVELOPING.md with the reason for each.

## Notes
- Where a caller wants "the last event of type X", prefer a seq kept in the session's row or the activity index over scanning backwards.
