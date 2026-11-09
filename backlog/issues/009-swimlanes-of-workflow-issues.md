---
title: The Board shows workflow issues in swimlanes, and the Queue gets its own icon
status: open
type: afk
blockedBy: []
prd: workflow-board
---

## What
The Board stops showing sessions. It shows one row per trusted project with a backlog, each across four columns: Backlog, Working, In review and Done, with one compact card per issue. The row header holds the project's counts, its run controls (Run AFK issues…, Pause and Stop, the run's progress line) and Archive finished. Since the Board no longer links to the Queue, the Queue gets an icon in the sidebar's bottom row, beside the Board, with a count of waiting tasks.

## Acceptance
- A pure function places a project's issues in columns from the backlog's items and the run's steps: ready, blocked, human and unreadable issues in Backlog; a working step in Working; a reviewing or QA step in In review; done issues in Done. Unit-tested with `node --test`.
- No ordinary session appears on the Board, and the Needs you, Running and Idle columns are gone.
- A card shows its title in at most two lines (the full title on hover) and one meta line: the short ID, AFK or Human, what it waits on in Backlog, how long it has worked in Working. The file path is in the hover text, not on the card.
- Columns keep a minimum width; a narrow window scrolls the board sideways. `make check-app` finds no card whose content is wider than the card.
- The Queue icon opens the Queue page and shows the number of waiting tasks; the palette still opens it.
- `make check-app`: the workflow step finds both issues in the workflow project's row, the first in Backlog as ready, the second as waiting on the first, and after the run both in Done; the queue step opens the Queue from the icon; the step "a finished session says so on the board" is removed.
- GUI-016 in `docs/REQUIREMENTS.md` is reworded to the workflow board; the help pages that describe the Board say what it shows now.

## Notes
- The engine already returns what is needed: `Backlogs` gives each project's analysed issues and its run (`BacklogInfo.Run`, with steps per issue); `issueStep` in `afk.ts` finds an issue's step.
- A done issue in a run's last step stays in Done. Order of Done is decided in a later issue; any stable order will do here.
- All new text goes through the catalogues (`t()`, en, de and nl; DEVELOPING.md, "Text in the interface"). German and Dutch address no one.
- Read the PRD `backlog/prds/workflow-board.md` first; decisions 1, 2, 3, 7 and 8 are this issue.
