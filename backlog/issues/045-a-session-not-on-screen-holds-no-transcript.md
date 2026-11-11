---
title: A session that is not on screen holds no transcript
status: open
type: afk
blockedBy: [043-events-reach-the-window-in-batches]
prd: before-going-public
---

## What
An event for a session that was never opened meets an empty transcript, counts as a gap, and `catchUp` fetches the session from seq 0 (`App.tsx:439-454`); nothing is ever removed from `transcriptsRef`. With twenty sessions the window holds twenty whole transcripts. A session's events are fetched when a pane shows it and let go a while after none does; for the others the window keeps only what the sidebar, the Board and the attention view need, which the activity fold and the session's row already give.

## Acceptance
- An event for a session in no pane fetches nothing and stores no event; `node --test` over the ingest logic.
- A session closed in every pane gives up its events after a short while, and opening it again shows it as before; app check step.
- A standalone session window takes in only its own session's events.
- The sidebar's status, unread mark, activity line and the attention count stay right for sessions that are not open; existing app check steps pass.
- Memory of the window with ten demo sessions is measured before and after and written in the issue's Done note.

## Notes
- `frontend/src/App.tsx:432-454` and `:580`; `frontend/src/activity.ts`.
