---
title: Events reach the window in batches, one per frame
status: done
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

## Done
- Batching is in the window (`frontend/src/eventBatch.ts`), not in Go, so the Wails event keeps its name and payload for every subscriber; the Go side still emits one event per engine event. That is why the "Go test over the batcher" became a `node --test` over `createBatcher` with a fake clock: a permission request, a decision, a turn's end and a status change flush the batch at once (a status change too, since a row's "Needs you" line reads the status and the app check polls it as the approval appears).
- Render counting in the app check uses a counter on `window.__freeloopCount`, set only by the check.
