---
title: The store reads a transcript from where it is asked
status: open
type: afk
blockedBy: [040-a-long-session-to-measure-with]
prd: before-going-public
---

## What
`readEvents` parses every line of the file whatever `afterSeq` is, and `Store.Events` holds the session's append lock for all of it, so a long read stalls the session's writes. The open transcript keeps a sparse table of seq to byte offset (one entry every few hundred events, built when it opens and kept on append), a read seeks to the nearest entry, and it reads without the append lock up to the size the file had when it began, which is safe for an append-only file. Two more reads are added: the last n events, and a range with a limit.

## Acceptance
- On the 80,000-event fixture a read of the last 100 events takes under 5 ms and its memory does not grow with the file; a test checks both as limits.
- An append during a long read is not held up; Go test with the race detector.
- `Events`, the tail read and the limited read return the same events as today's full parse, on every transcript the store's tests have, damaged and repaired ones included.
- A transcript repaired or truncated on opening rebuilds its table; `recover_test.go` and `repair_test.go` pass unchanged.
- `Engine.Events` takes a limit and never returns more than it; the Wails service and the phone server pass one.

## Notes
- `internal/store/transcript.go:211`, `internal/store/store.go:378`, `internal/engine/engine.go:1058`.
- Open transcripts are also never closed (`store.go:427`); issue 049 lets them go.
