---
title: Model sets: plan on a big model, hand the issues to a small one, and see what each cost
status: open
---

## Problem

Someone running workflows wants to plan (grill, PRD, slice) on a strong model such as Opus or Fable and hand the many small AFK issues to a cheap, fast one such as Qwen 3.8 Flash. Today they can't set that up once and reuse it.

- **Few models to pick from.** The AFK dialog's model picker lists every model of the issue profile's own provider, but only the default model of each other provider. Reaching Qwen on another provider means making a profile just for it.
- **No effort.** Effort can't be chosen for a run.
- **No fixed planning model.** Nothing pins the planning stages to a model. They run on whatever the session happens to be on.
- **No cost answer.** No view says what a workflow run, an issue or its commit cost, so they can't tell whether the split pays off.

## Solution

- **Model sets.** A model set is a named, reusable choice of model and effort for the three parts of a workflow: **Planning stages**, **Issues** and **Reviews**. Each row may be left "not fixed". Sets are made in Settings → Workflows → Model sets.
- **A workflow picks one set, or none.** With a set:
  - The planning stages switch the session to the Planning model before they run.
  - A session started explicitly with the workflow begins on that model.
  - The AFK dialog starts from the set's Issues and Reviews rows.
- **A full model list in the AFK dialog.** Its pickers list every model of every provider the user can call without being asked, grouped by provider, with an Effort picker beside each.
- **Cost per run and issue.** The Board shows what each issue (and so each commit) cost and what the run cost, split into Planning, Issues and Reviews by model. The Usage page can group by workflow run.

## User stories

- As a workflow user, I want to plan on Opus and run the issues on Qwen without making a profile per model, so that the cheap work costs little.
- As a workflow user, I want to define that split once and reuse it across workflows, so that I don't choose models before every run.
- As a workflow user, I want to change the issues' model for one run without changing my set, so that I can try a model out.
- As a workflow user, I want to choose effort with the model, so that a small model runs at low effort and planning at high.
- As a workflow user, I want to see what each issue, commit and run cost, by model, so that I can tell whether the split pays off.
- As a user whose provider lost its key, I want to be told and asked, never silently moved to another model, so that I never pay for a model I didn't choose.

## Decisions

1. **This round's scope.** This round builds three things: the full model list, model sets, and cost per workflow run. A model per issue comes later as the set's Issues row ("all sub-tasks on model X at effort Y"), so it slots in without a new concept.
2. **The model list.** The AFK dialog's pickers list every model of every provider callable without being asked (it has its key, or needs none, as a local one does).
   - **Order and source.** Models are grouped by provider, the profile's own provider first. Each provider's list comes from its own listing, cached for a few minutes.
   - **Other model…** names a model a provider doesn't list.
   - **Another provider.** Choosing one moves the run's sessions to it; the rest of the profile (instructions, mode, skills, MCP servers) stays.
   - **Built-in agent only.** An external agent can only switch among the models it reports, so for its profile the picker keeps what it last reported, with a line saying so. To send work to another provider, the user picks a built-in profile for the issues.
3. **Effort.** An Effort picker beside each model offers "the profile's" (the default) and then the levels the chosen model supports. It is disabled, with a line saying so, for a model without effort, and nothing is stored then.
   - **Stored on the run** as `effort` and `reviewEffort`, and switched through the session's effort option before its first message, as the model is (HRN-001).
   - **External agents.** Their effort choices are whatever effort option they report.
4. **Model sets, not per-workflow rows.** A set has a name and three rows (Planning stages, Issues, Reviews), each a model and effort or "not fixed".
   - **One per workflow.** A workflow picks one set or none, so there is one place to look, not a set and a workflow's own rows competing.
   - **Editing.** Settings → Workflows → Model sets has New, Edit, Duplicate and Delete. Deleting a set in use asks first, then sets those workflows back to none.
   - **The name.** It is "model set": a profile already means more than a model.
5. **Stored in `config.yaml`, not in the workflow file.** `modelSets:` holds the sets, keyed by an ID made from the name, and `workflowModelSets:` maps a workflow ID to a set ID. Three reasons:
   - A model is named by the user's own provider ID, which means nothing on another machine.
   - The built-in workflow can't be edited.
   - Editing a file workflow ends its approval, and choosing a model shouldn't.
6. **Planning stages switch the session.** A planning stage (grill, PRD, slice) in a session whose workflow has a set with a Planning row switches the session to that model and effort before sending its prompt. It goes through the session's own model option, as the composer does, and records a notice naming the set. Already on it, nothing switches.
   - **It stays.** The session keeps that model afterwards, since the next stage usually follows.
   - **When it can't be had,** the stage doesn't start and says why, offering **Run on \<current model\>** and **Open model sets**. The user is there to decide, so nothing falls back silently.
7. **The AFK dialog starts from the set.** The set's Issues and Reviews rows fill the dialog's pickers, labelled "from model set \<name\>".
   - **Changing them is for this run only.** The label then reads "changed for this run", and **Reset** restores the set's value.
   - **"Not fixed"** leaves the picker on the profile's model.
   - **Unavailable models.** A model the chosen profile can't have is marked "not available with this profile", and Start stays disabled until it is changed.
   - **The run records what it started with.** Editing the set later doesn't change a run under way. The Board's run line shows "Issues on \<model\> · \<effort\>".
8. **New sessions start on the Planning model only when the workflow was chosen explicitly** (Start a workflow, the workflow picker, the new-session form), and the form shows the model it will use. A session that merely picks up the default workflow starts on its profile's model and switches at its first stage. A default shouldn't silently put every chat on the big model.
9. **A model that's gone.** Only a missing provider or a missing key makes a set's model unavailable. A model a provider no longer lists still counts, since listings are incomplete; that's what "Other model…" is for.
   - **The set keeps the value.** Settings marks the row, so restoring the key fixes it.
   - **Where it is used,** decisions 6 and 7 apply.
   - **A run under way** that loses its model fails that issue with the reason, as WFL-010 does, and never moves to a costlier model.
   - **The set editor** uses the same grouped picker and effort rule. A set with an unavailable model may be saved, and is shown marked.
10. **Cost per workflow run.** Costs come from the usage already recorded per session and model (PRV-008), priced with the current table.
    - **Per issue, which is per commit.** The issue's panel shows the issue session plus its review, split by model, with the commit.
    - **Per run, which is per branch and PR.** The run line shows Planning, Issues and Reviews with their models and a total. Planning is what the session the run was started from cost before the run began.
    - **The PR.** A pull request Freeloop drafts from the run's branch gets that breakdown as one line, which the user can delete before creating it.
    - **The Usage page** gains a "By workflow run" grouping, and its CSV export gains `run`, `issue` and `commit` columns, empty for usage outside runs.
    - **Unpriced usage.** Unpriced models and agents that report no tokens show as "+ unpriced", never as $0.

## How it is tested

- **Engine, Go, on two scripted providers, "big" and "small"**, so crossing providers is real:
  - Sets in `config.yaml` are validated. An empty name, a duplicate ID or an unknown effort is refused. A set naming a missing provider loads and reports it unavailable.
  - The model list for a built-in profile holds the listed models of every callable provider, not only their defaults, and leaves out a provider without its key.
  - A planning stage in a session whose workflow has a set switches the session to the Planning model and effort and records the notice. A second stage on the same model switches nothing. With the provider's key removed, the stage is refused with an error naming the model.
  - A session created with the workflow chosen starts on the Planning model; one that only takes the default workflow doesn't.
  - An AFK run started with a set's Issues and Reviews models runs both issues and the review on "small" at the set's effort. It is refused when the issue profile can't have the model.
  - With scripted usage, a run's cost breakdown splits Planning, Issues and Reviews by model, gives each issue its cost and commit, and marks unpriced usage. The usage export carries the run, issue and commit columns.
- **Frontend, `node --test`.** The pure helper behind the pickers is tested for:
  - grouping by provider, the profile's provider first
  - the "from model set", "changed for this run" and unavailable labels
  - effort choices following the model's capabilities
  - the cost line's text, including "+ unpriced"
- **`make check-app`** runs with a second demo provider, "Demo small", in development mode. It:
  - makes a model set in Settings → Workflows and gives it to the built-in workflow
  - starts a workflow session and finds it on the Planning model
  - opens the AFK dialog and finds the Issues model filled, with "from model set"
  - starts the run and finds "Issues on Demo small" and the run's cost line on the Board.

## Out of scope

- **A model per issue:** the slicer marking issues small or large, and overriding single issues in the dialog. That comes next, on the set's Issues row.
- **A separate model for each planning stage.** Grill, PRD and slice share one Planning row.
- **Model hints in workflow files,** and sharing sets across machines.
- **Model sets for plain sessions or scheduled tasks.**
- **Cost estimates before a run starts.**
- **External agents reaching another provider's models.**
- **Cost per commit for ordinary sessions,** outside workflows.

## Open questions

None.
