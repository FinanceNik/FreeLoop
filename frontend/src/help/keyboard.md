# Keyboard and the command palette

Find anything with the palette, and see or change the shortcuts.

## Use the command palette

Press ⌘K to open the palette, and press it again or Escape to close it. It reaches nearly everything you can click: pages, sessions, actions and the controls of the open session.

- Move with ↑ and ↓ and press Enter to run the highlighted item.
- Nothing is highlighted until you type or move, so ⌘K then a reflex Enter runs nothing.
- A hint on the right shows the shortcut that does the same thing.

Before you type, it lists four sections:

| Section | Shows |
| --- | --- |
| Waiting for you | Approvals that need an answer |
| Actions | For example New session and Try the demo |
| Recent sessions | Your latest five |
| Go to | Pages such as Board, Queue, Providers, Profiles, Policy and Settings |

Type to search. This session holds what the open session offers, such as Stop the turn, Show approvals and Continue with another profile. Help finds the pages of this help.

Type `/` first to search only the open session's commands: modes, the commands its agent advertises, your profile's slash commands and your MCP servers' prompts.

## Default shortcuts

{{|On Windows and Linux, Ctrl takes the place of the Mac's Command key, and Alt of Option.}}

| Action | Keys |
| --- | --- |
| Open the command palette | ⌘K |
| Open Settings | ⌘, |
| Open Needs attention | ⌘0 |
| Larger text | ⌘= |
| Smaller text | ⌘- |
| Move to the pane on the left, right, above, below | {{⌃⇧← ⌃⇧→ ⌃⇧↑ ⌃⇧↓|Alt+Shift+← Alt+Shift+→ Alt+Shift+↑ Alt+Shift+↓}} |
| Approval: allow once | Y |
| Approval: allow always | A |
| Approval: deny | N |

Some shortcuts are fixed:

- ⌘1 to ⌘9 open the sessions listed first in the sidebar.
- {{⌥⌘1 to ⌥⌘4|Alt+1 to Alt+4}} make a pane active, and {{⌥⌘← and ⌥⌘→|Alt+← and Alt+→}} move between panes.
- In a session's terminal, {{⌘C and ⌘V copy and paste, and ⌃C goes to the shell.|Ctrl+Shift+C and Ctrl+Shift+V copy and paste, and so do Ctrl+V and, with text selected, Ctrl+C. Otherwise Ctrl+C goes to the shell.|Ctrl+Shift+C and Ctrl+Shift+V copy and paste, and Ctrl+C goes to the shell.}}

## Change a shortcut

Open **Settings → Keyboard**, choose the key beside an action and press the new combination. Press Escape to leave it as it was, or choose **Reset** for the default.

Freeloop refuses a key that can't work and says why:

- An app shortcut needs {{⌘ or ⌃|Ctrl}}, or {{⌥|Alt}} with an arrow or another named key, so it works while you type.
- Keys {{macOS|Windows|Linux}} and the app keep, such as ⌘C, ⌘V, {{⌘Q|⌘Y}} and ⌘1 to ⌘9, can't be used.
- A key already used for another action is refused.
- An approval shortcut is one letter with no modifier.

## Answer approvals with keys

The letters Y, A and N work only when no text field has focus. These work from anywhere, the message box included:

| Answer | Keys |
| --- | --- |
| Allow once | ⌘↩ |
| Allow always | ⌘⇧↩ |
| Deny | ⌘⇧⌫ |

In Needs attention, J and K or the arrow keys choose which request the keys answer.

> A new card ignores these keys for a moment after it appears, so a key you pressed for something else can't approve a call you haven't read.
