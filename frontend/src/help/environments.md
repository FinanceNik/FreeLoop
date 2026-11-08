# Environments

This page explains where a session's commands run, what each kind of environment holds back, and how variables and secrets reach commands.

An environment is what a session's commands run with: variables, secrets, the tools the agent may use and the mode new sessions start in. You manage them in Settings → Environments, with **Add environment**, **Edit** and **Delete** (**Reset** for a built-in one). Every install has two built in: **Local** and **Local, sandboxed**.

## Local

Commands run on this Mac, in the project folder, with your full access. Nothing confines them. The permission check still asks before a command runs, but an approved command can read and change anything you can. Local is not a sandbox.

## SSH keys

By default, the commands, agents and terminals of a local environment can use your SSH agent, so `git push` and `git pull` over SSH work. That also means an approved command can use your SSH keys for anything else, such as logging in to a server. To prevent it, open the environment on the Environments page and clear **Commands and agents can use my SSH agent**. Git over SSH then needs another way in, such as HTTPS.

## Local, sandboxed

Commands run under the macOS sandbox. Because they can only write in the project and read little outside it, the built-in agent's commands run without asking, and deny rules still win. Leaving the sandbox asks every time. If the sandbox cannot be applied, the command does not run.

Inside the sandbox, commands:

- write only in the project, its caches and temporary files
- cannot read your home folder, except common toolchains, and cannot read shell startup files or git's own config
- cannot write `.git/config`, git hooks, nested `.git` or `.freeloop` folders, or other files that tools act on later
- reach only this Mac's own network by default

Set **Network** to **This Mac only**, **This Mac and the hosts listed here** or **None**. With a host list, a tool must use the proxy the sandbox sets up. You can also add more folders it can read or write.

What the sandbox does not cover: an external agent's own tools, MCP servers, and what you type in a session's terminal. It also has limits inside: reads outside your home folder are open, `/tmp` is shared, and **This Mac only** still reaches anything else listening on this Mac. A command can still write a file you later run yourself, such as a `Makefile`.

## Docker

Each session gets its own container, with the project folder shared at the same path. Commands and external agents run inside it, so they see the project and the image, not the rest of your files. You give an image, or use the project's `.devcontainer/devcontainer.json`, which needs the project to be trusted. A **Setup command** runs once after the container is created, and a **Health check** runs before every session start. It needs Docker Desktop, or Colima and the docker CLI.

A container is not a vault. It has network access, runs as the image's user (often root), and can change anything in the shared project folder. The exceptions are mounted read-only: the project's `.freeloop` folder, the devcontainer files, and the repository's `.git/config` and git hooks. The rest of `.git` stays writable, so treat such a checkout with care in your own terminal.

## SSH

Commands run on another machine, in a working copy kept in sync with the project on your Mac. Enter a **Host**, either a name from `~/.ssh/config` or `user@host`. Run `ssh` to it once in a terminal and check its fingerprint, because Freeloop does not accept a host key by itself. It signs in with your keys and ssh agent, and never reads them.

The other machine is not trusted:

- Secrets, files a deny rule covers and what the project ignores are not copied there.
- Only regular files come back. Settings your Mac or an agent would act on, such as `.git`, `.claude` and `AGENTS.md`, do not.
- Other changes do come back, so read the diff before you run anything.
- Your code, and this environment's variables and secrets, go to that machine.

Use a machine you can afford to lose. So far an SSH environment works with the built-in agent only.

## Variables and secrets

**Variables** are one `KEY=value` per line, added to every command's environment.

**Secrets** are values you add with **Add secret**. They are stored in the macOS Keychain, handed to commands as variables when a command starts, and removed from transcripts. Removal covers values Freeloop knows. The text a model already received is not changed, so a secret a command prints can reach the model.

An environment edited by hand in its YAML file waits for your approval in Settings → Permissions before a session can use it.

## Choose an environment

- For one session, use the **Environment** menu in the new-session form. For an environment on this Mac it notes whether commands ask or run in a sandbox.
- As a default, the form starts on the project's default environment, or on the one chosen in Settings → Sessions under **Environment** for projects without their own.

An environment can also set the mode new sessions start in, under **New sessions start in**. A profile that sets its own mode wins over it.

If you want the strongest hold on a repository you do not trust, use Docker. For a quick, quieter session on your own code, use **Local, sandboxed**.
