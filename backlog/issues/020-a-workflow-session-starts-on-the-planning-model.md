---
title: A session started with a workflow begins on its Planning model
status: open
type: afk
blockedBy: [019-planning-stages-switch-to-the-sets-model]
prd: model-sets
---

## What
A new session for which the user chose the workflow explicitly (Start a workflow, the workflow picker, the new-session form) starts on the set's Planning model and effort, when its profile can have it; the new-session form shows that model with "from model set <name>". A session that only takes the default workflow starts on its profile's model and switches at its first stage. A profile that cannot have the model starts on its own, and the first stage's rule applies.

## Acceptance
- `CreateSession` with the workflow chosen starts on the Planning model; with only the default workflow it starts on the profile's; engine test.
- A profile on an external agent that does not offer the model starts on its own model; engine test.
- The new-session form shows the model it will start on and where it comes from.
- `make check-app`: Start a workflow opens a session already on the Planning model.

## Notes
- Requirement WFL-014; PRD decision 8.
