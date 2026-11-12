---
title: The fold goes on from where it was
status: done
type: afk
blockedBy: [043-events-reach-the-window-in-batches]
prd: before-going-public
---

## What
`apply` copies the whole events array for each event (`transcript.ts:53`) and `SessionView.tsx:279` folds the whole transcript again with `buildView`: 9 to 13 ms per event at 74,000 events, 97 ms when heavy with tools, where `replaceTool`'s `indexOf` (`transcript.ts:640`) makes it quadratic. The fold keeps its state and applies only the new events; an item that did not change keeps its identity, so only the item a token belongs to is drawn again.

## Acceptance
- For every fixture in `transcript.test.ts`, and for the long fixture cut at random places, the view built in steps equals the view built at once.
- A batch of new events folds in under 2 ms on the 80,000-event fixture, both shapes; a test checks it as a limit.
- Appending events does not copy the ones already held.
- A tool call is found by its ID without a scan.
- A rewind, a history reset and a change of language still give the right view: the fold starts over where it must, and a test covers each.
- The per-render passes over all items (`costs`, `toolRuns`, the palette's list at `SessionView.tsx:1042`, `panelData`) are kept from one render to the next or made to go on the same way.
- `ItemView`'s comparison covers every prop it draws from (`SessionView.tsx:1911`), or takes none it ignores.
- The phone uses the same fold.

## Notes
- `buildView` is the public face and stays; a second function carries the state.

## Done
`newFold` carries the state and `viewOf(transcript)` goes on from it; `buildView` stays and folds at once. A rewind and a history reset are applied to the items already folded (a test covers each, with the language change), so the fold starts over only for a change of language and for a transcript older than the one it last folded. The per-render passes over all items are in the view now: `runs`, `waiting`, `rewindable`, `models`, `panel` (what `panelData` returns), with a session's cost tallied from the calls it counted before (`tallyCalls`) and a turn's cost kept with its item. `optimizerCounts` and `planVersions` in the panel still scan the items once per render of the panel. `ItemView` compares every prop; the three functions it calls are now the same ones from render to render. App.tsx: one line, `transcript={transcripts[s.id] ?? emptyTranscript}` in place of `events={transcripts[s.id]?.events ?? []}`.
