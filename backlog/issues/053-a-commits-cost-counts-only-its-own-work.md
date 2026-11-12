---
title: A commit's cost counts only its own work
status: done
type: afk
blockedBy: []
prd: before-going-public
---

## What
The cost per commit (GUI-051) filters commits by `unit_ts >= since` (`internal/store/work.go:419`) but keeps all of the root's usage, so the first commit inside the window takes on the usage of every commit before it. It shows only once history is longer than the 26-week window, and then as one very expensive commit. `WorkSince` also scans all of `work_usage` (`:439`).

## Acceptance
- With commits and usage on both sides of `since`, the first commit in the window costs what was spent since the commit before it; Go test, written first.
- `WorkSince` reads only the rows of the window; a benchmark or a query plan in the test shows the index is used.
- The model a commit's usage is priced by is the one in force when it was used, not the session's current one; Go test with a model change between two commits.

## Notes
- The four usage folds (`search.go`, `recentusage.go`, `store/work.go`, `runcost.go`) differ slightly; making them one is out of scope here and named in the PRD.

## Done
The window now also reads the root's last commit before it (`WorkCommit.Before`, not reported) and the usage since, so the first commit in it costs only its own work; `WorkSince` reads only the window and those roots by index (new `work_commits_root`). A call naming no model is priced by the model in force (`model_changed` events are followed in the fold); before a first change the model is not recorded, so it is the first call's name, else none. `search.go` and `recentusage.go` still start from the session's current model (the folds stay separate, as noted).
