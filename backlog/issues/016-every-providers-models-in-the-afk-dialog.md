---
title: The AFK dialog lists every callable provider's models, and a run can take one from another provider
status: done
type: afk
blockedBy: []
prd: model-sets
---

## What
The AFK dialog's Issues and Reviews model pickers list every model of every provider the user can call without being asked (it has its key, or needs none), grouped by provider, the issue profile's own provider first, with "Other model…" for a model a provider does not list. Choosing a model on another provider runs that run's issue (or review) sessions on that provider; the rest of the profile stays. For a profile on an external agent the picker keeps what the agent last reported, as now.

## Acceptance
- `ProfileModels` for a built-in profile holds the listed models of every callable provider, not only their default models, and leaves out a provider without its key; engine test on two scripted providers, "big" and "small".
- An AFK run whose issue model is on "small" runs its issue and its review on "small"; engine test.
- Each provider's listing is cached for a few minutes, so opening the dialog twice asks a provider once; engine test.
- "Other model…" lets the user type a model name, which the run uses.
- In development mode a second demo provider, "Demo small", exists beside the demo provider.
- `make check-app`: the dialog's Issues picker shows a "Demo small" group, and a run started on it shows its issue session on Demo small.

## Notes
- The model option today adds only the default model of providers other than the session's (`builtinOptions`); `listedModels` already caches a provider's listing.
- External agents: unchanged (PRD decision 2).
- Requirement WFL-011; PRD decision 2.

## Done
- `modelOptions(ctx, ses, every)` (the built-in agent's model option) asks, with every set, each callable provider's listing side by side, cached as before; `ProfileModels` uses it and says `Custom`. The session's own provider comes first, in the composer's list too.
- A model typed by hand is taken when the choices come from that provider (`ProfileModels.offers`); WFL-010's test now names an unoffered provider instead of an unlisted model.
- "Callable unasked" also counts the scripted provider, which needs no key; "without its key" means none configured. A key configured but missing from the keychain is decision 9's, for 018.
- In development mode `demo-small` ("Demo small", model `small`) stands beside the demo. `frontend/src/modelChoice.ts` groups the choices and names a typed model; the dialog has "Other model on <provider>…" per group.
