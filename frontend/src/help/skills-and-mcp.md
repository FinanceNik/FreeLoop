# Skills and MCP servers

This page explains how to give your agents extra know-how with Agent Skills and extra tools with MCP servers.

## Skills

A skill is a folder with a `SKILL.md` file that tells an agent how to do one kind of task. It can also hold scripts, reference files and assets. The format is the open Agent Skills format, so skills written for Claude Code load without changes.

The agent sees only each enabled skill's name and description, and loads the full text when a task matches.

### Where skills come from

Open **Settings → Skills** to see every skill Freeloop found and where it came from:

* **Skill store**: the `skills` folder in Freeloop's data folder. Installed and imported skills go here.
* **Project .freeloop/skills**, **Project .agents/skills** and **Project .claude/skills**: folders inside a project. Freeloop reads them only for projects you trust, and never changes them.

Pick a trusted project in the **Project** list to include its skills. A project skill hides a store skill with the same name.

### Install a skill

Skills are copied into the store. The source is not changed. Use one of these:

* **Install from a folder…** and choose a skill folder.
* Drop a skill folder or a `.zip` file on the drop zone.
* Paste a folder path, a `.zip` path or a git URL and click **Preview**. Add `#branch`, `#tag` or `#commit` to pin a git URL.
* **Import from Claude Code** reads `~/.claude/skills`. Nothing is written back to Claude Code.

Every route shows a preview first. Each skill is marked new, up to date, changed or cannot load, and a changed skill shows what differs. Tick the skills you want and click the install button, which names the count. A skill that is up to date cannot be ticked.

Skills installed from git show **Check for updates**, which shows the changes before **Update**. **Roll back** puts back the version the last install replaced. **New skill…** creates a skill from a template, and **Test…** asks a model once whether it would pick the skill for a prompt you write. A test uses tokens.

### Turn skills on and off

Each skill has **On everywhere** and, when a project is chosen, **In this project**. A skill that is off is not listed to the model. A profile can also limit a session to named skills in its **Skills** field.

### Skills and scripts

A skill's scripts never run by themselves. The built-in agent runs them only through the shell tool, so your mode and rules decide each one like any other command. The preview lists a skill's scripts. Claude Code also receives your enabled skills for its sessions.

## MCP servers

MCP servers give an agent extra tools. Freeloop supports servers that run as a command on your Mac and servers you reach over HTTP. Manage them in **Settings → MCP servers**.

### Add a server

Click **Add a server** and fill in the form.

* **ID** names the server. Its tools appear as `mcp__ID__tool`.
* **Transport** is **Command (stdio)** or **Remote (Streamable HTTP)**.
* A command server takes a **Command**, **Arguments** (one per line) and **Variables**.
* A remote server takes a **URL** and optional **Headers**.
* **Secrets**, such as tokens, are stored in the macOS Keychain. They are passed as variables or sent as headers.
* **Offer its tools in every session** turns the server on for everyone. A profile can add servers that are off.

You can also click **Browse a registry…** and choose the official MCP registry or an address of your own. A result only fills in the form. **Import from Claude Code…** copies servers from Claude Code's settings.

### Approve before it runs

A new server shows **needs approval** and the exact command or URL it would run. Click **Approve and start**. A later change to what it runs or where it connects, including its variables, headers or secret names, asks again. Commands that fetch packages, such as `npx`, carry a warning, so pin a version where you can. Servers run with your privileges.

### Sign in

A remote server that wants OAuth shows **needs sign-in**. Click **Sign in…**. The server's page opens in your browser. Tokens are kept in the Keychain, and **Sign out** removes them.

### Using servers

Tools reach the built-in agent as `mcp__server__tool`. Every call asks unless a rule allows it, for example `mcp__github__*`, or the session is in Bypass mode. Plan mode blocks them.

When a session's servers offer resources, an **MCP resource…** button appears next to the message box. Choose one to attach its content to your next message. A server's prompts appear as slash commands marked **MCP prompt**. A prompt with arguments opens a form.

External agents get the same servers. The approved ones that are on, plus those the profile's **MCP servers** field adds, are handed to the agent when the session starts. Remote servers go only to agents that support HTTP.
