---
title: The planning stages switch the session to the set's Planning model
status: done
type: afk
blockedBy: [018-model-sets-in-settings]
prd: model-sets
---

## What
Running grill, PRD or slice in a session whose workflow has a set with a Planning row switches the session to that model and effort before the prompt, through its own model option, and records a notice naming the set. The session stays on it. Already on it, nothing switches. When the session cannot have that model (an external agent, a provider without its key), the stage does not start; it says why and offers "Run on <current model>" and "Open model sets".

## Acceptance
- `/grill` switches the session to the Planning model and effort, and the notice comes before the stage's prompt; engine test.
- A second stage on the same model records no switch; engine test.
- With the Planning provider's key removed, `RunStage` refuses with an error naming the model, and running it "on the current model" sends the prompt without switching; engine test.
- `make check-app`: with the set from 018, `/grill` in a workflow session switches it to the Planning model and the transcript shows the notice.

## Notes
- Prompt and afk stages are both stages; only the planning stages (the workflow's prompt-kind stages) use the Planning row.
- Requirement WFL-014; PRD decision 6.

## Done
- `planningModel` in `internal/engine/workflows.go` runs before a prompt stage: the set's Planning model and effort through the session's own options, a notice naming the set, nothing when already there.
- A model the session cannot have is not an error but `StageResult.Blocked`, which names the model and why, so the composer can show it with **Run on <model>** (`RunStageOnCurrent`) and **Open model sets**; nothing is sent. The new-session form's first stage reports it and leaves the brief in the box.
- The app check counted every `.turn-end` as a turn's end, and a model change and a notice render that way too, so it stopped approving too early; a real turn's end is now also `.turn-stop`, which the check counts.
