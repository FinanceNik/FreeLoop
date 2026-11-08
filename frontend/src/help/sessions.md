# Sessions and panes

This page covers working in a session, keeping many sessions in order, and the views that show them together.

## Start and write

Choose **New session** in the sidebar or the command palette. The sidebar lists your sessions with a status badge on each: **Running**, **Needs you**, **Waiting**, **Queued**, **Idle** or **Error**.

In the message box, Enter sends and Shift+Enter adds a line.

- Type `@` to attach a file from the project. Pick one with the arrow keys and Enter or Tab.
- Type `/` to see commands: the mode commands `/ask`, `/accept-edits`, `/plan`, `/auto` and `/bypass`, plus whatever the agent and your MCP servers offer. `/auto` and `/bypass` appear only when the session can use them.
- Use **Attach files**, paste or drop files to add them. Text goes into the message and pictures go to the model.

While a turn runs, the send button becomes **Queue**. A message sent then waits for the turn to end, and the box shows how many are queued. **Stop** ends the current turn.

Beside the box are the session's controls: model and effort where the agent offers them, the token optimizer, and the permissions menu (Ask, Accept edits, Plan, Auto, Bypass and **Rules…**).

## Approvals

When the agent wants to do something your policy does not decide, a card appears in the conversation with the command or paths. You can add a note for the agent, then choose **Allow once**, **Allow always** or **Deny**. **Allow always** saves a rule, and a menu next to it sets whether it applies to this session or everywhere. Keys answer too; see the Keyboard page.

## Rewind, fork and compact

Point at one of your messages to see its actions: copy, **Rewind to here** and **Fork from here**.

- **Rewind** restores the files to how they were before that message, and the conversation too, and puts the message back in the box to edit. If the conversation was compacted or rewound since, only the files go back. It asks first; choose **Rewind** or **Keep**.
- **Fork** starts a new session with the conversation up to that message.
- **Compact context** summarises the conversation so each message sends less. Its button appears in the session header when the context is at least 80% full and the agent offers `/compact`.

## Worktrees

In the new-session form, **Work in its own git worktree and branch** gives the session a separate checkout, so sessions in one project do not edit the same files. It needs a trusted project. To bring the work back, commit it, then open the Diff tab and choose **Merge…**, then **Merge into the main checkout**. Only committed work is merged.

## Rename, tag, archive, delete

The **More actions** menu in the session header has **Rename**, **Add a tag**, **Archive** (or **Unarchive**) and **Delete…**. Right-click a row in the sidebar for the same, except tags. Delete removes the transcript from disk and cannot be undone. Archived sessions stay hidden until you tick **Archived** in the filters.

Tags group sessions. Type several separated by commas, and remove one with its ×.

The filter button next to **Find a session** opens **Group by**, **Status**, **Tag** and **Archived**. When a filter is on, **Save filter…** keeps it under a name.

## Panes side by side

The buttons at the foot of the sidebar show one to four sessions at once. Drag a session from the sidebar into a pane, hold ⌘ while you click it, or choose **Open beside what is open** from its menu. One pane is active, and the panel and the palette act on it. Narrow panes turn into tabs.

## Board, Queue and Needs attention

The **Board** shows one card per session in three columns: **Needs you**, **Running** and **Idle**.

The **Queue** holds tasks that start as sessions when there is room. You set how many run **At once**, can **Pause** it, and add a task with **Add to queue**. A queued session asks for every tool call as usual.

**Needs attention** (⌘0) lists every session waiting for you across projects. You can answer approvals there or open the session. J and K move between requests.

## The right panel

The **Panel** button in the session header shows or hides the panel.

| Tab | What it shows |
| --- | --- |
| **Approvals** | Requests waiting for you, the session's rules, and what you decided |
| **Diff** | Changes since the session started; stage, commit, comment on lines and merge |
| **Files** | The project's files, with the ones the agent changed marked |
| **Terminal** | A shell in the session's environment |
| **Preview** | Your running app, by port, with **Go**, **Reload** and **Open in browser** |
| **Info** | Status, model, mode, tokens, cost and what your policy held back |

A **Plan** tab also appears when the agent proposes a plan.

The Terminal runs as you, with no approvals, and the agent does not see what you type there.
