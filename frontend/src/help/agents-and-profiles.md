# Agents and profiles

This page explains the agents Freeloop can run, how you sign in to them, and how profiles choose what a session runs with.

## The built-in agent and external agents

The **built-in agent** is Freeloop's own. It runs on a provider you add, with an API key or a local model, and uses the tools Freeloop gives it: reading and editing files, running commands and searching.

**External agents** are separate programs that Freeloop starts on your Mac and talks to over the Agent Client Protocol. The ones that ship are Claude Code, Codex CLI, Gemini CLI, OpenCode and goose. An external agent brings its own model access, so it does not use your providers. Freeloop never passes it your providers' keys or a vendor login; it gets only the variables and secrets of the session's environment.

The built-in agent asks Freeloop before it changes files or runs commands, and your policy decides. An external agent is different: Freeloop decides only what the agent asks about. It runs its own tools inside its own process, and what it does there without asking, for example under its own settings or in its own auto mode, never reaches Freeloop, so your deny rules do not apply to it. A banner tells you when a session is in auto mode. For an agent you do not trust, use a Docker environment.

If an agent cannot start, for example because Node.js or the program is missing, the new-session form says why.

## Sign in to an agent

A subscription agent such as Claude Code signs in through its own program, in a terminal, not in Freeloop. Freeloop never reads, stores or forwards that login. It starts the unmodified program, and the program does the rest.

For Claude Code, run `claude` in a terminal once and sign in there. The first-run guide and **Settings → Providers** show a note for each agent saying what it needs, and the new-session form shows it for the agent you pick:

- Codex CLI: run `codex login`, or add `CODEX_API_KEY` or `OPENAI_API_KEY` as a secret of the session's environment.
- OpenCode: run `opencode auth login`, or add its API key as a secret of the session's environment. Without either it uses OpenCode's own hosted free models, so your prompts and code go there.
- goose: install it, then run `goose configure` to choose its provider and key.
- Gemini CLI: use an API key, added as `GEMINI_API_KEY` in the environment's secrets. Do not use a Google login with it.

After that, pick the agent with **Use** and its name in the first-run guide, such as **Use Claude Code**, which also makes it the default for new sessions. Or choose its profile when you start a session.

## What a profile sets

A profile is a saved choice of how a session runs. Open **Settings → Profiles** to see them. Each is a YAML file in the `profiles/` folder of the app's data folder, so you can also edit it by hand. The built-in **Default** profile is the built-in agent with every tool.

Choose **New profile**, or **Edit** on an existing one, to set:

- **Harness**: the built-in agent or one of the external agents.
- **Provider**, **Model** and a fallback provider and model, for the built-in agent.
- **Permission mode** the session starts in, and **Effort**.
- **Token optimizer** and **Max model calls per message**.
- **System prompt**, added to the built-in agent's own.
- **Tools**: untick some to offer fewer.
- **Skills** by name (empty means every skill that is on), and **MCP servers** by ID, added to the ones that are on for everyone.
- Whether to read the project's `AGENTS.md` or `CLAUDE.md`.
- **Slash commands**: a name and a prepared prompt.

Rules and hooks live in the profile's YAML. Choose **YAML** to edit it. A hook can stop a tool call; it cannot approve one.

## The default profile

New sessions use the default profile unless you choose another in the new-session form. Change what projects without their own choice use in **Settings → Sessions**, under **Profile**. In the new-session form, when you pick another profile, a checkbox under **Profile** makes it that project's default.

To replace the built-in default, choose **Edit** on **Default** and save. **Reset** brings the built-in one back.

## Switch to another profile

A session keeps its agent. To carry on with another one, open **More actions** and choose a profile under **Continue with**. Freeloop starts a new session on that profile and tells it what happened in the old one. The old session stays as it was.

When a session reaches its account limit or its turn limit, or keeps failing, a banner offers to continue on another profile. In **Settings → Profiles**, **When a session is stuck** sets which profile each agent offers.

## Project profiles

A trusted project can add profiles in its `.freeloop/profiles/` folder. They appear in the new-session form in their own group, marked *this project's .freeloop/*. A project can only tighten what you allow: its profiles use the built-in agent, start in Ask or Plan, add only deny rules, and cannot add MCP servers, a token optimizer or a fallback model.

## Instruction files

For a trusted project, the built-in agent reads `AGENTS.md` first, and `CLAUDE.md` if there is no `AGENTS.md`. Freeloop itself never writes to them; an agent that wants to change one asks like for any other edit. In an untrusted project they are not used. A profile can turn this off.
