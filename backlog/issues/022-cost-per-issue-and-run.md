---
title: What each issue and each run cost, on the Board
status: open
type: afk
blockedBy: [021-the-afk-dialog-starts-from-the-set]
prd: model-sets
---

## What
From the usage recorded per session and model, the engine gives a run's cost: Planning (what the session the run was started from cost before the run began), Issues and Reviews, each by model, with a total; and each issue's cost (its session plus its review) with its commit. The issue panel shows the issue's cost by model and its commit; the Board's run line shows the split. Unpriced models and agents that report no tokens show as "+ unpriced", never as $0.

## Acceptance
- With scripted usage, the run's breakdown splits Planning, Issues and Reviews by model and sums them; each issue has its cost and commit; engine test.
- Usage of the planning session after the run began is not counted as Planning; engine test.
- An unpriced model makes the breakdown say so instead of counting it as nothing; engine test and unit test of the line's text.
- `make check-app`: a finished demo run shows a cost line on the Board and a cost in an issue's panel.

## Notes
- Usage is stored per day, session and model (PRV-008); price with the current table, as the Usage page does.
- Requirement WFL-015; PRD decision 10.
