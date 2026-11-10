---
title: An Effort picker beside each model in the AFK dialog
status: done
type: afk
blockedBy: [016-every-providers-models-in-the-afk-dialog]
prd: model-sets
---

## What
Beside the Issues and Reviews model pickers, an Effort picker offers "the profile's" (the default) and then the levels the chosen model supports. For a model without effort it is disabled with a line saying so. The run stores `effort` and `reviewEffort`, and each issue and review session switches to it through its effort option before its first message, as it does for the model. For an external agent the choices are its reported effort option; with none, the picker is not shown.

## Acceptance
- A run started with effort low runs its issue sessions at low and its review at the review effort; engine test.
- A model without effort stores no effort, whatever was chosen before; engine test.
- The picker helper offers effort only for a model that supports it; unit test.
- `make check-app`: the dialog shows Effort for the demo model and a run started at low shows low on its issue session.

## Notes
- The built-in effort option exists already (`optionEffortID`, levels low to max, shown when the model's capabilities say Effort).
- Requirement WFL-012; PRD decision 3.

## Done
- `Engine.ModelEfforts(root, profile, model)` says which efforts a profile's sessions take on a model (the built-in agent's when the model's capabilities say Effort, an external agent's reported effort option); `runEffort` stores none for a model without effort and refuses a level not offered. The run, its queue tasks and its reviews carry the effort option and level, switched after the model.
- The scripted provider can say its models take an effort (`WithEffort`), which "Demo small" does, so the check runs a run at low.
