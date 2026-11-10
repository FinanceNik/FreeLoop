---
title: Model sets made in Settings and given to a workflow
status: open
type: afk
blockedBy: [017-effort-for-a-run]
prd: model-sets
---

## What
Settings → Workflows gets a "Model sets" section: New, Edit, Duplicate and Delete. A set has a name and three rows (Planning stages, Issues, Reviews), each a model and effort from the same grouped picker, or "not fixed". Each workflow picks one set or none. A row whose provider is missing or has no key is marked "not available" and keeps its value. Deleting a set in use asks first and sets those workflows back to none.

## Acceptance
- `config.yaml` holds `modelSets:` (keyed by an ID made from the name) and `workflowModelSets:` (workflow ID to set ID); a set with an empty name, a duplicate ID or an unknown effort is refused; config test.
- A set naming a missing provider loads and is reported unavailable for that row; engine test.
- Choosing a set for a workflow does not change the workflow file or its approval; engine test.
- Deleting a set in use clears it from those workflows; engine test.
- `make check-app`: make a set "Big plans, small works" with Planning on the demo provider and Issues on Demo small, give it to the built-in workflow, and find it chosen there after reopening Settings.

## Notes
- Strings in English, German and Dutch (the catalogues are typed against English).
- Requirement WFL-013; PRD decisions 4, 5, 9.
