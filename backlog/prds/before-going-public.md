---
title: Before going public, the policy holds and long sessions stay fast
status: open
---

## Problem

A review on 2026-11-11 read the permission path, the Go engine and the window, each on its own, and the project's plan beside them. It found the code careful and the app not ready to be shown:

- **The policy has holes.** A never-readable file is readable through `/System/Volumes/Data/...` (reproduced with `freeloop check`). `grep` shows a `.env` file's contents without the question a read gets. `git "push"` passes a `deny git push *` rule. A paired phone's key is a cookie, which every port of the same host receives. Four smaller ones sit beside these. The policy is what the app says sets it apart, so these come first.
- **A long session costs the whole session, again and again.** Every read of a transcript parses the whole file under the session's lock: about 150 ms and 263 MB on the user's 88 MB session of 77,845 events, from 18 places in the engine that ask small questions, and from the window. The window copies and folds the whole transcript for each event, redraws the whole app for each token of any session, loads every session that says anything, and never lets one go. The flicker fixed on 2026-11-11 was one sign of it.
- **What runs is never let go, and some spending is not counted.** An agent's process lives until the app quits. A crash between two saves of an AFK run can start the same paid session twice. A stopped call on the built-in loop, a suggested commit message and a skill test spend on the user's key and record nothing.
- **The documents say what is no longer so.** VISION.md says a macOS app with no server component; ARCHITECTURE.md says a virtualized list and 30 ms coalescing. Windows has never been run on Windows. No CI job has run since 2026-11-06.

## Solution

Six stages, in this order. A stage's issues are in `backlog/issues`, 033 to 056.

1. **The policy holds (033 to 039).** Each hole gets a test that shows it, then the fix, then its line in SECURITY.md.
2. **A long session to measure with (040).** A made-up transcript the size of the real one, a Go benchmark and a fold timing, so every later issue has a number to meet and the next slowdown is seen.
3. **The engine reads what it is asked for (041, 042).** The store finds a seq without parsing what is before it and reads outside the append lock; the engine's small questions stop reading whole transcripts.
4. **The window does work in proportion to what changed (043 to 048).** Events arrive in batches, one per frame; the fold goes on from where it was; a session not on screen holds no transcript; a long session opens at its end if the numbers still ask for it; a pane that fails does not take the window with it.
5. **What runs is let go, and what is spent is counted (049 to 053).**
6. **The documents say what is, and the repository goes public (054 to 056).**

Stages 1, 3 and 4 touch different packages (`internal/permission` and `internal/environment`; `internal/store` and `internal/engine`; `frontend/src`), so three sessions can work them side by side. Within a stage the issues go in order.

## Decisions

1. **Nothing new until the repository is public.** The freeze of 2026-11-07 did not hold: about 31,000 lines came after it. Until issue 056 is done, only these issues and fixes for what they find. The phone's open issues (026 to 032) and sharing usage across devices wait.
2. **A hole is closed from a failing test.** The test goes in first and fails for the reason the review gave; a hole that cannot be shown is recorded as not reproduced, not fixed blind.
3. **The numbers to meet**, on the fixture of issue 040 (80,000 events), on the user's Mac:
   - a read of the last 100 events: under 5 ms, with memory that does not grow with the file;
   - none of the engine's periodic work (attention, activity, live usage, the queue) reads a whole transcript;
   - a batch of new events folds in under 2 ms, and a streamed token redraws only the item it belongs to;
   - a session that is not open holds no events in the window;
   - opening the long session shows its end in under a second.
   
   **Measured before any fix** (issue 040, Apple M4 Pro, 80,000 events; `go test ./internal/store/ -run xxx -bench Long`, `node --test --test-name-pattern=timing frontend/src/transcript.test.ts`, `make check-app`; the fixture is `internal/store/longfixture_test.go` and `frontend/src/longfixture.ts`). The text shape is 22 MB, the tool shape 105 MB (the user's longest: 92 MB):

   | | text | tools |
   |---|---|---|
   | store read from seq 0 | 70 to 85 ms, 85 MB allocated | 150 to 190 ms, 327 MB |
   | store read of the last 100 events | 55 to 80 ms, 45 MB | 150 to 195 ms, 287 MB |
   | store, last error | 60 to 70 ms, 85 MB | 140 to 180 ms, 327 MB |
   | window folds all events at once | 9 ms | 37 ms |
   | `apply` of one event, at the end | 204 us | 483 us |
   | view rebuilt for each new event, at the end | 5 ms | 35 ms |
   | lived-in app check, click to the end drawn | not run | 6.6 s (197 turns) |

   The last 100 events cost as much as the whole file: the first number to meet (under 5 ms) is missed by a factor of 10 to 40. The timings vary by about 30% between runs.

4. **Opening at the end (046) is built only if the last number is missed** once 041 to 045 are in. It is the largest change to the window in this plan, and the cheaper ones may be enough.
5. **No large rework before going public.** `Engine` (567 methods), `App.tsx` and `SessionView.tsx` (48 and 46 pieces of state) stay as they are, since splitting them changes nothing a user sees and risks a lot. The same for the four usage folds, the built-in harness's 23 branches in the engine and the app check's fixed waits. They are listed under "Out of scope" so they are not forgotten.
6. **Checks run here.** CI cannot run until the repository is public, so each issue ends with `make lint` and `make test`, and `make check-app` for a change to the window.

## How it is tested

- **Go, at the broker and the environment's guard:** each policy hole as a red-team case (the alias, grep on `.env`, the quoted `push`, `noglob`, an approval for another command, unreadable rules), in ask, accept-edits and bypass mode.
- **Go, `internal/remote` on a real port:** a phone's key without its Tailscale login, and from another origin, is refused.
- **Go benchmarks in `internal/store`** on the 80,000-event fixture, with the numbers above as limits a test checks.
- **Frontend, `node --test`:** the fold gives the same view built at once and built in steps, for every fixture the fold's tests have; the fold's time per batch; what a closed session holds.
- **`make check-app`, lived-in:** opens the long session and times its first drawing; a pane made to fail leaves the other pane and the approvals working.
- **People:** Windows and Linux run once (055); the first CI run after going public (056).

## Out of scope

- Splitting `Engine`, `App.tsx` and `SessionView.tsx`; one usage fold in place of four; the harness branches in the engine and the window; the RPC socket's 17 methods beside the Wails service's 234.
- The app check's 149 fixed waits and its English selectors, and running it on Windows and Linux.
- Unit tests for components.
- Docker, bubblewrap and Windows sandboxes, the pull side of the SSH sync, redaction in logs and reports, and MCP sign-ins: the review did not read them. A second security reading after going public.
- Signing (OQ-009) and five testers (B-G5 in ROADMAP.md): after the repository is public.

## Open questions

None. Closed on 2026-11-11, the user taking each recommendation:

- OQ-053: an idle agent is stopped after 30 minutes, the time a setting with "never" as a choice (issue 049).
- OQ-054: the documents lead with the user's policy and bill over every agent they run, the built-in harness being the one that needs no vendor (issue 054).
- OQ-055: ESLint with the two hooks rules, three pinned development packages (issue 048).
