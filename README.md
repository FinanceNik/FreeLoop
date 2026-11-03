# Freeloop

An open-source macOS app for running AI coding agents with your own models, switchable harnesses, Agent Skills, MCP servers and custom environments, built for running many sessions across many repositories at once. Apache-2.0.

Status: early, and not yet released. Most of what v1 lists is built; [docs/ROADMAP.md](docs/ROADMAP.md) says what each milestone covers and what remains. Releases will not be signed or notarized. The specification is in `docs/`.

## What it does

- **Your models.** The built-in agent runs on providers you add: Anthropic, OpenAI-compatible endpoints (OpenAI, OpenRouter, LM Studio, llama.cpp, vLLM) and Ollama. API keys go to the macOS Keychain, never into files.
- **Switchable harnesses.** Besides the built-in agent, any agent that speaks the Agent Client Protocol runs as a harness from a preset. Claude Code ships as one: it runs the unmodified Claude Code through its ACP adapter and signs in by itself; Freeloop never sees or stores its login.
- **Profiles.** A profile sets a session's harness, model, tools, prompt, permission rules, skills, MCP servers and slash commands, as a YAML file.
- **Skills and MCP.** Agent Skills (`SKILL.md`) load unchanged, from the app's store or a trusted project. MCP servers run over stdio or Streamable HTTP after you approve exactly what they run. Both can be imported from Claude Code.
- **One permission chokepoint.** Every tool call from every harness is decided by one broker: modes (ask, accept edits, plan, bypass), rules at global, environment, profile, project and session scope where deny always wins, and protected paths that are never touched.
- **Many sessions.** A sidebar by project, an attention view that answers every waiting approval in one place, notifications and a dock badge, a diff and commit panel per session, and an optional git worktree per session so sessions on one repository never edit the same checkout, merged back from the panel.
- **Containers.** A Docker environment runs each session's commands and agents in its own container, from an image or the project's devcontainer.json, with only the project folder shared (Docker Desktop, or Colima and the docker CLI).
- **Projects you trust.** A project's `AGENTS.md`, `.freeloop/` settings and git status are used only after you trust it; its own settings can tighten permissions, never loosen them.

## Install

Build from source on macOS 14 or later. You need Go, Node.js with npm and the Xcode command line tools.

```sh
make tools     # once: install the pinned wails3 CLI
make package   # build bin/Freeloop.app, ad-hoc signed
make dmg       # optional: bin/Freeloop.dmg with the app and a link to Applications
```

Move `bin/Freeloop.app` to Applications, or open the DMG and drag it there. A build you made yourself opens normally. Releases are not signed or notarized, so macOS blocks a downloaded one the first time: try to open it, then choose Open Anyway in System Settings, under Privacy & Security. Check it against the release's `SHA256SUMS` first.

## First run

1. **Add a provider.** The guide opens on a fresh install: pick a kind (Anthropic, OpenAI, OpenRouter, Ollama…), paste an API key or a local URL, and test it. If you only want Claude Code, skip this: it brings its own model access and needs Node.js and one sign-in with `claude` in a terminal.
2. **Import from Claude Code (optional).** MCP servers, permission rules and skills are listed with where each goes; nothing is written back to Claude Code's files. Existing sessions can be imported later from the same page, and continue here: the agent copies each one, so the original stays as it is.
3. **Choose a project** and decide whether to trust it.
4. **Start a session** and describe a task. Approvals appear in the conversation and in "Needs attention"; Y, A and N answer them.

## Where things live

Everything is in `~/Library/Application Support/freeloop/`, readable and editable by hand; edits are picked up within a second.

| File | Holds |
|---|---|
| `config.yaml` | settings and defaults |
| `providers.yaml` | providers (keys are Keychain references) |
| `profiles/*.yaml` | profiles |
| `environments/*.yaml` | environments: variables, secrets as references, rules |
| `rules.yaml` | global permission rules |
| `mcp.yaml` | MCP servers |
| `presets/harnesses/*.yaml` | external agents |
| `optimizers/*.yaml` | token optimizers of your own |
| `colors.json` | custom colors |
| `logs/freeloop.log` | the app's log, which stays on this Mac; Settings shows it in Finder |
| `skills/` | the skill store |
| `sessions/<id>/events.jsonl` | each session's transcript |

A project can add `.freeloop/rules.yaml` (deny rules) and `.freeloop/profiles/`, read once the project is trusted. Set `FREELOOP_HOME` to use another folder.

Freeloop has no telemetry. It connects to the providers, MCP servers and agents you configure, and to three things only if you use them: the update check you can turn on in Settings (GitHub), a token optimizer's pinned download when you click it (GitHub), and the MCP registry when you browse it. [docs/SECURITY.md](docs/SECURITY.md) describes the threat model and the limits you should know, among them that the local environment is not a sandbox.

## Develop

```sh
make dev       # run with live reload; adds a scripted demo provider
make test      # Go tests with the race detector, then frontend tests
make lint      # formatting, vet, architecture boundaries, TypeScript
```

Start with [docs/DEVELOPING.md](docs/DEVELOPING.md) for how the code fits together and how to extend it, [CLAUDE.md](CLAUDE.md) for the rules the code follows, then [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). Contributions: see [CONTRIBUTING.md](CONTRIBUTING.md).
