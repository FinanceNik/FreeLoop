---
title: A run's costs in its pull request and on the Usage page
status: open
type: afk
blockedBy: [022-cost-per-issue-and-run]
prd: model-sets
---

## What
A pull request Freeloop drafts from a run's branch gets the run's breakdown as one line, which the user can delete before creating it. The Usage page gains a "By workflow run" grouping, and its CSV export gains `run`, `issue` and `commit` columns, empty for usage outside runs.

## Acceptance
- The drafted pull request text for a run's branch holds the cost line; for a branch of no run it does not; engine test.
- The usage export carries the run, issue and commit of rows that belong to a run; engine test.
- The Usage page's "By workflow run" grouping lists each run with its total; unit test of the grouping.
- `make check-app`: the Usage page offers "By workflow run" and shows the demo run.

## Notes
- Requirement WFL-015; PRD decision 10.
