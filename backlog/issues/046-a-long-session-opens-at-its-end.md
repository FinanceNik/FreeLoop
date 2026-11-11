---
title: A long session opens at its end
status: open
type: afk
blockedBy: [041-the-store-reads-from-where-it-is-asked, 044-the-fold-goes-on-from-where-it-was, 045-a-session-not-on-screen-holds-no-transcript]
prd: before-going-public
---

## What
Only if, with 041 to 045 done, opening the 80,000-event fixture still takes more than a second to show its end, or the 88 MB session still crosses the bridge whole. Then: the window asks for the last few thousand events and for earlier pages as the reader scrolls up, and what the view needs from the whole session (token totals, the mode, git state, the files touched, rewinds) comes from the engine in one small answer, not from folding everything.

## Acceptance
- First, the measurement: the numbers after 045 in this file. If the limit is met, the issue is closed as not needed, with the numbers.
- Opening the long fixture shows its end in under a second, and no call across the bridge carries more than the page size; app check and Go test.
- Scrolling up loads earlier events without the place jumping; app check step, with the checks the flicker fix added still passing.
- The header's totals, the right panel's files and a rewind's greyed range match the fully loaded view; `node --test` comparing both on the long fixture.
- Search and "jump to seq" open the page that holds the seq.
- The phone opens at the end the same way.

## Notes
- The largest change to the window in this plan; see decision 4 in the PRD.
