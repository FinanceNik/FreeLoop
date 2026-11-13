# Vision

**Freeloop** is a free, open-source desktop app for macOS, Windows and Linux that puts your policy and your bill over every agent you run: Claude Code, Codex, Copilot CLI, Gemini CLI, OpenCode, Goose, or its own built-in agent, which needs no vendor and runs on your own models. It is built for people who run many sessions at once.

## The problem

Coding agents have become a primary way software gets written, and the best ones tie you to one vendor's models, one vendor's agent loop and one vendor's idea of where code runs. Someone who runs five agent sessions across five repos juggles five terminal windows, or a vendor desktop app that only talks to that vendor's models. Switching to a cheaper model, a local model or a different agent loop means switching tools and re-learning everything: where skills live, how permissions work, how to see what changed, which session is waiting on you.

The pieces needed to fix this now exist as open protocols. The Agent Client Protocol (ACP) lets a host drive any agent. The Model Context Protocol (MCP) gives agents tools. Agent Skills define reusable instruction folders. `AGENTS.md` carries project rules. What is missing is a desktop app that puts them together for a person who runs many sessions in parallel and wants one place to see what needs attention.

## What Freeloop is

A host for coding agents: a desktop app for macOS, Windows and Linux that keeps your policy and your bill over every agent you run. The Mac app is in daily use; the Windows and Linux builds have not yet been run on a Windows or Linux desktop. In it you:

- put every agent under one permission policy: what an agent asks to do is decided by your modes and rules, and no file an agent or a repository writes can loosen them ("How far the claim goes" below says what has been tested and where it ends);
- see one bill: tokens and estimated cost by agent, model and project;
- choose the agent: any external agent that speaks ACP, with presets for Claude Code, Gemini CLI, Codex CLI, Goose, OpenCode and Copilot CLI, or the built-in agent, which needs no vendor: it is configured by declarative profiles and runs on models you bring through API keys or local runtimes (Anthropic, OpenAI, Google, OpenRouter, Ollama, LM Studio, llama.cpp, vLLM, any OpenAI-compatible endpoint);
- define the environment an agent runs in: a local folder, a native sandbox on a Mac and on Linux, a Docker container, or another machine over SSH, each with its own tools, permissions, env vars and secrets;
- install and author skills in the Agent Skills format and manage MCP servers once; MCP servers are handed to every harness, skills to the built-in agent and, of the external ones, to Claude Code so far;
- run all your sessions across all your repos from one window with one approval inbox, and follow them from a phone you paired.

Claude Code's workflow is the inspiration. Freeloop is not built on it: Freeloop is the host, and Claude Code is one of several external agents it runs, using your own subscription through Claude Code's own sign-in, which the app never touches. The built-in agent is the one that needs no vendor.

## Who it is for

- **Now: the author.** A heavy Claude Code user running several sessions in parallel across multiple repos, who wants model freedom, custom harnesses and a GUI designed for parallel work rather than for one conversation.
- **At v1: developers who already use a CLI coding agent** and want one GUI over several agents and models, including local ones, without giving up control over permissions and secrets.
- **Later, perhaps:** people running non-coding work agents. Nothing in the core assumes code, and nothing in the roadmap before v1 serves them.
- **Not for:** non-technical users who need a finished product rather than a tool; teams that need shared infrastructure, accounts or central audit; anyone who wants a hosted service.

## Positioning

| Category | Examples | What they do well | Where Freeloop differs |
|---|---|---|---|
| Vendor agents with a GUI | Claude Code desktop app, Codex app | Polished, multi-session, cloud and SSH environments | One vendor's models; closed source; features gated on the vendor's account |
| Open-source agents | OpenCode, Goose, Cline, Aider, Gemini CLI | Bring your own key, local models, most speak ACP | Each is a harness, not a host. In Freeloop they are harnesses you switch between, not competitors |
| Parallel-session orchestrators | Conductor, Crystal, Vibe Kanban, Claude Squad | One worktree per task, diff review, run scripts | They force a worktree per task, review per workspace only, offer no cross-session approval inbox, and couple to one or two vendors |
| ACP hosts | Zed agent panel, JetBrains IDEs | Reference ACP clients, registry install flow | Editors first, one session in focus, no environment model |

### What is and is not unique (checked 2026-11-04)

Parallel sessions, worktrees, several agents in one window and your own models are common now: an October 2026 comparison lists eight desktop apps that run agents in parallel, and the ACP site lists many desktop clients. None of that is a reason to choose Freeloop.

What is left is one policy and one bill across agents. Each vendor has its own layers for its own agent:

| Agent | Who can loosen it | What holds above that |
|---|---|---|
| Claude Code | `.claude/settings.json` in a repository can allow commands | Managed settings, deployed by IT |
| Codex CLI | `.codex/config.toml`, once the folder is trusted | `requirements.toml`, for enterprises |
| Cursor | The project's `sandbox.json` takes priority over the user's when merged | Team and enterprise settings; not looked into |
| Zed with Codex | Codex's own config stays the source of truth for permissions | Each agent's own |
| Freeloop | No file an agent or a repository writes can loosen it | The user's broker, for every agent |

Not yet known: how Conductor, Superset and similar orchestrators handle this. The searches found nothing on them, so do not claim they cannot. AG2 and clai show the idea is not unheard of, with an approve-or-deny setting and reports of denials respectively.

**How far the claim goes.** `make verify-agents` runs the real agents on a stand-in model in a repository whose own config tells them to stop asking. Claude Code is tested with a `git push` and a control case that shows a push would get through: allow rules, a default bypass mode, local settings and hooks all left it to the user, after three fixes (docs/SECURITY.md, review of 2026-11-05). Gemini CLI, Codex, OpenCode and Copilot CLI are tested with a file write, and each asked; Copilot CLI runs a repository's hooks itself, so in an untrusted project it does not start while there are any. Not shown yet: Goose, which has no pinned build for this Mac; shell commands in those four; real models other than Claude Code's, in one earlier manual run. In Claude Code's auto mode, which the user chooses per session, the agent approves for itself and the app's deny rules do not reach it. An approved shell command still runs as the user unless the environment is sandboxed, and only a sandbox holds against an agent that does not ask. Say "nothing runs without your policy", and the hard guarantee only with a sandbox.

### Open-source ACP desktop clients (found 2026-11-11, read 2026-11-12)

Six open-source desktop clients for ACP agents were found on 2026-11-11. On 2026-11-12 the front page of each one's repository or site was read, through a tool that fetches a page and answers questions about it. No code was read and none of them was run. Each line is what the project says of itself; "does not mention" means only that the page does not.

- **ACP UI** ([formulahendry/acp-ui](https://github.com/formulahendry/acp-ui), MIT): a client for ACP on Windows, macOS, Linux, Android and the web, with iOS from source. It lists "Approve or deny agent actions before execution". The page does not mention cost or tokens.
- **OneAgent** ([RaspberryCola/OneAgent](https://github.com/RaspberryCola/OneAgent), MIT): "A unified desktop & web workspace for multiple AI coding agents", for macOS 12 or later, Windows 10 or later and Linux. It lists a review of file writes and commands with allow once, allow always, reject once and reject always. The page does not mention cost or tokens.
- **Harnss** ([OpenSource03/harnss](https://github.com/OpenSource03/harnss), MIT): a desktop client for Claude Code, Codex and any other ACP agent, for macOS, Windows and Linux. It lists three permission levels: Ask First, Accept Edits and Allow All. The page does not mention cost or tokens.
- **Agentaps** ([cachix/agentaps](https://github.com/cachix/agentaps), Apache-2.0): "A universal GUI for coding harnesses", for Linux, macOS on Apple Silicon and Windows; it names Codex, Claude Code, Gemini CLI and OpenCode. The page does not mention permissions, cost or tokens.
- **Crafter Code** ([code.crafter.run](https://code.crafter.run/), which says MIT): "a desktop app for running multiple AI agents side by side", for macOS, with Claude Code and Gemini CLI supported, Codex CLI "Coming Soon" and any ACP-compatible agent. The site does not mention permissions or token use. Its repository was not read.
- **Acepe**: found, not read. The repository address the search gave, `github.com/flazouh/acepe`, answered "not found" on 2026-11-12.

Not checked for any of them: whether a repository's or an agent's own config can loosen what the client asks about, whether one set of rules holds across agents, and whether use is counted across agents. So do not claim that they cannot.

Sources: [parallel desktop apps](https://pasqualepillitteri.it/en/news/24114/desktop-app-parallel-ai-coding-agents), [ACP clients](https://agentclientprotocol.com/overview/clients), [Claude Code settings](https://code.claude.com/docs/en/settings), [Codex sandboxing](https://developers.openai.com/codex/concepts/sandboxing.md), [Cursor run modes](https://cursor.com/docs/agent/run-modes), [Codex in Zed](https://codex.danielvaughan.com/2026/05/05/codex-cli-in-zed-parallel-agents-acp-integration-ide-workflows/), [AG2](https://docs.ag2.ai/docs/user-guide/cli_agents/).

## What to borrow, what to avoid

Borrow:

- Codex CLI's split between a **sandbox mode** (what is technically possible) and an **approval policy** (when to ask). Freeloop maps sandbox onto the environment and policy onto rules, so policy can vary per environment.
- Gemini CLI's **declarative policy engine** that matches on tool kind, MCP server wildcards and paths.
- OpenCode's **named agents with baked-in permission profiles**, switchable without leaving the session. Freeloop's profiles are the same idea with model, tools, skills and MCP servers added.
- Goose's **recipes**: shareable, versioned agent configurations. Profiles are files for the same reason.
- Conductor's **Checks tab** and archive-after-merge lifecycle for a session's branch.
- Zed's **ACP registry install flow**: pick an agent, the host fetches the adapter.
- Claude Code's **permission modes, plan mode, skills format and progressive disclosure**, and its approval dialog with "allow always" creating a rule.

Avoid:

- Single-vendor model lock and account-gated features.
- Forcing a worktree per task. Users of every orchestrator complain about setup cost; Freeloop makes it an option.
- Fixing the permission mode when the session starts. Modes and rules change mid-session.
- A terminal UI's mental model inside a GUI.
- Extension only through MCP (Goose) with no lightweight skills layer.
- Assuming feature parity across agents. Capabilities are negotiated and the UI degrades per agent.
- Depending on any orchestrator product as a base. Several were abandoned or absorbed within a year of launch. Build on protocols instead.

## Where Freeloop is genuinely different

In one sentence: your policy and your bill over every agent you run. Every coding agent, vendor or local, runs under your permission policy and your accounting, and neither the agent nor the repository can loosen either, as far as "How far the claim goes" above says (`docs/ROADMAP.md`, direction of 2026-11-03). The five below are what that rests on.

1. **One approval inbox across sessions and repos.** None of the orchestrators looked at on 2026-11-04 did this; whether the ACP clients read on 2026-11-12 do was not checked. Every permission request from every harness lands in one queue with inline approve and a jump to context.
2. **Any model and any harness under one permission model.** The GUI never special-cases a harness. The built-in agent, which needs no vendor, and external ACP agents render the same, are approved the same and are logged the same, and their tokens and estimated cost are counted in one place.
3. **Environments are first-class.** Tools, secrets and permission policy belong to the environment, not the agent. A session runs in a folder, under a native sandbox on a Mac and on Linux, in a Docker container, or with its commands on another machine over SSH.
4. **Harness switching by profile, harness creation in any language.** Switching is a dropdown. Writing a new harness is a template that speaks ACP, in Python or Go or anything else, running out of process.
5. **Local-first and honest.** No telemetry, no account, no service of ours in between, no token tricks. Everything the app stores is a readable file or a SQLite database you can open.

## Principles

- **Protocols over products.** ACP, MCP, Agent Skills and `AGENTS.md` are the integration surface. Product-specific adapters are isolated and replaceable.
- **The GUI never special-cases a harness.** If a feature needs harness-specific UI, the harness interface is wrong.
- **Everything pluggable is a file or a process.** No in-process plugin loading. Profiles, skills, MCP servers, environments and harnesses are configured as files and run as processes.
- **Local-first.** Fully functional offline with a local model. Nothing phones home; the update check is off until you turn it on.
- **Safe by default, quiet when trusted.** Ask by default; rules make it quiet; bypass is explicit, visible and per session.
- **Readable state.** YAML config, JSONL transcripts, SQLite index, documented event schema.
- **Respect vendor boundaries.** Subscriptions are used only through unmodified vendor binaries doing their own sign-in.
- **Small core, additive milestones.** Interfaces land early with one implementation; new implementations never require a refactor of the GUI.

## Non-goals

- Not a Claude Code clone or wrapper. Claude Code is one harness.
- Not an editor or IDE. Files open in the editor you already use.
- Not a hosted service. No accounts, no sync, no server of ours. The one server is in the app itself: off until turned on, it listens on 127.0.0.1 and answers phones you paired, reached over your own Tailscale network (GUI-052).
- No team features (shared sessions, central policy, audit export) before v1 is out.
- No model hosting or inference of our own.
- No intermediation of vendor subscriptions or consumer OAuth tokens, ever.
- Not a general chat client.
- No marketplace or registry hosting in v1. Install from a folder or a git URL; an MCP registry whose address you set can be browsed from the app.

## What success looks like

- **M1:** the author uses it for at least one real task a day for a week and prefers it for that task.
- **MVP:** the author stops using the vendor desktop app for parallel sessions; two weeks without transcript loss or session cross-talk.
- **v1:** five people other than the author use it for a week and their blocking issues are fixed. Builds are not signed for Apple or Microsoft (OQ-057).
- **Long run:** a new agent appearing in the ACP registry works in Freeloop the same day, with a configuration entry and no code.
