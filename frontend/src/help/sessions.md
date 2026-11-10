# Sessions and panes

Work in a session, keep many in order, and see several side by side.

## Start a session

Choose **New session** in the sidebar or press ⌘K and pick it. In the message box:

- Enter sends, Shift+Enter adds a line.
- `@` attaches a file from the project.
- `/` lists commands: the modes and what the agent offers.
- Drop or paste files to attach them.

While a turn runs, a new message waits in a queue until the turn ends. **Stop** ends the turn.

## Answer approvals

When the agent wants to do something your policy doesn't decide, a card appears with the command or files. Choose **Allow once**, **Allow always** or **Deny**, and add a note for the agent if you like.

> **Allow always** saves a rule. The menu beside it decides whether it holds for this session or everywhere.

## Go back or branch off

Point at one of your messages:

- **Rewind to here** puts the files and the conversation back to before that message, and the message back in the box. It asks first.
- **Fork from here** starts a new session with the conversation up to that message.

When the context is 80% full, **Compact context** in the header summarises the conversation so each message sends less.

## Keep sessions apart with worktrees

**Work in its own git worktree and branch**, in the new-session form, gives the session its own checkout, so sessions in one project don't edit the same files. It needs a trusted project.

To bring the work back, commit it, then choose **Merge…** in the Diff tab. Only committed work is merged.

## Organize

- **Rename, tag, archive, delete:** in the session's ⋯ menu, or right-click it in the sidebar.
- **Find:** the filter button beside **Find a session** groups and filters by status, tag and archived. **Save filter…** keeps one.

> Deleting removes the transcript from disk. It can't be undone.

## Several at once

| View | What it's for |
| --- | --- |
| Panes | The buttons at the foot of the sidebar show one to four sessions. Drag a session in, or ⌘-click it. |
| Board | One card per issue of a workflow's backlog, a row for each project: Backlog, Working, In review, Done. |
| Queue | Tasks that start as sessions when there is room. Its icon in the sidebar shows how many are waiting. |
| Needs attention (⌘0) | Everything waiting for you, across projects. J and K move between requests. |

## The right panel

**Panel** in the session header shows it.

| Tab | Shows |
| --- | --- |
| Approvals | What waits for you, the session's rules, what you decided |
| Diff | Changes since the session started; stage, commit, merge |
| Files | The project, with the agent's changes marked |
| Terminal | A shell in the session's environment |
| Preview | Your running app, by port |
| Info | Model, mode, tokens, cost and what the model sees |

> The Terminal runs as you, with no approvals, and the agent doesn't see what you type there.
