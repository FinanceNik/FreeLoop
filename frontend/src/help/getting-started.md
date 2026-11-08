# Getting started

This page takes you from a fresh install to your first session, and explains what it means to trust a project.

## What Freeloop is

Freeloop runs AI coding agents on your Mac: its own built-in agent on the models you choose, and external agents such as Claude Code, Codex CLI, Gemini CLI, OpenCode and goose. Every file change and command of the built-in agent goes through your permission policy, so you decide what runs. An external agent runs its own tools, and only what it asks about reaches your policy; **Agents and profiles** explains the difference.

## The first-run guide

The first time you open the app, **Welcome to Freeloop** walks you through four steps.

1. **Choose how agents run.** Either pick an agent you are already signed in to, with **Use** and its name, or open **Use an API key or a local model** and add a provider. Next stays disabled until you have done one of them.
2. **Import from Claude Code (optional).** If you use Claude Code, you can bring its MCP servers, permission rules and skills along. The import only reads its files.
3. **Choose a project.** Pick the folder of a repository with **Choose…**. The **Trust this project** box is off by default.
4. **Start a session.** The app opens the new-session form with your folder filled in.

**Skip the guide**, on the first step and the project step, ends it. To see it again, open the command palette with ⌘K and choose **Setup guide**.

## Try the demo

You can see Freeloop work before you set anything up. On the first screen choose **Try it with a demo**. Later, press ⌘K and choose **Try the demo**.

The demo needs no key and no network. It creates a small scratch project, a git folder with a README, a `main.go` and a `.env` file, and starts a session on a scripted agent. In one turn the agent tries things your policy allows, things it asks you about, and things it refuses. You answer the approval requests as you would in a real session. The session is titled **Demo: your policy at work**.

A banner on the session reminds you that nothing outside the scratch project is touched. When you are done, choose **Remove demo** in the banner and confirm. This deletes the demo session, its scratch folder and its project entry, and nothing else.

## Add a provider

A provider is a model service the built-in agent talks to. Open **Settings → Providers** and choose **Add provider**.

1. Pick the **Kind**: Anthropic, OpenAI, Google Gemini, OpenRouter, or a server on this Mac such as Ollama or LM Studio.
2. Check the **Name** and **Base URL**.
3. Paste the **API key**. Local servers do not need one. Keys go to the macOS Keychain, not to a file.
4. Optionally set a **Default model**, then choose **Save**.
5. Saving tests the connection and lists the models. **Test connection** checks again later.

You do not need a provider for an agent you are signed in to.

## Start your first session

Choose **New session** in the sidebar, or press ⌘K and choose **New session**.

1. Pick the **Project**, or choose a folder with **Choose…**.
2. Leave **Profile** on its default for now. For the built-in agent, pick a **Provider** and a **Model**.
3. Leave **Permission mode** on its default. Unless your profile or environment says otherwise, that is Ask, so the agent asks before it changes files or runs commands.
4. Choose **Start session**, then describe a task in the message box.

## Trust a project

Freeloop treats a project folder as untrusted until you say otherwise. A repository can hold files that steer an agent, so reading them is your decision.

Until you trust a project, sessions in it use your global settings only. Freeloop does not read its `AGENTS.md` or `CLAUDE.md`, its `.freeloop/` settings, run its hooks, or run `git status` in it.

When you choose a folder in the new-session form, a box asks **Do you trust this folder?** and lists any hook commands that trusting would allow to run. Choose **Trust this project**, or start without it. In a running session, an untrusted project shows a banner with **Trust project**. Untrusted projects have a small mark in the sidebar. To take trust back, choose **Stop trusting this project…** in a session's **More actions** menu (**⋯** in the header) or in the command palette, and confirm. Trusting it again approves its hooks as they are then.

Some features need trust: working in its own worktree, Bypass mode, starting from a GitHub issue and comparing profiles.

Trust does not turn off approvals. The agent still asks before it acts, deny rules still win, and a project's `.freeloop/` files can tighten permissions but never loosen them. Trust does let the project's hooks run, and its worktree scripts in worktree sessions, which is why the box lists them. Trust only folders whose contents you know.

## Where to go next

- **Sessions and panes** covers the message box, approvals, rewinding and running several sessions at once.
- **Agents and profiles** explains the built-in agent, external agents and profiles.
- **Permissions and the policy** explains how approvals and rules decide.
- **Keyboard and the command palette** lists the shortcuts.
