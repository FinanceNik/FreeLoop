---
title: An issue whose agent waits for an answer stands out and is answered on its card
status: done
type: afk
blockedBy: [009-swimlanes-of-workflow-issues]
prd: workflow-board
---

## What
When an AFK session working on an issue waits for an approval or an answer, its card stays in Working with a strong "Waiting for you" mark. The waiting request is shown on the card with its Allow and Deny buttons, and the card opens the session. The board's header says how many issues wait for the user and scrolls to the first one.

## Acceptance
- The placement function marks an issue as waiting when its working step's session has a pending request; unit-tested.
- A waiting card shows the request and answers it in place, with the same buttons and options as Needs attention.
- "Open session" on the card opens the issue's session.
- The header counts waiting issues ("2 issues waiting for you") and clicking it scrolls to the first; with none, nothing is shown.
- `make check-app`: a run started without `git commit *` allowed makes its session ask; the card in Working shows "Waiting for you"; answering on the card lets the run go on to Done.

## Notes
- The Board already received the attention list and rendered a session's request with `RequestView`; reuse it for the card.
- Needs attention keeps listing the same request; nothing there changes.
- PRD decision 4.

## Done
- `placeIssues` takes the pending requests per session (`waiting`), marks only a card whose step is working, and `waitingIssues` counts them; the card shows the mark, the request (`RequestView`, as Needs attention shows it) and "Open its session". The header's count is a button that scrolls to the first.
- The check needed a request that really asks: a command in the run's sandbox does not, so the demo's first sliced issue says to ask a question first (`askMarker` in the scripted provider), and its AFK session calls `ask_user`, which asks in every mode. The check answers it with Skip on the card.
- An approval waiting on a card is the same request in the same view; only the question was tried in the app.
