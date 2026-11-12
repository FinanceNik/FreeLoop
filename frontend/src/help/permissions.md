# Permissions

How Freeloop decides what an agent may do, and how you change it.

## How a request is checked

Every file read, edit, command and tool call goes to one permission check, for the built-in agent and external agents alike. It looks at protected paths, deny rules, the mode and allow rules, in that order, and puts to you whatever none of them settles.

Nothing is approved because time ran out. Each decision is recorded in the transcript, with who made it.

> An external agent such as Claude Code runs some tools inside its own process. What it does without asking never reaches Freeloop.

## Pick a mode

The mode menu beside the message box shows the current mode.

| Mode | What it does |
| --- | --- |
| **Ask** | Reads inside the project go ahead. Edits and commands ask, unless a rule decides. |
| **Accept edits** | Edits inside the project go ahead. Commands still ask, and so do edits to files other programs act on later, such as `AGENTS.md` or `.github`. |
| **Plan** | Only reads. The agent proposes a plan for you to approve; everything else is denied until then. |
| **Auto** | The agent approves for itself. Only some agents offer it, and Freeloop sees only what they still ask about. |
| **Bypass** | Calls run without asking. Protected paths, deny rules and hooks still hold. |

> New sessions start in Ask, Accept edits or Plan. You choose Auto or Bypass inside a session, and only in a trusted project. Bypass first needs **Allow bypass mode to be chosen for a session** in Settings → Permissions. A command that leaves a sandbox asks every time, even in bypass.

## Rules

A rule names a tool and, optionally, a command or path, and allows or denies it. Global rules come from Settings → Policy or **Always: everywhere** and live in `rules.yaml`. **Always: this session** saves a rule that ends with the session. Environments, profiles and trusted projects can carry rules too, but a project's rules can only deny.

- A deny rule wins at every scope and in every mode, bypass included.
- An allow rule never lifts plan mode or protected paths.
- A deny rule on a path that names `write_file` or `edit_file` holds for both, and for an external agent's edits, which are judged as `edit_file`. An allow rule covers only the tool it names.
- A shell rule ending in `*` matches a prefix, and only approves a single plain command, never a chain, pipe or redirection.
- A shell deny rule matches the words the shell will run, so `git "push"`, `git p\ush`, `noglob git push`, `eval "git push"` and `sh -c 'git push'` are all `git push`, and flag letters are read as a set: a rule on `rm -rf *` also stops `rm -fr`, `rm -r -f`, `rm -rfv` and `rm --recursive --force`.
- A command that builds its program or its subcommand when it runs, such as `$a push` or `git "$sub"`, cannot be compared with a deny rule. While a shell deny rule could apply to it, no allow rule approves it (it asks) and bypass mode refuses it.

On an approval card, **Allow always** saves a rule for exactly that command or path. When one rule can't cover the call, such as a call on several files, it allows the call once and saves nothing. The other answers are **Allow once** and **Deny**. The shortcuts are ⌘↩, ⌘⇧↩ and ⌘⇧⌫.

> A deny rule can't stop what an agent does by itself in its own auto mode, since that never reaches Freeloop.

## Protected paths and secret files

Some places are never read or written, in any mode, whatever a rule says. They include `~/.ssh`, `~/.aws`, your {{keychain files|credential files|keyring files}}, browser data, other tools' sign-in files and Freeloop's data folder. Settings → Policy lists them under **Protected paths**.

In a project:

- Agents never write into `.git` or `.freeloop`, or outside the project.
- Reading outside the project asks.
- Reading a file whose name says it holds secrets, such as `.env`, also asks, unless an allow rule covers it or the session is in bypass. A search over a folder leaves such files out and says which, unless you name them in the path or the include, which asks.

> These checks cover the file tools. An approved shell command runs as you, unless the environment is sandboxed.

## Hooks

A hook is a shell command that runs at a point in a session. In Settings → Permissions, under **Hooks**, choose **Add a hook**, pick **Session start**, **Before a tool call**, **Before asking you** or **After a tool call**, and press **Save**. The default timeout is 30 seconds.

The hook gets the event as JSON on standard input, and its exit code decides:

- 0 goes on.
- 2 blocks, with standard error as the reason. A blocked call is denied and a blocked session start stops the session. After a tool call runs once the call is done, so 2 there only shows its objection.
- Anything else, a timeout or a hook that can't start is a failure. If a guard hook (Before a tool call or Before asking you) fails, the call asks you, even where the mode or an allow rule would have allowed it. Other hooks report the failure and the session goes on.

> A hook can block a call but never approve one. A project's hooks, in `.freeloop/hooks.yaml`, run only once you trust the project, and only as the set you saw then or approved since.

## Check a decision and approve waiting items

In Settings → Policy, **Would this be allowed?** judges a command, edit, read or fetch without running it. Pick where, a mode and an action, enter the command, path or URL, and press **Check** to see the verdict and why.

A hook, allow rule, environment, setting, token optimizer choice or MCP server address that arrived outside the app, such as from a restored backup or a hand edit, does nothing until you approve it under **Waiting for your approval** in Settings → Permissions. A trusted project's hooks wait again when they change. Approve only what you recognize. Deny rules need no approval and apply at once.
