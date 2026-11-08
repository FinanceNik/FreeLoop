---
name: slice-into-issues
description: Split a PRD into vertical-slice issues in the project's backlog, each cutting through every layer, with what blocks it and whether an agent may do it unattended. Use after writing a PRD.
license: Apache-2.0
metadata:
  source: Freeloop's built-in workflow "Plan, slice, run AFK"
---

# Slice a PRD into issues

## Vertical slices

Each issue is a thin slice through **every layer** the feature touches (a tracer bullet): data, logic, interface and tests together, small enough for one session, and working end to end when done. Never slice by layer ("the database part", "the UI part").

The first slice is the thinnest path that works end to end. Later slices widen it.

## Files

One file per issue in `<backlog>/issues/`, named `<number>-<short-name>.md` with a three-digit number in the order they should be done: `001-store-a-draft.md`.

```markdown
---
title: Store a draft
status: open
type: afk
blockedBy: []
prd: <the PRD's file name without .md>
---

## What
The behaviour this slice adds, as the user sees it.

## Acceptance
- A checkable statement per line; each becomes a test.

## Notes
Anything the person or agent doing it needs to know.
```

- `type: afk` when an agent can do it alone from the issue, the PRD and the code: the behaviour is clear and tests can prove it. `type: human` when it needs a person: a design choice, a credential, a look at the running app, or something risky.
- `blockedBy` lists the file names, without `.md`, of issues that must be done first. Leave it empty when nothing blocks it. No cycles.
- `status` is `open` until the work is done, then `done`.

## Before you finish

List the issues in order with what blocks each, and ask the user to check them. Then commit the backlog folder, so a run can start from it.
