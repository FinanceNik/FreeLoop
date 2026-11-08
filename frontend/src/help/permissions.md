# Permissions

This page explains how Freeloop decides what an agent may do: modes, rules, protected paths, hooks and where to check a decision.

## One check for every agent

When an agent wants to read a file, change one, run a command or call a tool, the request goes to one permission check. It applies to the built-in agent and to external agents alike. Nothing is approved because time ran out. Each decision is recorded in the session's transcript, with who made it.

The check looks at protected paths, deny rules, the mode and allow rules, in that order. Whatever none of them settles is put to you.

One limit: an external agent such as Claude Code runs some tools inside its own process. What it does without asking never reaches Freeloop.

## Modes

You pick a mode in the session's mode menu, beside the message box, which shows the current mode, such as **Ask**.

| Mode | What it does |
|---|---|
| **Ask** | Reads inside the project go ahead. Edits and commands ask, unless a rule decides. |
| **Accept edits** | Edits inside the project go ahead. Commands still ask, and so do edits to files other programs act on later, such as `AGENTS.md` or `.github`. |
| **Plan** | Only reads. The agent proposes a plan for you to approve. Everything else is denied until then. |
| **Auto** | The agent approves for itself. Only some agents offer it, and Freeloop sees only what they still ask about. |
| **Bypass** | Calls run without asking. Protected paths, deny rules and hooks still hold. |

A new session starts in **Ask**, **Accept edits** or **Plan**. It never starts in **Auto** or **Bypass**: you choose those inside the session, and only in a trusted project.

Bypass must be allowed first. Turn on **Allow bypass mode to be chosen for a session** in Settings → Permissions. Until then the option is unavailable. A command that leaves a sandbox asks every time, even in bypass.

## Rules

A rule names a tool and, optionally, a command or a path, and either allows or denies.

- **Global** rules are the ones you add in Settings → Policy or save with **Always: everywhere**. They are kept in `rules.yaml`.
- **Session** rules come from **Always: this session** and end with the session.
- Environments, profiles and trusted projects can carry rules too. A project's rules can only deny.

Three things hold for every rule:

- A deny rule wins over allow rules at every scope and in every mode, bypass included. It cannot stop what an agent does by itself in its own auto mode, since that never reaches Freeloop.
- An allow rule never lifts plan mode or protected paths.
- A shell rule ending in `*` matches a prefix, and only approves a single plain command, never a chain, pipe or redirection.

**Allow always** saves a rule for exactly that command or path, for this session unless you choose **Always: everywhere**. When one rule cannot cover the call, for example a call on several files, it allows the call once and saves nothing. The other answers are **Allow once** and **Deny**. The shortcuts are ⌘↩, ⌘⇧↩ and ⌘⇧⌫. An allow rule that comes from a file, such as one you add by hand in `rules.yaml`, an environment or a profile, does nothing until you approve it.

## Protected paths and secret files

Some places are never read or written, in any mode, whatever a rule says. They include `~/.ssh`, `~/.aws`, your keychain files, browser data, other tools' sign-in files and Freeloop's data folder. Settings → Policy lists them under **Protected paths**.

In a project, agents never write into `.git` or `.freeloop`, or outside the project. Reading outside the project asks. Reading a file whose name says it holds secrets, such as `.env`, also asks, unless an allow rule covers it or the session is in bypass.

These checks cover the file tools. An approved shell command runs as you, unless the environment is sandboxed.

## Hooks

A hook is a shell command that runs at a point in a session. Add them in Settings → Permissions under **Hooks**, with **Add a hook**, then press **Save**. You choose when: **Session start**, **Before a tool call**, **Before asking you** or **After a tool call**. The default timeout is 30 seconds.

The hook gets the event as JSON on standard input, and its exit code decides:

- Exit 0 goes on.
- Exit 2 blocks, with standard error as the reason. A blocked call is denied, and a blocked session start stops the session. **After a tool call** runs once the call is done, so exit 2 there only shows its objection.
- Any other exit, a timeout or a hook that cannot start is a failure. If a guard hook (**Before a tool call** or **Before asking you**) fails, the call asks you, even where the mode or an allow rule would have allowed it. Other hooks report the failure and the session goes on.

A hook can block a call but never approve one. A project's hooks, in `.freeloop/hooks.yaml`, run only once you trust the project, and only as the set you saw then or approved since.

## Check a decision

In Settings → Policy, **Would this be allowed?** judges a command, edit, read or fetch without running it. Pick where, a mode and an action, type the command, path or URL, and press **Check**. It says whether the call would be allowed, denied or asked, and why.

## Approvals waiting

A hook, allow rule, environment, setting, token optimizer choice or MCP server address that arrived outside the app, such as from a restored backup or a hand edit, does nothing until you approve it. So do a trusted project's hooks once they change. Settings → Permissions lists these under **Waiting for your approval**. Approve only what you recognize. Deny rules need no approval and apply at once.
