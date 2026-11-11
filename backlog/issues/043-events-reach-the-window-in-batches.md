---
title: Events reach the window in batches, one per frame
status: open
type: afk
blockedBy: [040-a-long-session-to-measure-with]
prd: before-going-public
---

## What
`cmd/freeloop/main.go:256` emits one Wails event per engine event, and `App.tsx:580-607` answers each with three state updates, so every token of every session redraws the whole app. Events are gathered and handed over once per frame (about 16 ms; ARCHITECTURE.md says 30 ms coalescing, which was never built), and the window applies a batch in one update per piece of state. A permission request, a decision and a turn's end are not held back.

## Acceptance
- A burst of 1,000 chunk events causes at most a handful of renders of `App`, counted in a `node --test` over the batching function and in the app check with a render counter.
- A permission request reaches the window within one frame of being emitted; Go test over the batcher.
- Order within a session is kept, and a gap still triggers the catch-up; the existing transcript tests pass.
- The `setSessions` updater has no side effects: refreshing sessions and projects for an unknown session happens outside it.
- The phone's stream handler applies events in batches too.
- ARCHITECTURE.md says what is built.

## Notes
- `frontend/src/App.tsx:439-454` (`ingest`, `catchUp`) and `:580-607`; `frontend/src/phone/PhoneApp.tsx:224-232`.
- Batch in the window if doing it in Go would need the event name or payload to change for other subscribers.
