# Token optimizers

Shorten shell output before the model sees it, without giving up the safeguards.

## What it does

A shell command's output goes back to the model and costs tokens. A token optimizer, such as RTK, is a small program that shortens it first.

Shorter output can leave out something the agent needs, such as a commit message or the end of a large diff, so optimizers are off unless you choose one. They work with the built-in agent and Claude Code, but not other external agents, SSH environments, or Claude Code in a Docker environment.

## Turn one on

Open **Settings → Token optimizers**. You can choose in four places:

- **For new sessions** sets the default. **Off** is the starting choice.
- A profile has a **Token optimizer** field of its own.
- The new-session form has a list whose first entry follows the profile or the default.
- An open session has a list in its header, or **Token optimizer: ...** in the command palette.

An optimizer that isn't installed shows **(not installed)**.

## Install RTK

If RTK is on your PATH, Freeloop finds it. Otherwise **Install RTK** shows the version, license and source (RTK's GitHub releases) before you click.

- The download is pinned to one version and checked against a fixed checksum.
- It starts only when you click, and **Local only** in **Settings → Network** refuses it.
- The copy stays in Freeloop's own folder. **Remove Freeloop's copy** deletes it.

For a program you install yourself, the page shows an install command with a **Copy** button. **Check it** runs the program once and says whether it works.

## Add your own

Put a file in the `optimizers` folder of the data folder. Freeloop lists it as **(not approved)** and shows what it would run for every command. Click **Approve** only if you trust it.

The same goes for a choice that comes from a file. If a config or profile file names an optimizer, new sessions start with it off until you approve it in **Settings → Permissions**. A project's profile can't turn one on.

## What stays safe

- Your approval comes first. The permission broker judges the command as the model wrote it, and only then is the optimizer asked for a shorter form.
- Only plain commands are rewritten. A pipe, redirection or substitution runs as written.
- If the optimizer returns anything but the same command in its own form, or is missing, slow or fails, the original runs.
- Listing runs nothing. Freeloop runs the program only after you choose or check it, and refuses one found inside the project folder.

> Choosing an optimizer still means trusting that program. It runs with the session's rights.

> If RTK has its own hook in Claude Code's settings file, that hook rewrites Claude Code's commands whatever you choose here, and it can approve commands without asking you. Freeloop doesn't change that file; a session shows a warning when it finds such a hook.
