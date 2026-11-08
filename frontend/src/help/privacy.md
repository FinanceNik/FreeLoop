# Privacy

This page explains what Freeloop keeps on your Mac, what it sends to model providers, and the few other connections it can make.

## What stays on your Mac

Freeloop keeps its data in one folder, `~/Library/Application Support/freeloop/`, which only your user account can open.

- `sessions/<id>/events.jsonl` is each session's transcript.
- `config.yaml`, `providers.yaml`, `rules.yaml`, `mcp.yaml` and the `profiles`, `environments`, `presets`, `optimizers` and `skills` folders hold your settings and skills.
- `freeloop.db` is the index of sessions and projects.
- `logs/freeloop.log` is the app's log.

All of it is readable and stays on this Mac. Freeloop has no telemetry, analytics or crash reporting.

Transcripts are plain text, protected only by file permissions. Freeloop replaces secrets it knows about before it saves a transcript. It cannot catch a password you type into a message, a key a command prints from somewhere else, or a secret shorter than eight characters.

## What goes to a provider

When the built-in agent calls a model, the provider you chose receives the conversation, the system prompt, the tool definitions and the skill index. That includes file contents the agent read, command output and files you attach with `@`.

To see what goes out, open a session's **Info** tab and look under **What the model sees**. It shows the system prompt and tools for each kind of model call, such as a turn, compaction or a commit message, and notes when they change. A provider's own terms govern what it does with the data. A model served on your own Mac, such as one in Ollama or LM Studio, keeps it there.

An external agent builds its own requests. Freeloop sees only the messages and tool calls in the conversation, so **What the model sees** stays empty for those sessions. The agent talks to its vendor under its own sign-in, and Freeloop never reads or stores that sign-in.

Secrets stored in an environment are removed from transcripts but not from what the model has already received, so a secret a command prints can reach the provider.

## Other connections

Besides what you configure or ask for, such as providers, MCP servers, agents, SSH machines, a skill's git address or a page the agent fetches with your approval, Freeloop connects to three things, and only when you use them:

- The update check, which is off until you turn on **Check for a newer version once a day** in Settings → Network. It asks GitHub for the latest release and sends nothing about your copy of the app. **Check now** runs it once.
- A token optimizer's download, only when you click its **Install** button, such as **Install RTK**, in Settings → Token optimizers. It comes from GitHub, at a pinned version, and is checked against a known checksum.
- The MCP registry, only when you search it from **Browse a registry…** on the MCP servers page. It sends your search words to the registry address you chose.

If you set a search address for the built-in agent's web search in Settings → Network, queries go there. Each search asks you first, unless an allow rule or bypass mode lets it through.

## Local-only mode

In Settings → Network, **Local only: the app connects to nothing but this Mac** limits the app's own connections to this Mac. Providers and MCP servers elsewhere become unreachable, while Ollama, LM Studio and servers on localhost keep working. Each blocked attempt is listed on the same page. Docker containers created while it is on have no network.

It does not cover programs the app starts, such as commands, external agents and stdio MCP servers. They run as you. Use a Docker environment for those.

## Keys and secrets

API keys, MCP sign-in tokens and environment secrets are stored in the macOS Keychain. Files such as `providers.yaml` hold only references to them. Provider keys are never placed in the environment of a command.

## Backups and exports

In Settings → Data, **Back up configuration…** writes settings, providers, profiles, environments, rules, MCP servers, token optimizers and agent presets to one file. API keys and other Keychain secrets are not in it. Values you typed into config files as plain text are, so check the file before you share it. Transcripts and skills are not part of it. **Restore from a backup…** shows what it would do first, and anything that could run commands or loosen your policy waits for your approval.

A session's menu has **Export as Markdown…** and **Export as JSON…**. Each writes the conversation to a file you choose. An export is always something you ask for, and it can hold anything the agent read, so read it before you send it on.

In Settings → Data, under **Report a problem**, **Copy a report** copies a short diagnostic to your clipboard, with known secrets and your home folder's path taken out. Nothing is sent anywhere. The Troubleshooting page says what it holds.
