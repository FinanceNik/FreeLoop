# Skills and MCP servers

Give your agents extra know-how with skills and extra tools with MCP servers.

## What a skill is

A skill is a folder with a `SKILL.md` file that tells an agent how to do one kind of task, plus any scripts and reference files. It uses the open Agent Skills format, so skills written for Claude Code load without changes.

The agent sees only each enabled skill's name and description, and loads the full text when a task matches.

## Install and manage skills

**Settings → Skills** lists every skill Freeloop found. They live in the skill store, the `skills` folder in Freeloop's data folder, or in a trusted project's `.freeloop/skills`, `.agents/skills` or `.claude/skills`. Freeloop never changes project folders, and a project skill hides a store skill with the same name.

Installing copies a skill into the store and leaves the source alone:

- **Install from a folder…** or drop a skill folder or `.zip` on the drop zone.
- Paste a folder path, a `.zip` path or a git URL and click **Preview**. Add `#branch`, `#tag` or `#commit` to pin a git URL.
- **Import from Claude Code** reads `~/.claude/skills`. Nothing is written back.

Every route shows a preview first, with each skill marked new, up to date, changed or cannot load. Tick the ones you want and click the install button.

Each skill has **On everywhere** and, when a project is chosen, **In this project**. A skill that is off is not listed to the model. A profile can also limit a session to named skills in its **Skills** field.

A git skill offers **Check for updates**, then **Update**, and **Roll back** restores the previous version. **New skill…** starts from a template. **Test…** asks a model once whether it would pick the skill for a prompt you write, which uses tokens.

> A skill's scripts never run by themselves. The built-in agent runs them only through the shell tool, so your mode and rules decide each one like any other command. Claude Code also receives your enabled skills.

## Add an MCP server

MCP servers give an agent extra tools. They run as a command on your Mac or are reached over HTTP. Click **Add a server** in **Settings → MCP servers** and fill in the form.

| Field | Meaning |
| --- | --- |
| ID | Names the server. Its tools appear as `mcp__ID__tool`. |
| Transport | Command (stdio) or Remote (Streamable HTTP). |
| Command, Arguments, Variables | For a command server. Arguments go one per line. |
| URL, Headers | For a remote server. |
| Secrets | Kept in the macOS Keychain, passed as variables or sent as headers. |
| Offer its tools in every session | Turns the server on for everyone. A profile can add servers that are off. |

**Browse a registry…** fills in the form from the official MCP registry or an address of your own. **Import from Claude Code…** copies servers from Claude Code's settings.

## Approve and sign in

A new server shows **needs approval** and the exact command or URL it would run. Click **Approve and start**. A later change to what it runs or where it connects, including variables, headers or secret names, asks again.

A remote server that wants OAuth shows **needs sign-in**. Click **Sign in…** and finish in your browser. Tokens stay in the Keychain, and **Sign out** removes them.

> Servers run with your privileges. Commands that fetch packages, such as `npx`, carry a warning, so pin a version where you can.

## Use servers in a session

Tools reach the built-in agent as `mcp__server__tool`. Every call asks unless a rule allows it, for example `mcp__github__*`, or the session is in Bypass mode. Plan mode blocks them.

- **MCP resource…** next to the message box attaches a server's resource to your next message.
- A server's prompts appear as slash commands marked **MCP prompt**.
- External agents get the approved servers that are on, plus those in the profile's **MCP servers** field. Remote servers go only to agents that support HTTP.
