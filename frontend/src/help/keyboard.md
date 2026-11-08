# Keyboard and the command palette

This page covers the command palette, the default keyboard shortcuts, how to change them, and the keys that answer approvals.

## The command palette

Press ⌘K to open the palette over the window, and press it again or Escape to close it. It reaches nearly everything you can click: pages, sessions, actions and the controls of the open session.

Before you type, it shows a short list in four sections:

- **Waiting for you**: approvals that need an answer.
- **Actions**: for example **New session** and **Try the demo**.
- **Recent sessions**: your latest five.
- **Go to**: pages such as **Board**, **Queue**, **Providers**, **Profiles**, **Policy**, **Settings** and **Keyboard shortcuts**.

Type to search. Results come in sections, each showing its best few matches. Besides the sections above you may see **This session**, **Sessions**, **New session in**, **Filters**, **Attach to the message** and **Help**, which finds the pages of this help. **This session** holds what the open session offers, such as **Stop the turn**, **Show approvals**, **Show changes**, a permission mode, **Rewind to before the last message…**, **Fork before the last message** and **Continue with** another profile.

Move with ↑ and ↓ and press Enter to run the highlighted item. Nothing is highlighted until you type or move, so ⌘K followed by a reflex Enter runs nothing. A hint on the right shows the shortcut that does the same thing.

### Agent commands

Type `/` first to search only the open session's commands. These are the mode commands, the commands the session's agent advertises, your profile's slash commands and your MCP servers' prompts. The list appears under **Agent commands**. It is empty when no session is open.

## Default shortcuts

| Action | Keys |
| --- | --- |
| Open the command palette | ⌘K |
| Open Settings | ⌘, |
| Open Needs attention | ⌘0 |
| Larger text | ⌘= |
| Smaller text | ⌘- |
| Move to the pane on the left | ⌃⇧← |
| Move to the pane on the right | ⌃⇧→ |
| Move to the pane above | ⌃⇧↑ |
| Move to the pane below | ⌃⇧↓ |
| Approval: allow once | Y |
| Approval: allow always | A |
| Approval: deny | N |

Some shortcuts are fixed:

- ⌘1 to ⌘9 open the sessions listed first in the sidebar.
- ⌥⌘1 to ⌥⌘4 make a pane active, and ⌥⌘← and ⌥⌘→ move between panes.
- ⌘↩, ⌘⇧↩ and ⌘⇧⌫ answer an approval from anywhere.

In the message box, Enter sends, Shift+Enter adds a line, `@` and `/` open pickers, and Escape closes one.

## Change a shortcut

Open **Settings → Keyboard**. Choose the key shown beside an action, press the new combination, and it is saved. Press Escape to leave it as it was. **Reset** puts the default back.

Freeloop refuses a key that cannot work and says why:

- An app shortcut needs ⌘ or ⌃, so it works while you type.
- Keys macOS and the app keep, such as ⌘C, ⌘V, ⌘Q and ⌘1 to ⌘9, cannot be used.
- A key already used for another action is refused.
- An approval shortcut is one letter with no modifier.

## Approval keys

When an approval card is waiting, **Y**, **A** and **N** allow once, allow always and deny. The letters work only when no text field has focus. The ⌘ forms work everywhere, the message box included, so you can answer without leaving it:

| Answer | Keys |
| --- | --- |
| Allow once | ⌘↩ |
| Allow always | ⌘⇧↩ |
| Deny | ⌘⇧⌫ |

A new card ignores these keys for a moment after it appears, so a key you pressed for something else cannot approve a call you have not read. Buttons on the card show their keys. In **Needs attention**, J and K or the arrow keys choose which request the keys answer.
