# Getting started

Go from a fresh install to your first session.

## Try the demo

Choose **Try it with a demo** on the first screen, or press ⌘K and pick **Try the demo**. It needs no key and no network.

- It creates a scratch project and starts a session on a scripted agent.
- The agent tries things your policy allows, asks about and refuses, and you answer the approvals as in a real session.
- **Remove demo** in the session's banner deletes the demo session, its scratch folder and its project entry, and nothing else.

## Follow the first-run guide

**Welcome to Freeloop** walks you through four steps the first time you open the app.

- Choose how agents run: pick an agent you're signed in to with **Use** and its name, or open **Use an API key or a local model** and add a provider.
- Import from Claude Code (optional): brings its MCP servers, permission rules and skills.
- Choose a project with **Choose…**. The **Trust this project** box is off by default.
- Start a session: the form opens with your folder filled in.

**Skip the guide** ends it. To see it again, press ⌘K and choose **Setup guide**.

## Add a provider

A provider is a model service the built-in agent talks to. You don't need one for an agent you're signed in to.

- Open **Settings → Providers** and choose **Add provider**.
- Pick the Kind, check the Name and Base URL, and paste the API key. Local servers don't need one.
- Optionally set a Default model, then choose **Save**.

Saving tests the connection and lists the models. Keys go to the macOS Keychain, not to a file. See Providers for more.

## Start your first session

Choose **New session** in the sidebar, or press ⌘K and pick it.

- Pick the Project, or choose a folder with Choose….
- Leave Profile on its default. For the built-in agent, pick a Provider and a Model.
- Leave Permission mode on its default. Unless your profile or environment says otherwise, that's Ask, so the agent asks before it changes files or runs commands.
- Choose **Start session**, then describe a task in the message box.

## Trust a project

A project folder is untrusted until you say otherwise, because a repository can hold files that steer an agent. Until then, sessions in it use your global settings only. Freeloop doesn't read its `AGENTS.md` or `CLAUDE.md` or its `.freeloop/` settings, run its hooks, or run `git status` in it.

- In the new-session form, **Do you trust this folder?** lists the hook commands that trusting would allow. Choose **Trust this project**, or start without it.
- In a running session, choose **Trust project** in the banner.
- To take trust back, choose **Stop trusting this project…** in **More actions** (**⋯** in the header) or the command palette.

Working in its own worktree, Bypass mode, starting from a GitHub issue and comparing profiles need trust.

> Trust doesn't turn off approvals: the agent still asks, deny rules still win, and a project's `.freeloop/` files can tighten permissions but never loosen them. Trust does let the project's hooks run, so trust only folders whose contents you know.

## Where to go next

- Sessions and panes: the message box and several sessions at once.
- Agents and profiles: external agents and profiles.
- Permissions and the policy: how approvals and rules decide.
