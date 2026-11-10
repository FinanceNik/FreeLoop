# Workflows

Work in stages: settle the design, write it down, slice it, and let agents do the unattended parts.

## What a workflow is

A workflow gives a session slash commands, one per stage. The built-in one, **Plan, slice, run AFK**, follows Matt Pocock's way of working with agents:

| Command | What happens |
| --- | --- |
| `/grill <brief>` | The agent interviews you about the brief, one question at a time, each with the answer it suggests. In plan mode, so nothing is written. |
| `/prd` | It writes the design you settled as a PRD in `backlog/prds/`. |
| `/slice` | It splits the PRD into issues in `backlog/issues/`: thin slices through every layer, each marked AFK or human, with what blocks it. Then it commits them. |
| `/afk` | Opens the AFK run's dialog. |
| `/review` | A second agent reviews this session's changes, read-only. |
| `/cleanup` | Moves finished issues and PRDs out of the project. |

Every tool call still goes through your policy, in every stage.

## Start one

1. Open **Settings → Workflows** and click **Install skills**. The stages' skills go into Freeloop's own skill store, with their source recorded. Nothing is written to `~/.claude` or the project.
2. In a session, choose the workflow in the list beside the optimizer, or pick **Start a workflow…** in the palette (⌘K) for a new session.
3. Type `/` to see the stages.

**For new sessions** on the Workflows page sets a default, and a profile can choose its own. **Off** on either wins.

## The backlog

Issues are Markdown files the agents write and the app reads, in a trusted project only:

```
---
title: Search drafts
status: open        # open or done
type: afk           # afk: an agent may do it alone; human: it needs you
blockedBy: [001-store-a-draft]
---
```

The **Board** shows a row for each trusted project that has a backlog, with a card for each issue in four columns: **Backlog**, **Working**, **In review** and **Done**. A card shows its title, its number, whether an agent (AFK) or a person does it, and in Backlog what it waits on. An issue the app can't read counts as one for a person. The Board only shows where each issue stands, from its file and the run; you don't move cards. Ordinary sessions are not on it: they are in the sidebar.

**Archive finished** removes done issues, and PRDs whose issues are all done, in a commit of their own, so later agents aren't misled by stale docs. Git history keeps them: `git log -- backlog/` lists them and `git show` brings one back.

## Run the AFK issues

**Run AFK issues…** on the Board, `/afk`, or **Run AFK issues: <project>** in the palette opens the run's dialog. It shows the first issue, the profiles for the issues and their reviews, and the commands the run will want: its own git commands and your project's tests and checks.

- **Allow for this run's sessions** lets those commands run without asking, for the run's sessions only. Anything else still asks, and waits in **Needs attention** until you answer.
- The run does one issue at a time, each in a fresh session, in a git worktree and branch of its own. Your checkout stays as it is.
- An issue is done when its file says `status: done`. Then the run commits what's left and a second agent reviews it, before the next issue starts.
- It stops when no AFK issue is ready, at its cap, when a budget is reached, or when a session ends without marking its issue done. **Pause** and **Stop** are on the Board.

> The backlog must be committed first: the run starts from the last commit.

A schedule can start a run overnight: in **Settings → Schedules**, choose **An AFK run of the backlog** under **It starts**.

## QA what it did

Each finished issue waits in **Needs attention** as **Ready for QA**, with the review's answer. Try the change, then click **QA done**. When the run's issues pass, merge its branch from the Diff tab of one of its sessions.

## Add your own

Put a file in the `workflows` folder of the data folder. It runs only after you click **Approve** on the Workflows page, and any change to the file needs approving again. A workflow can never turn asking off.
