---
title: The Board as a scrum board of workflow issues
status: open
---

## Problem

Someone running workflows across several projects opens the Board to see where the work stands, and gets a mix instead. The Board puts workflow sessions (AFK runs and their reviews) and ordinary sessions in the same Needs you, Running and Idle columns, beside a Backlog column of issue files. So one issue can show twice: as a backlog card and as a session card. The cards overflow into each other in narrow columns, the layout has no clear structure, and what matters most, an issue whose agent is waiting for an answer, is hard to find. Idle tells them nothing they need.

## Solution

The Board becomes a scrum board of workflow issues only. Ordinary sessions live in the sidebar, and Needs attention stays the place that answers "who has a question right now".

- **Swimlanes:** one row per project, across four columns:
  - **Backlog:** ready, blocked, human, unreadable, and issues a run stopped at.
  - **Working:** an AFK session is on it.
  - **In review:** the agent's review, then the user's QA.
  - **Done.**
- **Row header:** the project's counts and its run controls: **Run AFK issues…**, Pause and Stop, the run's progress line, and **Archive finished**. A row can collapse, and a filter shows one project or all.
- **Cards:** one card per issue, with:
  - a title of at most two lines;
  - one meta line: short ID, AFK or Human, plus what it waits on in Backlog or how long it has worked in Working;
  - marks and actions only when they apply.

  Clicking a card opens a read-only detail panel with the issue's text, its file, its sessions and what it blocks.
- **Waiting for you:** an issue whose session waits for an answer stays in Working with a strong mark. The request is answered on the card (Allow, Deny), or the card opens the session. The board's header counts the waiting issues and scrolls to the first, and a collapsed row opens when one of its issues waits.
- **In review:** shows "Agent reviewing" (muted) or "Ready for your QA" (highlighted, with the start of the review's answer, **QA done**, **Open session** and **Open the review**). QA done moves the card to Done.
- **Stopped issues:** an issue a run stopped at goes back to Backlog with a red "The run stopped here: <reason>" mark and **Open session**, until the next run.
- **Done:** shows the last five per row, newest first, with "Show all N".
- **Columns:** they keep a minimum width; on a narrow window the board scrolls sideways instead of squeezing cards.
- **The Queue:** gets an icon of its own in the sidebar's bottom row, with a count of waiting tasks, since it no longer has a link on the Board.

## User stories

- As someone running AFK runs in several projects, I want each project's issues in one row by stage, so that I see where each project stands at a glance.
- As someone away while a run works, I want an issue whose agent waits for me to stand out and be answerable on its card, so that a run is not stuck longer than it must be.
- As the one who QAs, I want issues ready for my QA marked in In review with the review's answer, so that I know what to try next.
- As someone with ordinary sessions too, I want them off the Board, so that the Board only shows planned work.
- As someone using a small window, I want cards that never overflow, so that the Board stays readable.

## Decisions

1. **Only workflow issues, one card per issue.** An ordinary session has no backlog or review, so scrum columns would be artificial for it, and two kinds of card made the clutter.
2. **Sessions stay in the sidebar; the Board loses its session columns.** A Sessions view would duplicate the sidebar and Needs attention. GUI-016 is reworded rather than replaced.
3. **Swimlanes per project.** Each column then means one stage everywhere, and projects do not mix. A single set of columns with project labels gets cluttered with two or three projects.
4. **Waiting is a mark, not a column.** A Needs you column would pull a card out of its stage. Answering on the card saves a trip, and the card still opens the session.
5. **Four columns; In review holds both the agent's review and the user's QA.** It shows them as two states. A QA column would make five.
6. **A stopped issue goes back to Backlog.** Its file still says open and the next run retries it; leaving it in Working would claim work that is not happening. The red mark says why it stopped until the next run starts.
7. **Compact cards with a read-only detail panel.** This fixes the overflow: the file path, relations and run line no longer sit on the card. Minimum column widths with sideways scrolling keep cards from squeezing.
8. **The Queue moves to a sidebar icon with a count.** It holds ordinary tasks too, so it does not belong on a workflow board.
9. **Done is capped at five per row.** Done grows until archived. Issues a run did are ordered by when it finished them, then by ID; an issue file carries no date of its own.
10. **Rows:**
    - Trusted projects with a backlog get a row.
    - An untrusted project with a backlog folder gets a collapsed row with **Trust project**. Seeing that the folder exists reads nothing in it.
    - Projects without a backlog get no row.
    - With no backlog anywhere, the board explains itself, with **Start a workflow…** and a link to the Workflows help.
11. **No dragging.** The board indicates; a card's column follows the issue file and the run, so the board can never disagree with them.
12. **The actions kept on the board:**
    - answering a waiting request;
    - **QA done**;
    - the row's run controls and **Archive finished**;
    - **Trust project**.

    Each answers something the board shows, and none moves a card by hand. There is no Mark done and no Run this issue.

## How it is tested

- **Unit tests (`node --test`)** for one pure function that places a project's issues in columns from the backlog's items and the run's steps. They check:
  - each column and state: ready, blocked, human, unreadable, working, waiting for the user, agent reviewing, ready for QA, done, and stopped by a run;
  - the Done cap and its order;
  - which projects get a row: trusted with a backlog, untrusted with a backlog folder, none.
- **`make check-app`:** the workflow step is reworked to follow two issues through the board:
  - both start in Backlog, the second blocked by the first;
  - a run started without `git commit *` allowed has its session ask, and the request is answered on the card;
  - the issues move through Working and In review to Done;
  - QA done on a card moves it;
  - the detail panel opens with the issue's text.

  The same pass checks that no ordinary session appears on the board, that the Queue icon opens the Queue, and that no card's content is wider than the card.
- **Removed:** the old check that a finished session shows on the Board; the sidebar's unread dot covers it.

## Out of scope

- Dragging cards, and any action that moves a card by hand (Mark done, Run this issue).
- A Sessions view on the Board.
- Filters beyond choosing a project.
- Parallel runs (WFL-007); a row with several Working cards already fits the layout.
- Changes to Needs attention, which keeps listing waiting requests and issues ready for QA as it does now.

## Open questions

None.
