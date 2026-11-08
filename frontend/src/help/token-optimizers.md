# Token optimizers

This page explains what a token optimizer does, how to turn one on, and which safeguards stay in place when you do.

## What it does

When an agent runs a shell command, the command's output goes back to the model and costs tokens. A token optimizer is a small program, such as RTK, that shortens that output before the model sees it.

Shorter output can also leave out something the agent needs, such as a commit message or the end of a large diff. For that reason optimizers are off unless you choose one.

An optimizer works with the built-in agent and Claude Code. Other external agents run their commands themselves, so Freeloop has no way to rewrite them. It also does not work in an SSH environment, or for Claude Code in a Docker environment.

## Turn one on

Open **Settings → Token optimizers**. You can choose an optimizer in four places:

* **For new sessions** on that page sets the default. **Off** is the starting choice.
* A profile has a **Token optimizer** field of its own.
* The new-session form has a **Token optimizer** list. Its first entry follows the profile or the default, and **Off** turns it off for that session.
* An open session has an optimizer list in its header, with **Optimizer off** at the top. You can switch at any time. The command palette offers the same choice as **Token optimizer: ...**.

An optimizer that is not installed shows **(not installed)** and cannot be picked in the new-session form.

## Install RTK

If RTK is on your PATH, Freeloop finds it. Otherwise the page offers **Install RTK**. It shows what it would download before you click: the version, the license and that it comes from RTK's GitHub releases.

* The download is pinned to one version and checked against a fixed checksum.
* It starts only when you click. Freeloop never downloads it on its own.
* The copy stays in Freeloop's own folder. Only sessions that choose RTK use it. **Remove Freeloop's copy** deletes it.
* With **Local only** on in **Settings → Network**, the download is refused.

For a program you install yourself, the page shows an install command with a **Copy** button. If you pick an optimizer that is missing from inside a session, a bar offers **Install RTK and turn it on**, or **Type it in the terminal** where an install command exists.

**Check it** runs the program once and says whether it works.

## Add your own

Put a file in the `optimizers` folder of the data folder. Freeloop lists it on the page marked **(not approved)**. It shows what it would run, for every command of a session that uses it. Click **Approve** only if you trust it.

The same rule covers a choice that comes from a file. If a config file or a profile file names an optimizer, new sessions start with it off until you approve that choice in **Settings → Permissions**. Choosing the default yourself on this page counts as approving it. A project's profile cannot turn one on.

## What stays safe

* **Your approval comes first.** The permission broker judges the command exactly as the model wrote it. Only after that is the optimizer asked for a shorter form.
* **Only plain commands are rewritten.** A command with a pipe, a redirection, a substitution or similar syntax runs as written, because shortened output would change what the next step does.
* **A rewrite must match.** If the optimizer returns anything other than the same command in its own form, Freeloop runs the original.
* **Failures run the original.** If the program is missing, slow or fails, the command runs as written.
* **Listing runs nothing.** Another program can share an optimizer's name, so Freeloop runs the program only after you choose or check it. A program found inside the project folder is refused.

Choosing an optimizer still means trusting that program. It runs with the session's rights.

If RTK has its own hook in Claude Code's settings file, that hook rewrites Claude Code's commands whatever you choose here, and it can approve commands without asking you. Freeloop does not change that file; a session shows a warning when it finds such a hook.
