# Agents and profiles

Choose which agent a session runs, sign in to it and save the setup as a profile.

## Built-in and external agents

| Agent | What it is |
| --- | --- |
| Built-in | Freeloop's own. It runs on a provider you add and uses Freeloop's tools: reading and editing files, running commands and searching. |
| External | A separate program Freeloop starts, such as Claude Code, Codex CLI, Copilot CLI, Gemini CLI, OpenCode or goose. It brings its own model access, so it doesn't use your providers. |

Freeloop never passes an external agent your providers' keys or a vendor login. If an agent can't start, the new-session form says why.

> With the built-in agent, your policy decides every file change and command. An external agent runs its own tools in its own process, and only what it asks about reaches Freeloop, so your deny rules don't apply to the rest. For an agent you don't trust, use a Docker environment.

## Sign in to an external agent

A subscription agent such as Claude Code signs in through its own program, in a terminal. Freeloop starts the unmodified program and never reads, stores or forwards that login.

**Settings → Providers** and the new-session form show what each agent needs:

| Agent | What it needs |
| --- | --- |
| Claude Code | Run `claude` once and sign in. |
| Codex CLI | Run `codex login`, or add `CODEX_API_KEY` or `OPENAI_API_KEY` as a secret of the session's environment. |
| Copilot CLI | A Copilot plan, your own or one an organization gives you. Run `npx @github/copilot login` once. Each message uses the plan's premium requests, and the organization's Copilot policy must allow Copilot CLI. In a project you haven't trusted, it doesn't start while the project has hooks for it (`.github/hooks`, or hooks in `.github/copilot/settings.json` or `.claude/settings.json`), since it would run them without asking. |
| OpenCode | Run `opencode auth login`, or add its API key as a secret of the environment. Without either it uses OpenCode's hosted free models, so your prompts and code go there. |
| goose | Install it, then run `goose configure`. |
| Gemini CLI | Add an API key as `GEMINI_API_KEY` in the environment's secrets. Don't use a Google login with it. |

Then pick the agent with **Use** and its name in the first-run guide, which also makes it the default for new sessions. Or choose its profile when you start a session.

## Create a profile

A profile is a saved choice of how a session runs, kept as a YAML file in the `profiles/` folder of the app's data folder. The built-in Default profile is the built-in agent with every tool.

Open **Settings → Profiles** and choose **New profile**, or **Edit** on an existing one. You can set:

- Harness: the built-in agent or an external one.
- Provider, Model and a fallback, for the built-in agent.
- Permission mode, Effort, Token optimizer and Max model calls per message.
- System prompt, added to the built-in agent's own.
- Tools, Skills and MCP servers to offer.
- Whether to read the project's `AGENTS.md` or `CLAUDE.md`.
- Slash commands: a name and a prepared prompt.

Rules and hooks live in the YAML. Choose **YAML** to edit it.

> A hook can stop a tool call. It can't approve one.

## Choose and switch profiles

- New sessions use the default profile unless you choose another in the new-session form.
- Change the default in **Settings → Sessions**, under Profile.
- To replace the built-in default, choose Edit on Default and save. **Reset** brings the built-in one back.

A session keeps its agent. To carry on with another, open **More actions** and choose a profile under **Continue with**. Freeloop starts a new session on it and tells it what happened in the old one, which stays as it was.

When a session reaches its account or turn limit, or keeps failing, a banner offers to continue on another profile.

## Project profiles

A trusted project can add profiles in its `.freeloop/profiles/` folder. They appear in the new-session form in their own group.

> A project can only tighten what you allow. Its profiles use the built-in agent, start in Ask or Plan, add only deny rules, and can't add MCP servers, a token optimizer or a fallback model.

For a trusted project, the built-in agent reads `AGENTS.md` first, and `CLAUDE.md` if there's no `AGENTS.md`. Freeloop never writes to them, and a profile can turn this off.
