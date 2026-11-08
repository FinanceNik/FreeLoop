# Privacy

What Freeloop keeps on your computer, what it sends to model providers, and what else it can connect to.

## What stays on your computer

Freeloop keeps its data in one folder, {{`~/Library/Application Support/freeloop/`|`%AppData%\freeloop\`|`~/.config/freeloop/` (or under `$XDG_CONFIG_HOME`)}}, which only your user account can open. All of it is readable and stays on this computer. There is no telemetry, analytics or crash reporting.

- `sessions/<id>/events.jsonl`: each session's transcript
- `config.yaml`, `providers.yaml`, `rules.yaml`, `mcp.yaml` and the `profiles`, `environments`, `presets`, `optimizers` and `skills` folders: your settings and skills
- `freeloop.db`: the index of sessions and projects
- `logs/freeloop.log`: the app's log

API keys, MCP sign-in tokens and environment secrets are in {{the macOS Keychain|Windows Credential Manager|the system keyring}}. Files such as `providers.yaml` hold only references to them, and provider keys are never placed in the environment of a command.

> Transcripts are plain text, protected only by file permissions. Freeloop replaces secrets it knows about before it saves a transcript. It cannot catch a password you type into a message, a key a command prints from somewhere else, or a secret shorter than eight characters.

## What goes to a provider

When the built-in agent calls a model, the provider you chose receives the conversation, the system prompt, the tool definitions and the skill index. That includes file contents the agent read, command output and files you attach with `@`.

A provider's own terms govern what it does with the data. A model served on your own computer, such as one in Ollama or LM Studio, keeps it there. To see what goes out, open a session's **Info** tab and look under **What the model sees**.

> An external agent builds its own requests. Freeloop sees only the messages and tool calls, so What the model sees stays empty for those sessions. The agent talks to its vendor under its own sign-in, and Freeloop never reads or stores that sign-in.

## Other connections

Besides what you configure or ask for, such as providers, MCP servers, agents, SSH machines, a skill's git address or a page the agent fetches with your approval, Freeloop connects to three things, and only when you use them:

- The update check is off until you turn on **Check for a newer version once a day** in Settings → Network. It asks GitHub for the latest release and sends nothing about your copy of the app. **Check now** runs it once.
- A token optimizer's download happens only when you click its **Install** button, such as **Install RTK**, in Settings → Token optimizers. It comes from GitHub, at a pinned version, and is checked against a known checksum.
- The MCP registry is contacted only when you search it from **Browse a registry…** on the MCP servers page. It sends your search words to the registry address you chose.

If you set a search address for the built-in agent's web search in Settings → Network, queries go there. Each search asks you first, unless an allow rule or bypass mode lets it through.

## Local-only mode

In Settings → Network, **Local only: the app connects to nothing but this computer** limits the app's own connections to this computer.

- Providers and MCP servers elsewhere become unreachable. Ollama, LM Studio and servers on localhost keep working.
- Each blocked attempt is listed on the same page.
- Docker containers created while it is on have no network.

> It doesn't cover programs the app starts, such as commands, external agents and stdio MCP servers. They run as you. Use a Docker environment for those.

## Backups, exports and reports

- **Back up configuration…** in Settings → Data writes your settings, providers, profiles, environments, rules, MCP servers, token optimizers and agent presets to one file. Secrets, transcripts and skills aren't in it.
- **Restore from a backup…** shows what it would do first. Anything that could run commands or loosen your policy waits for your approval.
- **Export as Markdown…** and **Export as JSON…** in a session's menu write the conversation to a file you choose, only when you ask.
- **Copy a report**, under **Report a problem** in Settings → Data, copies a short diagnostic to your clipboard, with known secrets and your home folder's path taken out. Nothing is sent anywhere.

> A backup holds any values you typed into config files as plain text, so check it before you share it. An export can hold anything the agent read, so read it before you send it on. The Troubleshooting page says what a report holds.
