---
title: A long session to measure with
status: open
type: afk
blockedBy: []
prd: before-going-public
---

## What
Nothing measures a long session today: the performance fixture has 5,000 events (NFR-PERF-002) and the user's longest has 77,845 in 88 MB. A generator writes a made-up transcript of 80,000 events shaped like it, in two kinds (mostly streamed text; heavy with tool calls), and three measurements run on it: the store's reads, the window's fold, and opening it in the app check. They record today's numbers; later issues turn them into limits.

## Acceptance
- `go test ./internal/store/ -bench Long` reports time and memory for a read from seq 0, from the last 100 events, and for the last error; the fixture is generated in the test's scratch folder, never checked in.
- A `node --test` timing folds the same shapes at once and one event at a time, and prints the time per event.
- The lived-in app check opens the long session and prints the time to its end being drawn.
- The numbers measured before any fix are written in the PRD under "The numbers to meet", beside the limits.
- The shape comes from counting event types and sizes in a real transcript, read here once; no content of a real session is copied.

## Notes
- `internal/store/transcript.go`, `frontend/src/transcript.test.ts`, `scripts/appcheck/lived-in.sh` and `lived-in.js`.
