---
title: Usage counts what was spent
status: done
type: afk
blockedBy: []
prd: before-going-public
---

## What
Three kinds of spending leave no record (PRV-008): a call on the built-in loop that was stopped or failed, since usage is emitted only on `EventDone` (`builtin.go:468-475`), where the external path records it (`acp/calls.go` `endCalls`); a suggested commit message (`changes.go:484`, `Engine.complete`); and a skill's test (`skillauthor.go:137`). And history is priced at today's table: `builtinprices.go` notes a price that doubles on 2026-12-01, which would re-price November.

## Acceptance
- A built-in call stopped mid-stream records the tokens the provider reported up to then, marked as partial, and nothing when it reported none; Go test with the scripted provider, written first.
- A commit message suggestion and a skill test each record a usage event against the session or, with no session, against the day; the Usage page's totals include them; Go tests.
- A price has a date it holds from; a day's usage is priced by the price of that day; Go test with a price that changes between two days.
- The Usage page's note on what is and is not counted is still true, in all three languages.

## Notes
- `internal/engine/builtinprices.go`, `internal/engine/usage.go`.

## Done
- Skill tests with no session are booked under an empty session id and shown as "The app's own calls". The session's live cost in the window (`cost.ts`), a run's cost (`runcost.go`) and what a commit cost (`work.go`) still use today's prices; the Usage page, budgets, the run comparison and recent usage price by day.
- Ollama, the Responses API and the OpenAI-compatible adapter report tokens only at the end of a stream, so a call cut off there records nothing; Anthropic and Gemini send running totals.
- A price added for 2026-12-01 is entered only for gemini-4-argon.
