---
title: A session that is not on screen holds no transcript
status: done
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

## Done
`frontend/src/heldTranscripts.ts` holds the rules and App.tsx applies them. A session is held when a pane or the screen shows it (a standalone window holds only its own); `ingestMany` goes through `ingestHeld`, which stores no event and asks for no catch-up for a session that is neither held nor still on its way out, and drops a transcript whose gap it cannot close instead of fetching. A session off screen keeps up with its events for `LET_GO_MS` (30 s) so going back and forth costs no fetch, then `letGo` drops it; opening it again builds a fresh transcript from `emptyTranscript` through `catchUp`. App.tsx: one effect on the set of held sessions (`holdTranscripts`) starts the fetch for each held session without a transcript and the let-go timer, replacing the fetches in `openSession`, `showWork` and the workspace restore; `catchUp` returns for a session not held. Activities, rows, unread marks and the attention count never read the transcript and are unchanged. App check: "a session left in every pane gives up its events, and opening it shows them again" (the let-go time is set shorter through `window.__freeloopCount.letGoMs`, and the window reports how many sessions hold events). Memory: `make memcheck` did not finish on this machine, so the number asked for is not measured. Its `prompt` call to the app blocks on the first demo turn (once on a Keychain prompt, `OSStatus -128`, then also with `FREELOOP_DEV_SECRETS=memory`), before and not because of this change. What is measured is what the window holds with ten demo sessions that said something and none open: before, ten whole transcripts; after, none (the app check reads this as `held`, at most one with one session on screen).
