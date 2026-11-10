# Roadmap

Milestones for **Freeloop**, from a thin vertical slice to v1. Each milestone lists the requirement IDs it delivers (see [REQUIREMENTS.md](REQUIREMENTS.md)), a definition of done, and a size.

## How sizes are estimated

Sizes are in **focused weeks**: a week of roughly 12 hours of concentrated work by one person with heavy AI-assisted coding. The planning interview assumed solo development at about 10 to 15 hours per week, so calendar time is close to focused weeks. Estimates for GUI work are the least reliable; polish expands to fill any time given to it, so each milestone names the polish it explicitly does not include.

| Milestone | Focused weeks | Cumulative |
|---|---|---|
| M0 Foundations | 2 | 2 |
| M1 Thin vertical slice | 7 | 9 |
| M2 MVP | 10 | 19 (about 5 months) |
| M3 v1 | 16 | 35 (about 9 months) |

If M1 overruns by more than two weeks, cut from M2 in the order given under "Cut order" rather than stretching M1.

## M0 Foundations (2 weeks)

Goal: a skeleton that proves the stack and fixes the boundaries everything else builds on.

Status: built 2026-10-28.

Delivers: project layout, Apache-2.0 license, Wails v3 shell with a React/TypeScript frontend, Go engine as its own package tree (`internal/`, one Go module) with an in-process API and a boundary test keeping Wails out of it, event schema v0 (EXT-002), SQLite plus JSONL store (SES-002 skeleton), config loading (SET-001 skeleton), Keychain wrapper behind an interface (NFR-XPLAT-002), CI on a macOS runner (NFR-REL-003), lint rule enforcing ENV-001.

Definition of done:
- `make dev` opens a window that lists sessions from SQLite and creates a new empty one.
- An event appended through the engine lands in a session's JSONL file and shows in the UI through the Wails event bridge.
- A secret can be stored and read back through the Keychain interface, with a fake implementation for tests.
- CI builds the app and runs engine tests on every push.

Not in M0: any model call, any tool, any styling beyond defaults.

## M1 Thin vertical slice (7 weeks)

Goal: replace one of your daily Claude Code sessions with Freeloop for a real task, on a real repo, with your own API key and a local model, with several sessions open at once.

Status (2026-10-28): slice 1 of M1, the agent loop end to end, is built. In: `Environment` with the local implementation (ENV-001, ENV-002 without per-environment config), the permission broker with modes, protected paths and reads outside the root (SEC-001, SEC-003, SEC-005, SEC-007, SEC-013), the Anthropic provider (PRV-001 partial, PRV-004, PRV-007), the built-in harness with the core tools and plan mode (HRN-001, HRN-002 without skills and MCP, HRN-009, HRN-010 root files only), restore on restart (SES-003), secret redaction by value (SEC-004), the provider form (PRV-002), and the conversation with approvals, composer, cancel and queueing (GUI-003 without markdown or virtualization, GUI-004 without mentions, GUI-006 without "allow always", GUI-007). Slice 2 (2026-10-29) added the OpenAI-compatible adapter with presets for OpenAI, OpenRouter, LM Studio, llama.cpp, vLLM and Ollama's `/v1`, the native Ollama adapter, model discovery in the new-session form, and capability resolution with the tool-calling probe, a probe cache and per-model overrides (PRV-001, PRV-003 without favorites and hidden models, PRV-005). Slice 3 (2026-10-29) added the rule engine with global rules in `rules.yaml` (reloaded after hand edits) and session rules kept in the transcript, "Allow always" with a scope choice, and a rules page (SEC-002 global and session scopes, GUI-006 without keyboard shortcuts). Slice 4 (2026-10-29) added projects with a git fingerprint and relink (ENV-003, OQ-020), environments in `environments/<id>.yaml` with variables, Keychain secrets injected at process start, disabled tools and a default mode (ENV-002 without default profile, ENV-004 without temp-file secrets), git state per session with the base commit recorded at start (SES-006; the diff panel is M2), and the sidebar grouped by project (GUI-001 without the right panel). A hardening pass (2026-10-29) fixed the findings of an independent review; `docs/SECURITY.md` lists them. Slice 5 (2026-10-29) added titles from the first message, renaming, search and status filters (SES-004; harness and date filters wait for a second harness and for search), archive and delete (SES-005), nested `AGENTS.md` loaded when their folder is touched (HRN-010), a hand-written Markdown renderer that never inserts HTML, with code copy buttons, memoized conversation items and `content-visibility` for long transcripts (GUI-003; the 60 fps target is not yet measured), theme and font size (GUI-011) and defaults (SET-004 without profile, notification and editor settings). Slice 6 (2026-10-29) added favorite and hidden models in the new-session form, kept in `providers.yaml` (PRV-003), `@` file mentions with a fuzzy picker whose files the engine embeds in the prompt (GUI-004), and keyboard shortcuts on approval cards: Y, A and N outside text fields, ⌘↩, ⌘⇧↩ and ⌘⇧⌫ anywhere, ignored for 400 ms after a card appears (GUI-006). Slice 7 (2026-10-29) added the right panel with Approvals (waiting and decided, with quick answers), Changes (files the agent's file tools edited, from the transcript; the diff against the base commit is GUI-005) and Info (session details, token totals, session rules) tabs (GUI-001), and hot reload: the engine checks `config.yaml`, `providers.yaml`, `rules.yaml` and `environments/*.yaml` every second, reloads what changed, reports problems instead of failing, and tells the UI to refresh (SET-001; profiles do not exist yet). Left in M1: the definition-of-done runs on real providers.

Delivers:
- Providers: PRV-001 (Anthropic, OpenAI-compatible, Ollama native), PRV-002, PRV-003, PRV-004, PRV-005, PRV-007.
- Harness: HRN-001, HRN-002 (built-in loop with the core tools), HRN-009 (plan mode), HRN-010 (instruction files).
- Environments: ENV-001, ENV-002, ENV-003, ENV-004.
- Permissions and security: SEC-001, SEC-002 (global and session scopes only), SEC-003, SEC-004, SEC-005, SEC-007, SEC-009, SEC-010, SEC-013.
- Sessions: SES-001, SES-002, SES-003 (built-in harness only), SES-004, SES-005, SES-006.
- GUI: GUI-001, GUI-003, GUI-004 (without slash commands), GUI-006, GUI-007, GUI-011.
- Settings: SET-001, SET-004.
- Extensibility: EXT-002.
- Non-functional: NFR-PERF-001, NFR-PERF-005, NFR-PRIV-001, NFR-PRIV-002, NFR-PRIV-003, NFR-OFF-001, NFR-OFF-002, NFR-REL-001.

Definition of done:
- Three sessions on three different repos run at the same time, one on Anthropic, one on OpenRouter, one on Ollama, each completing "add a test for X and make it pass" with edits approved through the UI.
- Quitting and relaunching restores all three with full history and the built-in sessions continue in context.
- `grep` of the config directory and transcripts for the API key value finds nothing.
- You use it for at least one real task per day for a week and prefer not to go back for that task.

Not in M1: profiles (one hard-coded default profile), external harnesses, skills, MCP, diff panel (use git in a terminal), notifications, settings UI beyond the provider form.

## M2 MVP (10 weeks)

Goal: the first public release. It must hold the five pillars at a minimum credible level: any model, switchable harnesses, skills and MCP, a configurable local environment, and all of it open source.

Status (2026-10-29): M2 slice 1 added the attention view (GUI-002): a sidebar entry with a count opens a list of every session waiting for approval, waiting for input or in error, across projects, oldest request first, with Allow once, Allow always (this session) and Deny on each request, the error message and a Dismiss button for a failed session, and a jump to each session. A failed turn now leaves its session in the `error` status until the next prompt or a dismiss. Slice 2 added the diff and commit panel (GUI-005) as the Diff tab of the right panel: every file that differs from the session's base commit (added, modified, deleted, untracked; changes committed since the start shown as committed), a unified diff per file with line numbers, stage and unstage per file or all, commit with a message the session's model can suggest, and discard with a second click; files the agent's tools edited are marked and link to the edit. Slice 3 added harness profiles (HRN-003, HRN-004 for the built-in harness): `profiles/<id>.yaml` with name, description, harness, provider, model, effort, default mode, a system prompt added to the built-in one, a tool list, profile-scope rules, a turn cap, whether project instruction files are read, and slash commands; unknown fields and tool names are reported with file, line and field; files are hot-reloaded; a built-in default applies until `profiles/default.yaml` exists. The new-session form lists profiles grouped by harness with the project's default (else the one in Settings) preselected and can make the chosen one the project's default; a Profiles page edits the common fields and commands. Skills and MCP servers join profiles with SKL-002 and MCP-001; project profiles in `.freeloop/profiles/` wait for SEC-006 and SET-002. Slice 4 added slash commands (GUI-004): typing `/` at the start of the composer opens a picker of the app's mode commands (`/ask`, `/accept-edits`, `/plan`, `/bypass` when allowed), which change the mode without sending anything, and the commands the session's harness advertises in `available_commands_update`; for the built-in harness those are the profile's commands, and `/name text` sends the command's prompt with the text appended while the transcript keeps what was typed. Slice 5 added project trust (SEC-006) and project settings (SET-002): a project is untrusted until the user trusts it from the new-session form or the session's banner (projects from before this slice start untrusted too); an untrusted project's sessions run with global settings only, without its `AGENTS.md`/`CLAUDE.md`, `.freeloop/` or `git status` (only `rev-parse` and `symbolic-ref` run, and the diff panel is off). A trusted project's `.freeloop/rules.yaml` adds deny rules (a project deny overrides a global allow; allow rules are reported and ignored) and `.freeloop/profiles/*.yaml` add profiles listed for that project only, which may start in ask or plan and carry deny rules only. Shell deny rules now also see through git's global options (`git -C . push`). `.freeloop/` environment defaults, skills and MCP servers arrive with ENV, SKL and MCP work. Slice 6 added notifications (GUI-008): while the window is not focused, a session that needs an approval, fails or (optionally) finishes a turn posts a macOS notification naming the session; clicking it opens that session; the dock badge counts what waits, as the attention view does; Settings turns notifications and finished-turn notices on or off and mutes projects one by one. Native delivery needs the app bundle, so `make dev` and server mode show none; macOS asks for permission with the first notice. Slice 7 added the first-run guide (GUI-010): on a fresh install (no sessions, `ui.onboarded` not set in `config.yaml`) the app opens a three-step guide to add and test a provider, choose a first project and decide whether to trust it, and start a session in the new-session form with that folder filled in; skipping or finishing sets `ui.onboarded`, and the empty screen links back to it. Importing Claude Code skills and MCP servers joins the guide with SKL-003 and MCP-002. Verified in server mode from an empty data folder to a first reply from the demo provider; the five-minute target with a real API key is part of the real-provider runs. Slice 8 added skills (SKL-001, SKL-002, SKL-004): Agent Skills are found in the app's skill store (`skills/` in the data folder) and, in trusted projects, in `.freeloop/skills`, `.agents/skills` and `.claude/skills` (read-only, in that precedence); frontmatter is read leniently so Claude Code skills load unchanged, while a bad name, a name that differs from the folder or a missing description is reported; the system prompt lists the enabled skills by name and a description capped at 240 characters, and the new `load_skill` tool returns a skill's body with its file list, or one of its files; scripts run through the shell tool and ask like any command; a Skills page turns each skill on or off everywhere or per project, and a profile's `skills:` limits its sessions to some. Slice 9 added import and install (SKL-003, SKL-005, the skills part of SET-003): the Skills page previews what `~/.claude/skills`, a chosen or pasted folder, or a dropped folder or `.zip` offers (each skill new, up to date, changed with a file-level diff, or unloadable with the reason, plus its scripts), then copies the chosen ones into the store with their source, date and a digest recorded in `.freeloop-source.yaml`; importing twice changes nothing, a changed source replaces the installed copy in one rename, the source is only read, and store skills can be removed. Importing Claude Code's permission rules and MCP servers (the rest of SET-003, MCP-002) comes with MCP. Slice 10 added MCP servers (MCP-001, MCP-003, MCP-005) on the official Go SDK (v1.8.0, approved 2026-10-29): stdio commands and Streamable HTTP endpoints in `mcp.yaml` with Keychain secrets passed as variables or headers; an MCP page adds, edits, starts, stops and deletes them and shows state, negotiated protocol, tools and the last error; a server runs only after the user approves its exact configuration (command, arguments, variables, URL, headers and secret names), any change asks again, and `npx`, `uvx` and unpinned `docker run` commands carry a warning; servers on for everyone, plus those a profile's `mcpServers:` adds, start with a session and their tools reach the built-in harness as `mcp__<server>__<tool>`, asking every time unless a rule such as `allow mcp__github__*` approves them, and never offered in plan mode. Slice 11 added the import from Claude Code (MCP-002, SET-003, the optional step of GUI-010): one page, also reachable from Settings, the MCP page and the first-run guide, previews the MCP servers in `~/.claude.json` (global and per project; project ones arrive off), the permission rules in `~/.claude/settings.json` and the skills in `~/.claude/skills`, each with its destination; variables and headers whose names say secret (token, key, password, authorization…) move to the Keychain; `Bash`, `Read`, `Edit`, `Write`, `Grep`, `Glob`, `LS` and `mcp__` rules convert, others (`WebFetch`, `ask` rules, the SSE transport) are listed as not imported with the reason; importing twice changes nothing, and imported servers still need approval before they run. Trusted projects' `.mcp.json` files are read too (slice 16). The 60 fps target for long transcripts was measured on a synthetic 5,001-event session: 226 ms to open and at most 1.7 ms of layout per scroll step in the built-in browser, without virtualization (OQ-022, closed: keep it unless the M1 check in the packaged app misses 60 fps). Slice 17 completed SEC-002's scopes: an environment file's `rules:` apply to its sessions at environment scope, so rules now come from global, environment, profile, project and session scope, deny winning at every one; the Rules and Info views list each with its scope and source file. Slice 18 added the YAML editor for profiles (GUI-009's "YAML editor with validation for the rest"): any profile, the built-in ones included, opens as its file, is checked as you type with the line and field of the first problem, and saves as written, comments kept, only when valid. Slice 19 checks each agent preset's command on the login PATH: the first-run guide lists the agents with "ready to start" or what to install (Node.js for `npx`), lets you continue without a provider when one is ready, and the new-session form will not start an agent whose command is missing. An accessibility pass (NFR-A11Y-001, NFR-A11Y-002) added a visible focus ring, ⌘1–⌘9 to open the sessions listed first and ⌘0 for the attention view, a hidden live region that announces approvals and turn ends instead of the whole streaming conversation, no smooth scrolling or transitions under Reduce Motion, and colours that pass WCAG AA in both themes (the light accent on the sidebar and white text on dark-theme buttons and badges did not); a scan of every page and form found no control without an accessible name. Not covered: the app does not follow the macOS system text size, only its own font size setting, and a VoiceOver pass is NFR-A11Y-003 (v1); their servers arrive off, and untrusted projects' files are not read. Slice 12 added external harnesses (HRN-004, HRN-005, HRN-006, HRN-008, MCP-004, SEC-008 for agents) on `github.com/coder/acp-go-sdk` v0.13.5 (approved 2026-10-29): presets in `presets/harnesses/*.yaml` plus a built-in Claude Code preset (the ACP adapter `@agentclientprotocol/claude-agent-acp@0.99.1` run with `npx`), each bringing a profile; the new-session form groups profiles by harness and leaves out provider and model for agents that bring their own; a session spawns its agent in the project with only the base environment and the environment's variables, initializes it, opens an ACP session with the session's approved MCP servers (HTTP ones only when the agent supports them), maps every `session/update` onto the event of the same name, sends permission requests through the broker under the built-in tool names (`shell`, `edit_file`, `read_file`), serves `fs/*` and `terminal/*` requests through the environment and asks the broker for any it was not just given permission for, and mirrors the app's mode into the agent's modes where the preset maps them. Verified against a reference agent in tests and in the browser; a real Claude Code run needs Node and a signed-in Claude Code, so it is part of the user's checks. After a relaunch an agent session starts fresh (SES-003 ACP load is next). Slice 13 added switching models mid-session (PRV-006): "Switch…" next to the session's model picks another provider and model for the built-in harness; the next model call uses it with the whole conversation, the transcript records a `model_changed` event shown as a line, and the session record follows. Slice 14 added the per-provider concurrency cap (OQ-013): at most `maxConcurrent` model calls (default 4) run on one provider across sessions; a session waiting for a slot is marked "Queued" in the sidebar and the conversation until one frees. Slice 15 added ACP session load (SES-003): the agent's session ID is kept with the session, and after a relaunch the agent is asked to `session/load` it when it advertises `loadSession`; the history it replays is not recorded again, since the transcript has it. An agent that cannot load starts a new session and the conversation shows a notice that it does not remember what came before.

Delivers:
- Harness: HRN-003 (profiles), HRN-004, HRN-005 (generic ACP client), HRN-006 (Claude Code harness), HRN-008.
- Providers: PRV-006.
- Skills: SKL-001, SKL-002, SKL-003, SKL-004, SKL-005.
- MCP: MCP-001, MCP-002, MCP-003, MCP-004, MCP-005.
- Sessions: SES-003 (ACP session load where supported).
- Permissions: SEC-002 (all scopes), SEC-006, SEC-008.
- GUI: GUI-002 (attention view), GUI-004 (slash commands), GUI-005 (diff and commit), GUI-008, GUI-009, GUI-010.
- Settings: SET-002, SET-003.
- Extensibility: EXT-001.
- Non-functional: NFR-PERF-002, NFR-PERF-003, NFR-SEC-001, NFR-SEC-002, NFR-SEC-003, NFR-XPLAT-001, NFR-XPLAT-002, NFR-A11Y-001, NFR-A11Y-002, NFR-REL-002, NFR-REL-003.

Definition of done:
- Every MVP-priority requirement passes its acceptance criteria, recorded in a checklist in the release PR.
- The Claude Code harness runs a subscription-authenticated session from the app with no token ever stored by the app.
- A skill imported from `~/.claude/skills` and an MCP server imported from `~/.claude.json` are both used by the built-in harness in one session.
- Five sessions prompting for approval at once are resolved from the attention view alone.
- Two weeks of daily use with no transcript loss and no session cross-talk.
- A tagged GitHub release with a DMG, README with install and first-run instructions, `docs/SECURITY.md`, and a filled-in `CONTRIBUTING.md` stating that external contributions open after v1 scoping.

Not in M2: Docker, worktrees, terminal, search, signing if the developer account is not yet set up (see OQ-009), presets for agents other than Claude Code.

## M3 v1 (16 weeks)

Goal: the release meant for other people. Environments beyond the local folder, the harness SDK, polish that makes it a daily tool for someone who is not the author, and the trust signals an open-source desktop app needs.

Delivers:
- Environments: ENV-005, ENV-006, ENV-007.
- Harness: HRN-007, HRN-011, HRN-012, HRN-013, HRN-014.
- Providers: PRV-008, PRV-009, PRV-010, PRV-011, PRV-012.
- Skills: SKL-006, SKL-007, SKL-008.
- MCP: MCP-006, MCP-007, MCP-008.
- Sessions: SES-007, SES-008, SES-009, SES-010, SES-011, SES-014.
- GUI: GUI-012, GUI-013, GUI-014, GUI-015.
- Permissions: SEC-011, SEC-012.
- Settings: SET-005.
- Extensibility: EXT-003, EXT-004, EXT-005.
- Non-functional: NFR-PERF-004, NFR-SEC-004, NFR-SEC-005, NFR-OFF-003, NFR-A11Y-003, NFR-REL-004.

Definition of done:
- The HRN-002 scenario runs inside a devcontainer with every shell call executing in the container.
- A new harness scaffolded from the Python template runs a session within 15 minutes following docs only.
- Gemini CLI, Codex CLI, Goose and OpenCode each pass the HRN-005 scenario from their presets.
- A VoiceOver pass with no blocking issues.
- At least five people other than the author have used it for a week and their blocking issues are fixed.

### Feature order (planned 2026-10-29)

With M2's buildable work done, the v1 features come in this order, chosen for what most limits running many sessions on your own models; each is a slice with its own commit, tests and docs.

1. **Worktree per session (SES-010, OQ-008).** *Built 2026-10-29.* A session can get its own git worktree and branch, so two sessions on one repository never edit the same checkout; its Diff panel shows its own changes; merge back into the main checkout from the panel; the worktree is removed on archive when it holds no uncommitted work. The new-session form asks when another session already works in the project and remembers the answer per project.
2. **Context compaction (HRN-013).** *Built 2026-10-29.* A context meter; at 80 % of the model's window the built-in harness summarises older turns into one message, records the compaction in the transcript and continues; a manual compact command.
3. **Docker and devcontainer environment (ENV-005, ENV-006, ENV-007, OQ-014).** *Built 2026-10-29, tested against Colima.* An environment kind that runs every command, file access and external agent inside a container built from `.devcontainer/devcontainer.json` or an image, the repository bind-mounted, with setup and health checks.
4. **Checkpoints and rewind (SES-014).** *Built 2026-10-29.* A snapshot of the working tree before each turn that changes files; "rewind to here" on a user message restores the files and the context.
5. **Search across sessions (SES-007).** *Built 2026-10-29.* An SQLite full-text index over transcripts, searched from the sidebar.
6. **Cost estimates (PRV-008, OQ-023).** *Built 2026-10-30.* A price table per model, cost per turn, session and day; since 2026-11-03 with the vendors' list prices built in.
7. **Command palette (GUI-014).** *Built 2026-10-30.* ⌘K for sessions, profiles, pages, actions and slash commands.
8. **Hooks (EXT-005).** *Built 2026-10-30.* Scripts on session start, before and after tool calls and on permission requests, JSON on stdin, an exit-code contract that can block a call.
9. **MCP resources and prompts (MCP-007).** *Built 2026-10-30.* Attach a server's resource to a message; run its prompts as slash commands with a form for their arguments.

Review follow-ups (2026-10-30). A review of the nine features above found three security problems and a dozen bugs, fixed in the commits after it; the five smaller findings left open then (the startup index rebuild holding up live sessions, `session_start` running twice on a concurrent start, project hooks missing from the trust prompt, a stopped command running on in its container, git in a Docker worktree session) were fixed on 2026-10-30 as well.

### Feature order, part 2 (planned 2026-10-30)

The next v1 items that need nothing from the user (no accounts, signing, real-provider runs or new dependencies), in order:

1. **Export (SES-008).** *Built 2026-10-30.* A session as Markdown (messages, tool calls with input and output, edits as diffs) or as JSON with every event, saved where the user chooses.
2. **Fork from a message (SES-009).** *Built 2026-10-30.* A new session with the history up to a message and a transcript of its own.
3. **Retry and fallback (PRV-010).** *Built 2026-10-30.* Backoff on 429 and 5xx, shown as a notice; an optional fallback model per profile.
4. **Switch harness or profile mid-session (HRN-012).** *Built 2026-10-30.* A new session seeded with a handoff summary, in the same environment.
5. **Config backup (SET-005).** *Built 2026-10-30.* Export and restore providers, profiles, environments, rules and settings without secrets.
6. **Parallel read-only tool calls (HRN-014, first half).** *Built 2026-10-30.* Independent reads in one model turn run concurrently.
7. **Local-only switch (NFR-OFF-003).** *Built 2026-10-30.* Block every non-loopback connection the app would make and report attempts.
8. **Contributor documentation (NFR-REL-004).** *Built 2026-10-30, as [DEVELOPING.md](DEVELOPING.md).* Architecture overview, adding a provider, a harness preset, an environment.

Left for later because they need the user: signed builds and the update check (SEC-012, NFR-SEC-004, OQ-009, OQ-015), verified external-agent presets and OAuth against real servers (HRN-007, MCP-006), prompt caching checked against a real provider (PRV-009), and the integrated terminal, which needs a PTY library (GUI-012).

### Feature order, part 3 (planned 2026-10-30)

1. **Reasoning controls (PRV-011).** *Built 2026-10-30.* The profile's effort reaches Anthropic (adaptive thinking with an effort), OpenAI's reasoning models (`reasoning_effort`) and OpenRouter (`reasoning.effort`); thinking is shown collapsed.
2. **Prompt caching (PRV-009).** *Built; the request payload is tested here, a run against the real API is the user's.* Cache breakpoints for the system prompt and tool definitions on Anthropic; checked against the request payload here, against the real API by the user.
3. **Skill authoring (SKL-007).** *Built 2026-10-30.* Create a skill from a template, validate its frontmatter, open it in the editor.
4. **File tree and viewer (GUI-013).** *Built 2026-10-30.* A read-only browser of the session's folder with the agent's changes highlighted.
5. **Subagents (HRN-014, second half).** *Built 2026-10-30.* The built-in agent hands a task to a child agent whose transcript the parent can open.

### Feature order, part 4 (planned 2026-10-30)

1. **Skills from git (SKL-006).** *Built 2026-10-30.* Install from a repository URL pinned to a commit, check for updates with a diff, update and roll back.
2. **Harness templates (EXT-003, HRN-011).** *Built 2026-10-30.* Python and Go ACP agent templates with a test harness, and "New harness" registering one as a preset.
3. **Headless control (EXT-004).** *Built 2026-10-30.* A JSON-RPC API on a Unix socket and a small CLI.
4. **MCP input-required round trips (MCP-008).** *Built 2026-10-30.* A server's request for more input shown as a form, the call retried with the answers.

### Feature order, part 5 (planned 2026-10-30)

1. **Pull requests (SES-011).** *Built 2026-10-30.* The Diff tab pushes the session's branch and opens a pull request with the GitHub CLI, its description drafted by the model and edited by the user; the URL is recorded in the session.
2. **Multiple windows (GUI-015).** *Built 2026-10-30; checked in the browser as two tabs, the native windows by the user.* A session opens in a window of its own, streaming alongside the main window.
3. **Idle memory (NFR-PERF-004).** *Built 2026-10-30; measured 110 MB on this Mac (the app process 38 MB, its three WebKit processes 72 MB); the M1 with 16 GB the requirement names is the user's run.* A repeatable measurement of the app with ten idle sessions against the 300 MB budget.

### Feature order, part 6 (planned 2026-10-30)

The user approved the dependencies these need on 2026-10-30.

1. **Native Gemini and OpenAI Responses providers (PRV-012).** *Built 2026-10-30; tested against recorded stream shapes, real runs are the user's.* Adapters on the official OpenAI and Google Gen AI SDKs.
2. **Integrated terminal (GUI-012).** *Built 2026-10-30; the Docker acceptance passes in a test against Colima.* A PTY per session in the session's environment, Docker included, shown with xterm.js.
3. **OAuth for remote MCP servers (MCP-006).** *Built 2026-10-30; tested against a fake authorization server, a real server is the user's check; Client ID Metadata Documents wait on OQ-024.* The authorization code flow with PKCE through the MCP SDK's auth package, tokens in the Keychain.
4. **Skills for external agents (SKL-008, OQ-006).** *Built 2026-10-30; checked with the reference agent, Claude Code itself is the user's check.* The session's enabled skills reach Claude Code as a per-session plugin folder in the app's data folder.
5. **Update check (OQ-015, SEC-009).** *Built 2026-10-30.* An opt-in daily check of the project's GitHub releases that says when a newer version exists; the cask and in-app updates wait on signed builds.

### Feature order, part 7 (planned 2026-10-31)

1. **Workspace (GUI-017).** *Groundwork built 2026-10-31; the panes built 2026-11-02 (`Workspace.tsx`). Checked in the browser: four sessions in four projects at once, an approval key acting only in the active pane, the layout after a restart, tabs in a narrow window, ⌘-click, a drop, closing a pane and deleting a session in one. A real mouse drag in the native window was not tried.* Up to four sessions side by side in the main window.
2. **A safer Stop button.** *Built 2026-10-31.* Stop sits at the far end of the composer from Send and Queue, and the transcript says who stopped a turn (you, quitting the app, archiving, or the parent of a subagent), since an agent reports every stop as the same cancel.
3. **Tags and saved filters (SES-013).** *Built 2026-10-31.* Tags set by a `tags_changed` event (so a fork keeps them) and indexed in SQLite, edited under the session title, shown in the sidebar and filtered by a menu or `#tag` in the search box; named filters are kept under `filters` in `config.yaml` (`frontend/src/filters.ts`, `Tags.tsx`, `internal/engine/tags.go`). The sidebar also no longer lets an older session list overwrite newer state.
4. **Board view (GUI-016).** *Built 2026-10-31.* One card per session in columns for needs you, running and idle, from the sidebar's "Board" link or the palette, showing the sessions the sidebar's filters let through (`frontend/src/board.ts`, `Board.tsx`).

### Feature order, part 8 (planned 2026-10-31)

1. **Scheduled and recurring sessions (SES-012).** *Built 2026-10-31.* Settings → Schedules adds a schedule (project, prompt, profile, provider, model, mode other than bypass; hourly, daily, weekdays or weekly at a local time). A loop in the engine checks every thirty seconds and starts a due run as a new session tagged `scheduled`, through the broker; runs missed while the app was closed make one run at the next start, and "Run now" runs a schedule at once (`internal/config/schedules.go`, `internal/engine/schedules.go`, `Schedules.tsx`). The app must be open: there is no background agent.
2. **Hardening review.** *Done 2026-10-31.* Three reviews (engine lifecycle, security boundaries, malformed input) and their fixes: writes through dangling symlinks, ACP approvals reused across kinds, zsh expansion in prefix allow rules, case-sensitive path rules and searches past deny rules, tool-wide "Allow always" rules; provider stream panics, Markdown that froze or blanked the window, config typos that turned off local-only mode and hooks; turns orphaned by an archive or delete, a shared WaitGroup reused while waited on, and shutdown that a starting schedule held up. The three lesser findings were fixed the same day: a damaged line in the middle of a transcript is repaired on open (readable events keep their seq, each lost one becomes a notice, the original is kept as `events.jsonl.damaged-<time>`), indexed text is cut at a character boundary, and a config.yaml that cannot be read at startup starts with `.config-last-good.yaml`, the last version that could be.
3. **Usage dashboard (GUI-018).** *Built 2026-10-31.* The sidebar's "Usage" link (and the footer's cost of the day) opens a dashboard: a range of 7 days to 12 months, grouped by kind of token (the default: input, output, cache write and cache read, each with its share of tokens and of cost), model, provider or agent, or project, measured in tokens or estimated cost, with a switch that leaves out cache reads, which can dwarf the rest; a headline figure with the change on the period before, tiles for input, output, the share read from the prompt cache, tokens per model call and per session, stacked columns per day (per week past 90 days) with a readout per column and a table view, a breakdown table and the sessions that used the most, and a CSV export through a save dialog. The engine's `UsageReport` joins the usage table with the sessions, prices each row and keeps a deleted session's tokens; `frontend/src/analytics.ts` groups the rows and `UsageChart.tsx` draws the columns in SVG, with series colours checked for colour-blind separation in both themes. No new dependency.

### Feature order, part 9 (planned 2026-10-31)

From a comparison with the Claude Code and Codex desktop apps, Cursor's Agents Window, Zed and the parallel-agent apps (Conductor, Emdash, Orca, Nimbalyst): the three gaps that cost the most in daily use. No new dependency.

1. **Images in prompts (SES-015).** *Built 2026-10-31; checked in the browser with the demo provider and in tests against every adapter's request shape and the reference ACP agent; real models are the user's check. As built, the built-in harness always sends pictures (Anthropic, Responses and Gemini declare `ModelCaps.Images`; the capability file has no image override), and a model that rejects them fails the turn with the provider's reason.*
   - *Composer:* paste or drop images (PNG, JPEG, GIF, WebP), thumbnails above the text with a remove button, at most ten per message. The page scales each down in a canvas to 2000 pixels on the longer side and under 3.75 MB (within every provider's limit), so the engine never decodes images.
   - *Storage:* `Engine.PromptWith` takes the images; the store writes each once to `sessions/<id>/attachments/<sha256>.<ext>` and the user's `user_message_chunk` carries an `image` content block naming it (ACP's image block, with the file instead of inline data, so transcripts stay small). Fork copies the attachments it needs; delete already removes the folder; the Markdown export links the files, the JSON export names them.
   - *Built-in harness:* a new `provider.PartImage` (media type and attachment path) is read at request time and sent as an Anthropic image block, a Responses `input_image`, a Gemini inline part or an OpenAI-compatible `image_url` data URL. A new `ModelCaps.Images` comes from the capability table (static per provider, overridable in `capabilities.yaml`); a model known not to take images is refused before sending, an unknown one is tried and a refusal explained.
   - *ACP:* sent as image content blocks when the agent's `promptCapabilities.image` is set, refused with a clear message when not.
   - *Transcript:* the bubble shows thumbnails, loaded through `Engine.Attachment(sid, name)`; a click opens the image full size. Only the user's own attachments are shown as images; model output still never reaches the page as markup or images.
2. **Worktree setup and run scripts (SES-016).** *Built 2026-10-31; checked in the browser on a scratch project (copy, setup, ports, Run serving on the session's port, a failed setup with Start anyway) and in `TestWorktreeScripts`. As built, the setup state lives in the app's run only: after a restart a session no longer waits for a setup that was interrupted, and the archive command runs on archive, not on delete. In a Docker environment the setup and archive commands run in the session's container (OQ-026; `TestDockerWorktreeScripts`, checked on a real daemon).*
   - *Configuration:* a trusted project's `.freeloop/worktree.yaml` (`copy:` paths from the main checkout, such as `.env`; `setup:`, `run:` and `archive:` commands), and the same fields per project in the app, for repositories you cannot commit to; the app's settings win field by field. The trust prompt lists the commands, as it lists hooks (SEC-006); an untrusted project runs nothing.
   - *Setup:* after `addWorktree`, the engine copies the files and runs `setup` once through the session's environment (ENV-001) with `FREELOOP_ROOT_PATH` (the main checkout), `FREELOOP_WORKTREE` and `FREELOOP_PORT` set. Progress and the output's tail are `env_health` events, so the session shows "Setting up…" and a failure with its output; the first prompt waits in the queue until setup ends, and a failed setup offers "Retry" or "Start anyway".
   - *Ports:* each worktree session gets a block of ten ports from 41000, the lowest block no live session holds, kept on the session row and exported as `FREELOOP_PORT` to the agent's commands, the terminal and the scripts.
   - *Run:* a "Run" button in the session header opens a terminal tab running `run` (GUI-012's terminal with an argv), shown as running until it exits; "Stop" ends it.
   - *Archive:* `archive` runs before the worktree is removed, with a time limit; its failure is a notice, never a block.
3. **Review comments on the diff (GUI-019).** *Built 2026-10-31; checked in the browser with the demo provider.*
   - *Diff tab:* hovering a line shows a "+" in its number column; a click opens a comment box under the line (⌘↩ adds it). Comments show inline and in a "Review" bar at the top of the tab with the count, "Send review" and "Discard"; they are kept per session in the page until sent, and shift with the diff's line numbers by matching their line's text.
   - *Message:* sending puts one message in the composer, for the user to edit before sending, or sends it directly with ⌘↩: each comment as `path:line`, the changed line and two lines around it in a fenced block, then the comment. The message is plain text, so the built-in harness and every ACP agent read it the same way (HRN-001).
   - *Tests:* the message format and the line matching are pure functions in `frontend/src/review.ts` with `node --test` tests.

Order: 3, then 1, then 2. Review comments are the smallest and touch only the frontend; images touch every provider adapter and the transcript; setup scripts add a config file, a trust surface and a session state, and should come last, once the other two have settled.

### Feature order, part 10 (planned 2026-11-01)

The second tier of the comparison with other agent apps; each builds on the GitHub CLI or worktrees the app already uses. No new dependency.

1. **Start from a GitHub issue (SES-017).** *Built 2026-11-01; tested against a fake gh and checked in the browser with one.* `Engine.Issues(root, search)` runs `gh issue list --json` in a trusted project's main checkout (no git hooks, local-only refuses); `Engine.IssueBrief(root, number)` reads one issue with its comments through `gh issue view --json` and writes the first message: the issue's title, link, body and comments, capped at 20 KB. The new-session form gets "From an issue…": a searchable list; choosing one sets the title to "#12 Title", tags the session `issue-12`, and opens it with the brief in the message box. The issue's text is the user's to read before sending: it is put in the box, never sent.
2. **Pull request checks (SES-018).** *Built 2026-11-01; tested against a fake gh. Development builds take a gh stand-in from `FREELOOP_DEV_GH`.* `Engine.PullRequestChecks(sid)` runs `gh pr checks <url> --json name,state,bucket,link,workflow` for the session's last pull request; the Diff tab's pull request section lists the checks with their state and refreshes every 30 seconds while any is pending and the tab is open. `Engine.CheckFailures(sid)` reads the end of each failed GitHub Actions job's log (`gh run view --job <id> --log-failed`, 3 KB each) and writes one message naming each failure with its link and log; "Send failures to the agent" puts it in the message box.
3. **One task on several profiles, compared (HRN-016).** *Built 2026-11-01; tested with two scripted providers and checked in the browser with two demo profiles. As built, the compare page opens after the runs start, from a session's "Compare with the other runs" link, or from the palette; it reads the runs again every few seconds.* The new-session form's "Also run on…" picks one to three more profiles; `Engine.CreateCompare` makes one worktree session per profile (a trusted git project is required), tags them `compare-<id>`, titles them "<title> · <profile>" and sends each the same first message. A compare page, from the sidebar group's "Compare" link or the palette, shows the runs side by side: status, files changed, lines added and removed against the shared base (`git diff --numstat`), tokens, estimated cost and the last reply; each run can be opened, and "Keep this one" archives the others (their worktrees go, their branches stay).

Order: 2, 1, 3. Checks extend the pull request section that exists; the issue picker adds a form control; the compare runs touch session creation and add a page.

### Feature order, part 11 (planned 2026-11-01)

The rest of the comparison with other agent apps, apart from the side-by-side panes. No new dependency.

1. **Web and to-do tools (HRN-017, OQ-025).** *Built 2026-11-01; tested with local pages, a local search endpoint and scripted calls; a real page and a real search endpoint are the user's check.* `web_fetch` and `web_search` go through a new `harness.WebSource` the engine implements (`internal/engine/web.go`, with HTML turned to text in `internal/web`): the fetch client refuses private addresses at every redirect and after dialling, reads at most 5 MB and returns at most 50 KB of text. Both have kind `fetch`, so the broker asks every time and plan mode refuses them; the URL or query is the call's rule target, so "Allow always" covers that exact one and never the whole tool. `web_search` is offered only with `web.searchURL` set in Settings, since the app has no search engine of its own to send queries to (OQ-025). `todo_write` emits ACP's `plan` event; the view shows the latest plan from any harness above the message box.
2. **Review by a second agent (HRN-018).** *Built 2026-11-01; tested with a scripted reviewer and checked in the browser with the demo provider.* The Diff tab's "Ask for a review" picks a profile; `Engine.RequestReview` makes a session in plan mode, a child of the first in the same worktree, tagged `review`, and sends it the diff against the base commit. `Engine.Reviews` lists a session's reviews with their status and reply; "Use these findings" puts the reply in the first session's message box.
3. **App preview (GUI-020).** *Built 2026-11-01; checked in the browser, and in the packaged app's WebKit with a hostile page in the frame, which is what showed the desktop build needed its own guard on engine calls (see SECURITY.md).* A Preview tab with a sandboxed frame on a `localhost` address, by default the session's `FREELOOP_PORT`, with Reload and Open in browser; `Engine.PortOpen` says when nothing is listening. Rules for the address are pure functions in `frontend/src/preview.ts`.

### Feature order, part 12 (planned 2026-11-01)

What the M3 list still holds and needs no account of the user's.

1. **Verified presets for other agents (HRN-007).** *Built 2026-11-01. `make verify-agents` runs each real agent at its pinned version against a local stand-in for its model: all four ask through the broker, write when allowed, not when denied, and not in plan mode. A real model and a real key are the user's check, as is signing in to Codex with a ChatGPT plan.* Gemini CLI 0.66.0, Codex through `@agentclientprotocol/codex-acp` 2.5.0, goose 1.57.0 and OpenCode 1.19.5. Each maps every mode but plan to the agent's mode that asks, so actions reach the broker. The runs found what reading the docs could not:
   - goose and Codex default to modes that approve by themselves, and OpenCode needs `OPENCODE_PERMISSION` to ask at all.
   - A read of a missing file returned a generic error where ACP has "not found" (fixed for every agent), and Gemini CLI still cannot recognize that one, so its preset answers with no content.
   - Gemini CLI sends `[MODE_UPDATE]` as agent text, which the preset hides.
   - An agent that switches itself to a mode the preset does not name is told the session's mode again.
2. **What a release can be checked against (SEC-012, NFR-SEC-004, the parts needing no account).** *Built 2026-11-01. `make verify-repro` on 6dfb484: two cold builds from clean clones at different paths, with separate Go caches, matched in all 6 compared files (the five of the bundle, signature included, and the SBOM). Both ran on one Mac; a build on another with the same tools is the user's check, and the signed, notarized half waits on OQ-009.*
   - *SBOM:* `make release` writes `bin/release` with the DMG, a CycloneDX 1.6 bill of materials (`internal/sbom`, `cmd/freeloop-sbom`, from the binary's own build information and the frontend's lockfile), `BUILDINFO.txt` and checksums. No new dependency.
   - *Pins:* an exact `toolchain` in `go.mod`, CI's actions pinned to commits, and `internal/archtest/pins_test.go` holding those, npm's exact versions and checksums, and a budget of four shipped npm packages. `make verify-deps` checks modules against `go.sum` in CI.
   - *The build:* no longer runs `go mod tidy`, which could rewrite what it builds from; it fails when the module needs tidying.
3. **Accessibility audit (NFR-A11Y-001, NFR-A11Y-002, the audit half of NFR-A11Y-003).** *Done 2026-11-01. Every screen was audited in both themes in the built-in browser with a script (names, roles, contrast, Tab order, visible focus) and walked with real key presses; the VoiceOver pass itself is the user's, with the checklist in DEVELOPING.md.* Control names were already sound. What it found and fixed:
   - *Moving between screens was silent:* the window's title now names the screen, the move is announced, and focus goes to the new screen; a session opened from its row keeps focus there. A skip link leads past the sidebar.
   - *Status by colour alone:* a tool card's dot now has words. Messages have a hidden "You" or "Agent" heading to jump by, and rewound ones say so.
   - *Contrast:* the stylesheet claimed AA, and missed it for faint text, a warning colour, a dark hover and the dark focus ring, as well as content dimmed with opacity. `a11y.test.ts` now computes every pair of tokens.
   - *Keyboard:* the panel's tabs are one stop with arrow keys, and arrows only move focus, because opening Terminal starts a shell; the usage chart is one stop instead of one per column. Shift+Esc leaves the terminal, which keeps Tab for the shell. Tab stays inside the palette and the picture view, which now takes focus so Escape works.
   - *Screen readers:* the terminal's output is exposed (xterm's `screenReaderMode`), a picker's highlighted entry is announced, and repeated announcements are heard again.
   - *Text size:* all 72 sizes fixed in pixels now follow the Font size setting, and ⌘+ and ⌘- change it from anywhere (OQ-027). The app has its own menu without the View menu's zoom items, which claimed those keys and ⌘0.
4. **Sandboxed execution (SEC-011, NFR-SEC-005).** *Met by the Docker environment (ENV-005), as their text says; the native sandbox stays ENV-009.*

### Feature order, part 13 (planned 2026-11-01): the native sandbox (ENV-009)

*Built 2026-11-01 with OQ-028's recommended defaults. The containment tests (`internal/platform/darwin/seatbelt_test.go`) and an end-to-end session (`internal/engine/sandbox_test.go`) run against the real Seatbelt; real projects' toolchains beyond this repository's are the user's check. As built, a built-in "Local, sandboxed" environment offers it in the new-session form; the file tools still ask as their mode says, since the sandbox holds shell commands only. Beyond the plan, an environment-specific deny rule on a path also closes it to the shell, other programs can no longer be signalled or have their command lines read, and files other tools act on later (`.mcp.json`, `.claude`, `.envrc`, `.vscode`, devcontainer files) are read-only.*

An approved shell command can today read or write anywhere the user can, and Docker is too heavy to use for every session. A sandbox closes that hole, and also removes most approvals: a command that cannot leave the project does not need asking about. No new dependency.

**Tried first, on macOS 26.7 with a draft profile (`sandbox-exec`, Seatbelt):**

| | Result in the sandbox |
|---|---|
| Works | Git status and commit, `zsh -l`, Python, `cc`, Node, Go tests, the engine's full test suite (with a pty allowed), the frontend's tests and Vite build |
| Refused | Writes outside the project, `.git/config`, `.git/hooks`, `.git/commondir`, `.freeloop/`; a hard link or symlink out; reading `~/.ssh`, `~/Documents`, `~/.zsh_history`; the internet, DNS, the SSH agent |
| Needed more than file and network rules | With `(allow default)` alone, `open` launched an app **outside the sandbox**, and the clipboard stayed readable. `(deny mach-lookup)` closed `open`, `pbpaste`, AppleScript and `launchctl submit`. Reading a Keychain item was refused too |
| Fails | Nested `sandbox-exec` exits 71, so a tool or agent using Seatbelt itself cannot run inside. `swift build` needs `--disable-sandbox` and its cache paths given: `swift build --disable-sandbox --cache-path "$XDG_CACHE_HOME/sw" --config-path "$XDG_CACHE_HOME/swc" --security-path "$XDG_CACHE_HOME/sws"` works |
| Cost | About 8 ms per command; a cold Go cache on first use |

Apple marks `sandbox-exec` deprecated with no replacement for this use. It still works, and other agent tools rely on it. So it fails closed (below), and the profile stays in one file.

**Rules the design must keep:**

- *Fail closed.* If `sandbox-exec` is missing or will not apply the profile, a sandboxed environment refuses to run commands and says so. It never falls back to running them unconfined. A self-test at session start also checks that a write outside the project is refused.
- *Auto-running only what is really contained.* An ACP agent asks permission and then runs the command in its own process, outside the sandbox. So a request only skips approval when its caller guarantees containment (`Request.Contained`, set by the built-in `shell` tool and ACP's `terminal/*`, never by `session/request_permission`). Otherwise the sandbox would approve commands it does not contain.
- *No shared caches.* A shared Go build cache is a way out: a sandboxed command could poison what the user's own builds later use. `GOCACHE`, `XDG_CACHE_HOME`, `npm_config_cache`, `PIP_CACHE_DIR` and `TMPDIR` point to a per-project folder under `~/Library/Caches/Freeloop/sandbox/`, used only inside the sandbox.
- *Nothing run outside may execute what the sandbox wrote.* As with Docker (OQ-026), setup and archive commands and hooks run in the sandbox, with network allowed for setup.
- *Deny rules still win*, and `.freeloop/` can only tighten what the sandbox allows.

**Slices, each landing on its own:**

1. **The profile and its proof.** `internal/platform/darwin/seatbelt.go` writes the profile from typed options, keeping macOS code in its one package (NFR-XPLAT-002); `environment.LocalOptions` takes a `Confine` function that wraps a command's argv. Absolute `/usr/bin/sandbox-exec`, never from PATH. `TestSeatbelt*` turns the table above into a containment test, which skips off macOS. The profile uses the policy's never-readable list, the app's data folder included; home is unreadable except the project, shell startup files and an allow-list of toolchains (`~/go`, `~/.nvm`, `~/.cargo`, `~/.rustup`, `~/.pyenv`, `~/.local/bin`, `~/Library/Developer`, git's config).
2. **A setting on the environment.** `sandbox: {enabled, network, read, write}` on a local environment (`config.Environment`, `Environments.tsx`). The runtime's `r.env` is confined and `r.host`, used for the app's own guarded git, is not. A worktree also gets its main checkout's `.git` writable, except config and hooks, as `gitMounts` does for Docker. Caches and temp as above; health shown like Docker's.
3. **Fewer approvals.** The broker allows a `Contained` command in `ask` and `accept_edits`, recorded as decided by the sandbox, after deny rules and hooks. The `shell` tool gains `outside_sandbox` with a reason, which always asks and is never covered by a rule. A failed command that looks like a sandbox refusal gets a note for the model saying how to ask. The tool card says Sandboxed, and an approval to leave it says so plainly.
4. **Plan mode that can look.** A read-only profile (no project writes) lets `shell` run in plan mode, so a review can run `git log` or the tests' listing instead of being refused.
5. **Docs and the known gaps.** SECURITY.md, including what it does not cover (below).

**Not in this plan:**

- *An external agent's own tools.* Only its `terminal/*` requests are contained. Wrapping the whole agent would need a test per agent, and fails for one that uses Seatbelt itself. SECURITY.md will say so, and the Docker environment stays the answer for those.
- *MCP servers and the integrated terminal*, which stay unconfined as today.
- *Per-host network rules*, which are ENV-010.
- *What the project's own files do later:* a sandboxed command can still edit a Makefile that you run yourself, as in Docker.

OQ-028 records the defaults.

### Feature order, part 14 (built 2026-11-01)

What the open "later" list held that follows from the sandbox or needs no account. No new dependency.

1. **Listed hosts for the sandbox (ENV-010).** *Tested on the real sandbox and internet with curl, npm, Python, git, Node and Go, gated by `FREELOOP_SANDBOX_NET=1`.* A local proxy per session (`netguard.Egress`) forwards to the hosts an environment lists and refuses the rest. The sandbox still lets commands reach only this Mac, so a tool that ignores the proxy gets no connection at all. TLS is not opened, a password keeps other programs off it, names resolving to private addresses are refused, and it obeys local-only. A refused host is a notice saying how to add it. Package managers get a project's own cache, since fetching writes where the user's builds read; Go still reads the user's modules first. It covers the local sandbox only, not Docker.
2. **Browsing an MCP registry (MCP-009).** *Tested against a stand-in and in the browser against the official registry.* Nothing connects until the user chooses an address on that page, which keeps to the rule that outbound traffic goes to endpoints the user set. A result only fills in the add-server form, pinned to its version, and the server starts after the usual confirmation of its exact command (MCP-005). Only npm and PyPI packages and https Streamable HTTP remotes can be added; the rest are named. Identifiers and versions that are not plain are refused, and secrets go to the Keychain.
3. **Shortcuts (SET-006).** *Checked in the browser with real key presses.* Settings changes the palette, Settings, Needs attention, text size and the approval letters; `ui.shortcuts` in `config.yaml` holds what changed. macOS's and the app's fixed keys are refused, and so is a key already in use.

### Feature order, part 15 (built 2026-11-01)

1. **Deeper analytics (GUI-021).** *Built 2026-11-01; checked in the browser in both themes, the rule action and the counting against real demo transcripts, the charts on seeded scratch history, since no real history of weeks exists here.* The usage page gets the tabs Tokens and cost, Activity, Approvals and Efficiency, and filters for project, provider or agent, model and tag across all of them. The engine folds each session's transcript into counts per hour (`internal/engine/activity.go`, the `activity` table) when the page asks, a session at a time, only for sessions that moved on, so a running turn pays nothing and past sessions count from the first look. The insights are pure (`frontend/src/activity.ts`). Not in: an activity CSV (Export CSV still covers tokens and cost), and turns per model are shared out by model calls rather than recorded, since the transcript does not say which model a turn used.

### Custom colors (built 2026-11-02)

1. **Custom colors (GUI-022).** *Checked in the browser: both directions between the editor and `colors.json`, a bad value, broken JSON, a restart, both appearances and Reset. The color picker's panel and "Open colors.json" need the native window, so they were not tried.* Settings → Appearance changes the app's colors for each appearance, live, and `colors.json` in the data folder holds the same set.

### What a session runs on (built 2026-11-05)

1. **Provider marks (GUI-034).** `frontend/src/source.ts`, `SourceMark.tsx`, `SourceMarks.tsx`, `ui.sourceMarks` in `config.yaml`. *`make check-app` sees them on the sidebar and panes with the demo provider and a stored choice; a colour-blindness simulation of the slots was not run, and the letters are what covers that.*
2. **Markdown and PDFs in the Files tab (GUI-035, OQ-035).** `frontend/src/FilesPanel.tsx`, `PdfView.tsx`, `pdfview.ts`, `ViewPDF` in `internal/engine/files.go`. *`make check-app` opens a hostile PDF on the served build: its script does not run, a click on its link leaves it in place, and that step fails with the viewer's policy loosened. Not yet seen on the packaged app's own scheme, on macOS 14 or 15, or with a large file.*
3. **Context at a glance (GUI-036).** A meter on each pane, the sidebar from 80 %, a Compact button and a note when a call sends the whole context again. `frontend/src/contextMeter.ts`, `noteContext` and `reportCall` in `internal/harness/acp/calls.go`. *`make check-app` fills a real session of the demo provider and compacts it. External agents are tested with the reference agent's messages, shaped as the Claude adapter 0.99.1 sends them; a real Claude Code session has not been watched. A session not touched since the app opened has no figure in the sidebar. A custom endpoint's model gets its limit from the user (`ContextLimit.tsx`, `SetContextLimit`).*
4. **Usage views in a pane (GUI-038).** `frontend/src/UsagePane.tsx`, `PaneRange.tsx`, `TokenFigures.tsx`, `usagePane.ts`. *`make check-app` steps through every view in a four-pane layout and compares the pane's tokens with the usage page's. How it looks with real data is for the user to see.*
5. **Reviewing a plan (GUI-043, OQ-036).** `frontend/src/PlanReview.tsx`, `planView.ts`, the Plan tab in `SessionPanel.tsx`; `agentMode` in `internal/harness/acp/acp.go`, `forwardPlanFeedback` in `internal/engine/runtime.go`. *`make check-app` reviews a demo plan end to end; an engine test runs an external agent's plan with the reference agent. A real Claude Code plan has not been reviewed in the app.*
6. **An agent asks the user (HRN-021).** `UnstableCreateElicitation` in `internal/harness/acp/acp.go`, `ask_user` in `internal/harness/builtin/tools.go`, `internal/harness/questions.go`, the choices in `frontend/src/McpForm.tsx` and `forms.ts`. *An engine test with the reference agent and `make check-app` with the demo provider. Claude Code's own AskUserQuestion has not been answered in the app yet.*

### A calmer interface (built 2026-11-06)

1. **Less on screen (GUI-042).** Quieter turn ends, a mark for allowed tool calls, one-line mode banners, no running command on sidebar rows, icons in the usage pane's head. Then the usage pane's Recent hours: one row of controls with the rest behind a filter button, limits as lines, short times, no box in the box (`LiveUsage.tsx`, `Limits.tsx`, `useDismiss.ts`). `SessionView.tsx`, `ToolView.tsx`, `Workspace.tsx`, `App.tsx`.
2. **Icons where the action is obvious (GUI-041).** One icon module (`frontend/src/icons.tsx`, the Settings icons moved in), icon-only header, footer, composer and queue controls, each named and with a tooltip. *`make check-app` checks every visible control has a name and every icon-only one a tooltip. Not yet looked at by the user in light and dark, or with VoiceOver.*
3. **Settings on short pages (SET-007).** General split into Appearance, Keyboard, Sessions, Costs, Permissions, Network and Data (`Settings.tsx` with a `page`, `settings.ts`, `SettingsNav.tsx` with counts). *Not yet looked at by the user.*

### Seeing what sessions do, and checking the running app (built 2026-11-04)

1. **What each session is doing (GUI-030).** A line on Board cards, and in the tooltips of sidebar rows and pane tabs (the line on the sidebar row itself went with GUI-042) (`frontend/src/sessionActivity.ts`, `ActivityLine.tsx`). *Unit-tested; `make check-app` sees it in the sidebar row's tooltip and on the Board with the demo provider. A real agent has not been watched. A session not touched since the app opened has no line.*
2. **The app check (`make check-app`).** See DEVELOPING.md. *Shown to fail with the scrolling bug of 2026-11-04 put back. The native dialog stays outside it.*

### A quiet window and grouping by date (built 2026-11-03)

1. **A quiet window (GUI-026).** *The user looked at the built app and found it much better. Unit tests cover which tool calls collapse, which `cd` is dropped and a collapsed project's summary; narrow panes, the permissions menu's Rules item and VoiceOver were not gone through.* The Approvals tab stays, by the user's choice.
2. **Grouping by last used (SES-021).** *Unit tests cover the day boundaries and the order; not looked at in the app when written down.*
3. **Supervising many sessions** (SES-022, GUI-027, GUI-002, GUI-006, GUI-016, GUI-017): a mark on sessions that finished out of sight, in `frontend/src/unread.ts`; the attention view answers by key, J and K move between requests oldest first, with waiting time, a note, the scope of "always" and the edit as a diff (`frontend/src/attentionview.ts`); the board puts the longest wait first and answers on the card; a pane tab and an empty pane show status; errors name their session and do not replace each other (`frontend/src/errorlist.ts`); quitting asks while work would stop (`cmd/freeloop/quit.go`, `Engine.CheckQuit`). Logic is unit-tested; the look and the native dialog have not been checked in the running app. *From the review of 2026-11-03.*
4. **Reliability after the same review** (SES-003, NFR-REL-002): an agent that cannot load its session gets a recap in its next prompt (`internal/engine/recap.go`), a rebuild keeps which agent session to load, opening one long transcript no longer holds up the others, a diff in the transcript keeps only the change and its surroundings (`shortDiff`), archiving removes a session's optimizer and skill data, the log moves on while the app runs, and `make test` covers `cmd/`. *Idle agent processes are still kept until quit: stopping one could end its background work and lose an agent's memory if it cannot load, so that waits on a decision.*
5. **Account limits (GUI-028).** Limit reports from the agent become a `limit_update` event (`internal/harness/acp/limits.go`) and a row per change in `limit_reports` (`internal/store/limits.go`), shown on the usage page and in the usage pane (`Limits.tsx`, `frontend/src/limits.ts`). *Unit-tested, with the adapter's forwarding checked against a real run on 2026-11-03; not yet looked at in the app. The per-model weekly figure `/usage` prints is not in these reports, and a figure only updates with a model call.*

### Fonts (built 2026-11-03)

1. **Fonts (GUI-025).** *Checked in the browser with the demo provider: both fonts on the conversation, tool cards, inline code, diffs, a review comment and the terminal, live from the menu, a typed name and an edit of the file, a restart, Back to the app's own font, a hostile value in the file (reported, no request reached a listener on its address), a missing font. Also in a real `WKWebView` pointed at the same app, where Manrope and JetBrains Mono from `~/Library/Fonts` draw and are listed, and the terminal changes font live. That run could not draw terminal text off screen, so columns there are checked from xterm's numbers only, and the native window was not opened.* Settings → Appearance has a Text and a Code font; `ui.textFont` and `ui.codeFont` in `config.yaml` hold them (`internal/config/fonts.go`, `frontend/src/fonts.ts`, `FontEditor.tsx`).

Found on the way: WebKit's canvas does not see fonts the user installed, though the page draws them, so presence is measured in the page. xterm 6 measures its cells on a canvas, so with such a font its cells have the width of the fallback font and it closes the gap with letter spacing: columns line up, slightly looser or tighter than the font's own (JetBrains Mono at 12 px: 0.225 px).

Not in: every font on the Mac in the menu, since a page cannot list them and nothing in the app reads the font folders; any other name can be typed.

### Token optimizers (planned 2026-11-02)

Rewriting shell commands so their output costs fewer tokens (HRN-019, OQ-029). No new dependency; the program is the user's to install, by hand or with one click.

1. **The built-in harness and the session dropdown.** *Built 2026-11-02; tested with a stand-in program, with real RTK 1.1.0 unconfined and under the real Seatbelt sandbox, and in the browser with the demo provider. Real models' behaviour with shortened output, Docker and a Homebrew-installed binary are not checked.* `internal/engine/optimizers.go` asks the program for a rewrite after approval and checks it (`validRewrite`); the shell tool has a permanent `raw` option, so the tool list and the prompt cache do not change with the switch; `optimizer_changed` records the choice; the Optimizer dropdown is in the session header and the ⌘K palette.
2. **Defaults, setup and files.** *Built 2026-11-02; tested with a stand-in program and checked in the browser with real RTK and with none on the PATH.* A Token optimizers page in Settings shows where each program is, a Check button that runs it, the install command and the default for new sessions (`defaults.optimizer`); profiles have `optimizer:`, the new-session form and `freeloop new -optimizer` choose one, `freeloop optimizer <session> [id|off]` and the socket's `sessions.optimizers` and `sessions.setOptimizer` switch one; `optimizers/<id>.yaml` adds or replaces one, hot-reloaded and included in backups. Choosing one that is not installed offers to type its install command in the session's terminal.
3. Measured savings per session from `rtk gain`, the Usage tab by optimizer, on and off in Compare (HRN-016). *Dropped 2026-11-03 (OQ-031): the savings measured were 0.18 %, so RTK is off by default and nothing more is built on it.*
4. **Claude Code and installing from the session.** *Built 2026-11-03; run through the real Claude Code adapter (0.99.1) with real RTK 1.1.0 on a stand-in model: read-only commands ran shorter with no prompt, `touch` and `git push` asked with the original shown, off gave raw output and on again the short form. The install button was tried against GitHub. A real model and a signed-in Claude Code were not tried.* `sessionOptimizer.forAgent` answers the plugin's `PreToolUse` hook (`freeloop optimizer-hook`, `sessions.rewriteForAgent`), the ACP client maps a rewrite back with `Original`, and a tool call's `via` names the optimizer. `internal/engine/optimizerinstall.go` downloads `rtk-oss`. A global `rtk init -g` hook gets a warning in the session.
5. Custom harnesses through `terminal/create`.

A hardening pass on 2026-11-02 tried 130 commands raw and through real RTK in identical repositories, and tried to get hidden commands past the check in zsh. See `docs/SECURITY.md`. An independent review added more. RTK turned a `git diff --output` file into its own summary and showed 10 of 31 commits for `git log` with no note, so those stay as written; pipes, redirections and substitutions no longer are rewritten; RTK runs a repository's copy of `tsc`, `jest`, `gradle` and others, so a rewrite now takes a list of tools and must be the approved text after the program's name; files need approval; a repository's settings no longer reach the program; a sandboxed session could read an unconfined session's stored output, and no longer can; `rtk bypass`, `run`, `err`, `summary` and `test` no longer get past deny rules. Known limits: `rtk recall` hints go stale after a fork or after switching the optimizer off.

Not in: output filters (`rtk pipe` shortened a 22 KB `git log` to 259 bytes in the test, which looks wrong), gateways that sit in front of the model (they raise HRN-008 questions), and squeez and PTK, tested on 2026-11-03 and not added (OQ-029).

### Production readiness (2026-11-03)

A check of the whole app against v1, then the fixes it asked for. What it found and how it was proven is in `docs/SECURITY.md` for security. For data and reliability, each proven on the running app or a real volume:

- **Quitting** stops sessions and their commands (`OnShutdown`, which a Mac's quit reaches where a deferred call does not). A hard kill or crash can still leave a command running; nothing reaps those at the next launch. There is no question before quitting with turns running.
- **A full disk** stops the turn and says so in the problems banner, in place of dropping events unseen. A partly written line is rolled back, and transcripts open in append mode.
- **`providers.yaml` and `mcp.yaml`** are not overwritten when they cannot be read or come from a newer version, and keep a `.bak`. A newer `config.yaml` is treated the same.
- **The session list** is rebuilt from transcripts when the database is missing or damaged (`internal/store/recover.go`): a daily copy, then `meta.json` beside each transcript. Trust is not rebuilt unless the copy holds it.
- **One copy per data folder** (a lock), and startup failures say why in a dialog in place of the app not opening.
- **A log**, `logs/freeloop.log`, redacted and rotated, with crash traces, and a button in Settings.
- **A panic** ends that turn or tool call, not the app. **A lost context line** no longer cuts what follows it, and the user is told. **A crashed agent** starts again at the next message. **A stream that stalls** ends after five minutes and is retried; **one that ends early** is an error, not a finished reply.

Not done: storage views and pruning, sharing checkpoints between sessions. A universal build was dropped on 2026-11-08: Apple Silicon only (NFR-XPLAT-001).

### Live usage (built 2026-11-03)

1. **Live usage in a pane (GUI-023).** *Checked in the browser with the demo provider: a turn moved the pane's totals without a reload, and the pane stayed after one.* `internal/store/recentusage.go` folds usage per five minutes from the transcripts when asked, keeping three days; `Engine.RecentUsage` steps it every 15 minutes for 12 hours and every 30 for 24; `LiveUsage.tsx` draws it, and a pane holds it as `USAGE_PANE` in `workspace.ts`. A pane gets it from "Show in a pane" on the usage page, by dragging Usage from the sidebar, from an empty pane or from the palette.

### Session controls (built 2026-11-03)

1. **Model and effort (PRV-014).** *Run through the real Claude Code adapter (0.99.1) on a stand-in model: choosing Sonnet and low sent `claude-sonnet-5-5` and `effort: low` to the API, and the same after restarting the app. The built-in agent is covered by a test, since the demo provider has no effort. A real model and a signed-in Claude Code were not tried, and the other presets' agents were not asked what they offer.* `internal/engine/options.go` lists a session's options for both harnesses; `harness.Configurable` is ACP's `configOptions`; `SessionControls.tsx` is the row beside the composer.
2. **Attaching any file (SES-019).** *Checked in the browser with a Markdown file, a file with no type, a PDF and a picture.* `frontend/src/attach.ts` sorts files; the engine checks them again in `attachedFiles`. PDFs and other binary files need the model's own document support, which no provider adapter has yet.
3. **Finder (GUI-024).** *The click was tried in the app on a scratch project and reported no error; a window opening was not observed from here, and `open` behaviour was tested with a harmless fake app.* `darwin.FolderArgv`.
4. **Permissions in the session (SEC-002, GUI-007).** *Checked in the browser, across a reload.* Permissions beside the composer, Rules… opening the panel, where rules are added and removed for the session or for all sessions (`session_rule_changed`).

### Session import (built 2026-11-03)

1. **Importing an agent's own sessions (SES-020).** *Run through the real Claude Code adapter (0.99.1) and `claude` 2.1.317 on a stand-in model and a scratch home, in the browser and from the command line: a session made with `claude -p` imported, and the next prompt reached the model with the earlier messages and tool output. Copies of three of the user's sessions (7, 42 and 281 MB) imported in 2 to 3 seconds, all 45 pictures of one showed, and no tool call was left running. A real signed-in Claude Code and a real model were not tried.* `internal/harness/acp/browse.go` lists, copies and replays with a process that can ask for nothing; `internal/engine/sessionimport.go` makes the session; `SessionImport.tsx` is the top of the Import page, with a link from the new-session form; `freeloop import` and the socket's `sessions.importable` and `sessions.import` do the same. `session_imported` records the source, and what comes before it stays out of the usage views.

Found on the way: `session/load` adds two metadata lines to the session's file, and `session/fork` leaves the original unchanged, which is why the import copies. The replay stops at the last compaction: 76 of the 9,677 tool calls of the largest session came back. `session/list` without a folder missed two of six sessions that the same call with their folder found, so the import also asks about every project here. An agent names its copy `<title> (fork)`, so an imported session keeps its title.

Not in: reading Claude Code's files to show what was compacted away. They are not a documented format, and the agent's context would not change.

### Direction (2026-11-03)

After a look at everything built, the one claim worth selling: **every coding agent, vendor or local, runs under the user's permission policy and the user's accounting, and neither the agent nor the repository can loosen either.** Two things proved it on 2026-11-03: an agent that switched itself to bypass stayed in plan (`internal/harness/acp/acp.go`), and a repository's `.claude/settings.json` allowing `Bash(*)` got a `git push` past Claude Code's own gate and not past the broker (OQ-029). Freeloop's token counts for Claude Code now match the agent's own reported cost to the digit (PRV-008), which no vendor's view gives across agents.

What follows, in order, before anything else in v1:

1. **What the policy held (SEC-014).** *Built 2026-11-03. The `git push` case ran through the real Claude Code adapter (0.99.1) with a real model and a scratch repository whose `.claude/settings.json` allows `Bash(*)`: Freeloop was asked, the denial held and the remote stayed empty. The views are covered by tests and were not checked in the running app.* Make the policy visible: what it stopped, in the session and across sessions. `heldStats` in `frontend/src/activity.ts` and `held` in `frontend/src/transcript.ts` read what the transcript already had, so past sessions count too; a `mode_refused` event replaces the notice for an agent's own switch, so refusals before this build do not count.
2. **A cost summary to share (GUI-029).** Make the cost view the one people show each other. *Built 2026-11-04: "Share summary" on the usage page, as Markdown tables and a PNG (`frontend/src/costsummary.ts`, `CostSummary.tsx`). The text is unit-tested; the picture has not been looked at in the app.*
3. **An SSH environment (ENV-008).** *Built 2026-11-04, at the user's choice and ahead of the release, for its boundary: an agent's commands on a machine that is not this Mac.* The project stays here and a session has a working copy there (OQ-034). Run with a real sshd and a real session of the built-in agent. Real networks, Linux machines and the form in the running app are not checked. `internal/environment/ssh.go`, `sshsync.go`, `internal/engine/ssh.go`. External agents over SSH, a terminal there and Run scripts there are not in yet: an external agent is refused, not run here instead.
4. The release: the definition of done above, with signing as OQ-009 has it.

**Plan to 0.1.0 (2026-11-05).** Goals: the claims true for every agent (G1), proven by `make verify-agents` in CI weekly (G2), visible to a new user within five minutes (G3), every agent's usage priced when its model is known (G4), the definition of done above met (G5), no code commit skipping CI (G6). In order:

- *Phase 0, baseline.* CI runs `make check-app` and, weekly, `make verify-agents` (built 2026-11-05). Goose is pinned by the user with `TRUST_NEW_AGENT=1 make verify-agents`; until then its scenarios are skipped, loudly.
- *Phase 1, the policy true for Claude Code.* SEC-015 and SEC-016 built 2026-11-05: repository hooks off in untrusted projects, "always" kept by the app, bypass and accept edits applied by the broker. Built 2026-11-06: one list of what is acted on later (SEC-005), reading a secret asks (SEC-017), the trust prompt naming agent config and its hooks (SEC-006), a restore reading only the app's own Keychain item. Not done: drafts in the data folder, and the hostile PDF in the packaged app, which needs the user.
- *Phase 2, proven and shown.* Every agent's scenarios with shell, mode switch and usage cases; one "Your policy" page with every scope and what each agent decides itself; "Would this be allowed?" in the app and as `freeloop check` (SEC-018, built 2026-11-06: `Broker.Explain`, `internal/engine/policy.go`, the Policy page in `Rules.tsx` and `frontend/src/policy.ts`; not yet looked at by the user in the app, and no link to it from a session's Approvals tab); the demo inside the app; the trust prompt naming what a repository tried; holds where people look.
- *Phase 3, the bill for every agent.* External agents priced by the chosen or reported model; what each agent reports, said wherever usage is shown; *Measured 2026-11-06 with the pinned agents on a stand-in model: Claude Code reports each call with its model; Codex CLI a turn's total and its model setting; Gemini CLI a turn's total and no model; OpenCode no tokens at all; goose waits to be pinned. Built the same day: a turn's total priced by the agent's model setting, a usage event saying how it was measured (`measured`), the session's cost saying so (`costText` in `frontend/src/cost.ts`), and each agent's report asserted in `TestPresetScenarios`. Then, the same day: the usage page and the shared summary say which agents went unreported and which are counted by turn totals (`frontend/src/reporting.ts`, two columns added to `usage`), budgets with their alerts and the queue waiting under one (PRV-017, `internal/engine/budgets.go`, `Budgets.tsx`, `BudgetTable.tsx`), and the held count in the shared summary. Not yet: Gemini CLI's model, and a look at all of it by the user.* budgets and limit alerts; the held count in the summary.
- *NFR-PRIV-003, built 2026-11-07:* what each model call carries besides the conversation is recorded in the transcript when it changes and shown in the Info tab as "What the model sees" (`internal/engine/limit.go`, `frontend/src/ModelSees.tsx`). Not yet looked at by the user.
- *Phase 4, the release.* The MVP checklist, the user's real runs, ten busy sessions measured, commands reaped after a crash (NFR-REL-005, built 2026-11-06: `internal/environment/ledger.go`, `darwin.ProcessStart`), a release workflow on tag (built 2026-11-06, not yet run: no tag exists), the notification permission shown in the app, two weeks of daily use.

**Feature freeze until 0.1.0 (2026-11-05).** Of 26 commits after the direction was set, 18 were interface features outside it. Until the release, only bug fixes, security fixes and the plan below go in: the claims made true for every agent, proven by `make verify-agents`, shown in the app, the bill covering every agent, then the release. Code changes run CI: no `[skip ci]` on a commit that changes code.

Not before those: features that copy a vendor app's. Token optimizers get no more work (OQ-031). What the vision lists as borrowed from vendor apps stays built; nothing more of that kind is added without a reason in this list.

## From here to 0.1.0 (planned 2026-11-07)

Where it stands. The release plan above is built through phase 3, and its phase 4 tooling is written: a release job on tags, the version at `0.1.0-beta.1`, download steps in the README, and the MVP checklist in `docs/CHECKLIST-0.1.md`. The checklist has 33 rows automated, 41 partly covered (most of the rest needs a real account or a person), 8 for the user's runs and 1 gap (which Macs are supported; closed 2026-11-08, Apple Silicon only, which leaves that row partly covered until macOS 14 itself is run). Since the release plan, stopping what a crash leaves running (NFR-REL-005), recording what each model call carries (NFR-PRIV-003), checking every event against the schema (EXT-002), and tests for SEC-010, HRN-008, GUI-004, NFR-PERF-003 and NFR-PERF-005 are built. What stands between this and a release is no longer code. It is three things: a CI the repository can afford, which the release job depends on; the user's runs on real accounts; and people other than the author using it.

The private repository ran out of GitHub Actions minutes on 2026-11-06: macOS minutes count ten times, and a run takes about eight. Twelve commits wait unpushed for that reason, and the release job could not run.

Goals, each measurable:
- **B-G1, an affordable CI that stays green.** Every push runs the Go and frontend tests; the macOS build and the app check run on tags, weekly and by hand; a red run is fixed before new work.
- **B-G2, the claims hold on real accounts.** The checklist's runs for the user are done, with what each found recorded in it.
- **B-G3, a stranger gets going in five minutes.** From download to a first reply, or to the demo, without help.
- **B-G4, no loss.** Two weeks of daily use without a lost transcript or crossed sessions, by the user and by testers.
- **B-G5, five testers for a week**, with their blocking issues fixed (the v1 definition of done).

### Stage 1: plumbing (Claude, about a day; needs nothing from the user)

*Done 2026-11-07, but for one thing: CI in two jobs, with the Go packages building on Linux (one macOS-only test tagged); the twelve commits pushed; a slow transcript read logged; the release files built and checked on a Mac (`make release`: every checksum matches, the DMG holds the app at 0.1.0), and the app rebuilt. Not done: no job ran, since GitHub refused to start any ("recent account payments have failed or your spending limit needs to be increased"), Linux included. Until the allowance resets or the limit is raised, releases take the route in DEVELOPING.md, "When CI cannot run", and the Linux job's first run will still show which tests assume macOS.*

1. **CI in two parts.** `.github/workflows/ci.yml`: a `test` job on `ubuntu-latest` runs `make verify-deps`, the Go tests of every package that builds off macOS, and the frontend tests and type check, on every push and pull request. The `build` job on macOS keeps the build, `make test`, lint and `make check-app`, and runs on tags, weekly and from the Actions page. First, check which Go packages build on Linux: the macOS-only code is in `internal/platform/darwin` (NFR-XPLAT-002), and anything else that does not build there is a finding. *Done when:* a push costs about a tenth of what it did, and both jobs are green.
2. **Push the twelve commits**, then watch both jobs (`gh run watch`), and fix anything red before going on.
3. **A dry run of the release job** from the Actions page (`dry_run`): download its files, check `SHA256SUMS`, mount the DMG and read the version. If minutes are short, `make release` on the user's Mac builds the same files; DEVELOPING.md gets the steps for that, with `gh release create --draft --prerelease`.
4. **A time for opening a session in the app's log**, so the stall seen only on CI (opening took over 30 s in about one run in four) can be found if a tester meets it.
5. **Rebuild the app** in `bin/` for the user's runs.

### Stage 2: the user's runs (the user, about a week alongside daily work)

In the checklist's order of risk, recording each result in `docs/CHECKLIST-0.1.md`:
1. Claude Code on a subscription, from the app, with an MCP server; nothing of its sign-in stored by the app.
2. Real providers: Anthropic, OpenRouter and Ollama, three sessions on three repositories at once.
3. A `kill -9` of the packaged app while an agent streams; relaunch; nothing lost, and nothing left running (`running.json`).
4. Ten busy sessions on the user's Mac: typing lag, memory (`make memcheck`), no crossed sessions.
5. A first run on an empty data folder, timed to a first reply (B-G3).
6. The hostile PDF in the packaged app; notifications in the background; light and dark; keyboard only, and a short VoiceOver pass over an approval.
7. `TRUST_NEW_AGENT=1 make verify-agents`, which pins goose and runs every agent's checks.

Claude, meanwhile: fix what these find, first; then stage 3.

### Stage 3: what testers need (Claude, about three days)

1. **The demo inside the app (G3 of the release plan; GUI-045).** *Done 2026-11-08.* "Try it with a demo" on the first-run screen and "Try the demo" in the command palette start a session on a scripted agent in a scratch git project beside the data folder (inside it, agents may not read or write: SEC-013). The one turn meets each claim in turn: a read goes ahead; an edit asks; `git push` is refused by a session deny rule, also in bypass; a write into `.git/hooks` is refused as a protected path; reading `.env` asks. Each step says why. "Remove demo" deletes the session, the folder and the project record. `Engine.StartDemo` and `RemoveDemo` (`internal/engine/demo.go`), the hidden `freeloop-tour` provider (`scripted.Tour`), `frontend/src/demo.ts`, the banner in `SessionView.tsx`. *`make check-app` runs it to the end and removes it; engine tests check each decision. The tour does not end on the usage page. Not yet looked at by the user.*
2. **A model for external agents' usage.** *Done 2026-11-08.* A profile on an external agent can name the model it runs on (**Model it runs on** on the Profiles page), for agents that do not report it (Gemini CLI), so its turn totals are priced (PRV-008). The agent is not told, and a model it reports wins; the session stores it, the ACP adapter prices by it. `harnesses_test.go` `TestAnAgentsProfileNamesItsModel`, `acp_test.go` `TestUsagePerTurn`.
3. **A way to report a problem (GUI-046).** *Done 2026-11-08.* Settings, Data: "Copy a report" puts the version, macOS, the Mac's model, chip and memory, the last 200 log lines (redacted again, with the home folder as `~`) and the session's agent, provider and model on the clipboard, for an issue, with "Show what is copied" beside it; the palette has it too. Nothing is sent anywhere (NFR-PRIV-001). `Engine.ProblemReport` (`internal/engine/report.go`), `config.LogTail`, `darwin.SystemInfo`.

### Stage 4: beta 1 (the user decides, Claude prepares)

1. **Decide which Macs.** *Decided 2026-11-08: Apple Silicon only, macOS 14 or later (NFR-XPLAT-001). The build is `arm64` alone, `make package` checks it, and `Info.plist` says so, so an Intel Mac refuses it with the system's own message.*
2. **Tag `v0.1.0-beta.1`**, read the draft release, publish it.
3. **Add three to five testers** to the private repository, with the release notes and a short note on what to try and how to report.
4. **A weekly beta**: fixes go out as `0.1.0-beta.2` and on, each through the same tag and draft.

### Stage 5: 0.1.0 (after two weeks of beta)

*Done when:* every checklist row is automated or has its run recorded; two weeks of daily use without loss (B-G4); five testers for a week with their blocking issues fixed (B-G5); and CI green.

Decisions before 0.1.0, each the user's:
1. **Signing (OQ-009, reopened).** With an Apple Developer account (99 USD a year), releases are signed and notarized: no "Open Anyway", and the Keychain stops asking after each update. Without it, both stay, as the README says.
2. **Going public.** The steps in DEVELOPING.md, "Before the repository goes public", with their own approval: rewriting commit identities, cleaning up what GitHub holds, a last scan, private vulnerability reporting. A public repository's Actions minutes are free, and the update check works only then.
3. **Contributions (OQ-017):** the DCO and a code of conduct when they open after v1 scoping.

### Not before 0.1.0

The feature freeze stands: only fixes, the stages above and what the testers' blocking issues need. The two "After 0.1.0" plans below wait for the release and for real use, which is what will show whether they are the right next thing.

### Risks

- **CI minutes run out again.** Stage 1 cuts the cost about tenfold; a public repository removes it.
- **Real accounts behave unlike the stand-ins.** Stage 2 comes before testers for that reason.
- **The stall seen on CI is real.** Stage 1's timing in the log will show it if a tester meets it. *2026-11-08: the app check's driver now holds off App Nap, the likeliest cause; not reproduced in four runs under load here. The accepted low risks in SECURITY.md are all fixed ("Closing the accepted risks").*
- **An unsigned app puts testers off.** Signing is the first decision before 0.1.0.

## After 0.1.0: the bill measured by what it bought (planned 2026-11-06)

Three features that make the second claim, one honest bill across agents, answer the question a user actually has: what did this work cost, and could it have cost less. They wait for the release and for every agent's usage to be priced (release plan, phase 3), or they mostly cover Claude Code.

1. **Outcomes recorded (SES-024).** `commit`, `merged`, `pull_request_state` and `compare_kept` events, folded into activity, with a one-time count of past sessions. About 4 days.
2. **Cost per outcome (GUI-039).** `frontend/src/outcomes.ts` and a card on the Efficiency tab; cost per merged change in the shared summary. About 3 days.
3. **Subscription value (PRV-016).** `subscriptions` in `config.yaml`, edited beside the prices; `frontend/src/subscriptions.ts` and a card beside the limits. Independent of the other two, and the one that could come earlier. About 3 days.
4. **A cheaper model for similar work (GUI-040).** `frontend/src/recommend.ts` and a card on the Efficiency tab, after 1 and 2 and a few weeks of outcomes. About 4 days.

Open for the user: whether a finished task is a merged change or archived with a commit (the plan) or an explicit "Mark done"; how often `gh` is asked about a pull request (the plan: every 30 minutes, 30 days back); the recommendation's thresholds (3 sessions, 30% cheaper, merge rate within 10 points).

## After 0.1.0: more work started, and none of it stranded (planned 2026-11-06)

For someone running many sessions, the waste is in the gaps: an agent idle while tasks wait, and a session that hit its limit or got stuck and sits there. Both build on what exists.

1. **A queue of tasks (SES-025).** *Built 2026-11-06, at the user's request ahead of the release: `queue.json` (`internal/config/queue.go`), `internal/engine/queue.go` run by the schedule loop, the Queue page (`Queue.tsx`, `frontend/src/queue.ts`) from the Board and the palette, and a `make check-app` step. Not yet: several issues at once, budgets (PRV-017), a task that hits its limit handing off by itself.*
   - Engine: `internal/engine/queue.go`, a `queue.yaml` beside `schedules.yaml`, run by the schedule loop (`scheduleLoop`, every 30 seconds). It starts a task with `CreateSession` and `Prompt` when there is room: the queue's limit, the provider's slots (`limit.go`), the agent's limits (`Engine.Limits`, paused until `resetsAt`) and budgets (PRV-017).
   - Interface: a Queue page with the board's columns (waiting, running, done), "Add to queue" in the new-session form, several issues at once from the issue picker, and the queue's limit in Settings.
   - Tests: a fake clock and the scripted provider for start order, slots, a limit pausing and resuming, approvals keeping a slot, and a paused queue. A `make check-app` step adds three tasks and sees two start.
2. **Hand off when stuck or out of allowance (HRN-020).** *Built 2026-11-06, at the user's request ahead of the release: `internal/engine/stuck.go` (the watcher, on the store's append, and `HandOff`), `event.Stuck` and `event.HandedOff`, `Broker.CancelSession`, the banner in `SessionView.tsx`, the attention entry, and "When a session is stuck" on the Profiles page. Tested from events and with `onLimit` continuing on the scripted provider; not yet looked at in the app.*
   - Engine: `internal/engine/stuck.go` watches a running session's events: a rejected `limit_update`, the turn limit, the same tool failing three times, two rewinds in a turn, errors after retries. It emits a `stuck` event with the reason and a suggested profile. `SwitchProfile` (`handoff.go`) gains: ending the old session's open approvals, a link both ways, and the old session's last request carried into the new one.
   - Interface: one banner in the session and an entry in the attention view, "<reason>: continue on <profile>?". A per-agent fallback profile in Settings, and `onLimit` in a profile for doing it without asking.
   - Tests: each trigger from recorded events, the handoff's links and carried request, and `onLimit`. A `make check-app` step with a scripted provider that keeps failing one tool.

Order: the handoff first, since it needs no new data and a reached limit is today's commonest dead end; then the queue, which uses the handoff for a task that hits its limit.

Open for the user: the queue's default limit of 3 sessions at once; whether a queued task gets its own worktree by default (the plan: yes, so tasks in one repository do not edit the same files); whether `onLimit` may continue without asking (the plan: only if the profile says so).

## Workflows (planned 2026-11-08, at the user's request ahead of the release)

Matt Pocock's way of working with agents, built into the app (WFL-001 to WFL-009): grill the brief, write a PRD, slice it into issues, run the unattended ones, review them, QA by hand, clean up. Packaged like the token optimizers: a file per workflow, one built in, nothing run or installed until the user asks. It is built on what exists: the queue (SES-025) runs the AFK sessions, the review by a second agent (HRN-018) reviews them, the attention view (GUI-002) holds what waits for the user, and the broker asks as always (SEC-003). No new dependency.

1. **Workflow files, the built-in workflow and how to invoke it (WFL-001 to WFL-003).** *Built 2026-11-08.* `internal/config/workflows.go` (format, validation, fingerprint, the built-in workflow and its skill texts), `internal/workflow` (templates; pure), `internal/engine/workflows.go` (listing, approval, choosing, running a stage, installing skills), `workflow_changed`, a `workflow` column on sessions, `workflow:` on profiles and `defaults.workflow`; the Workflows page in Settings, the picker beside the optimizer, stage commands in the composer and the palette.
2. **The backlog (WFL-004).** *Built 2026-11-08.* `internal/workflow/backlog.go` (frontmatter, ready and blocked, cycles), `internal/engine/backlog.go` (reading through the environment in trusted projects, archiving), the Backlog column on the Board.
3. **The AFK run (WFL-005).** *Built 2026-11-08; engine tests with the scripted provider, and `make check-app` on the demo provider in the real app with its sandbox.* `internal/engine/afk.go` and `afkruns.json`: one run per project at a time, its worktree shared by its sessions through queue tasks, the commands allowed up front as session rules, the stop conditions, started from the Board, the palette, `/afk` and a schedule.
4. **Review and "Ready for QA" (WFL-006).** *Built 2026-11-08.* The run commits what is left, reviews each issue with the review profile, and lists it in the attention view until the user marks it QA'd. A `make check-app` step runs the built-in workflow on the demo provider from `/slice` to two issues ready for QA.
5. **Guided stages and the run in full (WFL-008, WFL-009).** *Built 2026-11-08.* `workflow_stage` events and `needsInput` (`internal/engine/workflows.go`), the stage strip (`WorkflowStrip.tsx` over `stageProgress` and `stageOutputs` in `src/workflows.ts`), the brief in "Start a workflow…" (`NewSession.tsx`), the run view (`AFKRunView.tsx`) and commands of the user's own in the AFK dialog (`AFKStart.Extra`, `projectMoreChecks`).
6. **Parallel runs (WFL-007), later.** *Not built.* One worktree per unblocked issue up to the queue's limit, then a merge session (OQ-042). It needs the run to track several steps at once and a merge step; the run's record is kept a list of steps so it can.

Not in: GitHub issues as a source (OQ-038, not planned), a run continuing past a failed issue, the review's findings sent back to the issue's session without the user.

Not tried yet: a real model on any stage, Claude Code or another external agent as the AFK or review profile (the code path is the same as for the built-in agent, since done is read from the issue file and the review is HRN-018's), and a run on a real project's tests under the sandbox. The skill texts are Freeloop's own, written after the talk; none of Matt Pocock's skill files were copied.

## Windows and Linux (planned 2026-11-08, at the user's request before going public)

The user wants the app on macOS, Windows and Linux, working essentially the same, before the repository goes public (NFR-XPLAT-003, OQ-045). Linux was begun the same day (bubblewrap, the Secret Service, the platform layer); Windows starts from nothing but the interfaces NFR-XPLAT-002 kept. Branch `cross-platform`.

1. **The process and file layer on Windows.** *Built 2026-11-08; cross-compiled and vetted on a Mac, not yet run on Windows.* `internal/environment`: commands in job objects that end with the app (`proc_windows.go`), so a crash leaves nothing running without a ledger; Git Bash as the shell (OQ-043); Windows' system variables passed on; reads and writes that check where the opened file really is (`nofollow_windows.go`); the terminal on ConPTY (`terminal_windows.go`); the data folder lock; Docker Desktop with Windows paths mounted at `/c/...`; SSH through Windows' OpenSSH, a connection per command.
2. **The Windows platform.** *Built 2026-11-08, not yet run on Windows.* `internal/platform/windows`: the Credential Manager, long values in parts; Notepad for a file to edit, the PDF viewer for a PDF, File Explorer for a folder, never a file opened by its type otherwise; system details for a problem report. No sandbox (OQ-044).
3. **The engine on Windows.** *Built 2026-11-08, tested on a Mac with the system as a parameter, not yet run on Windows.* Windows' protected paths (credential stores, DPAPI keys, browsers, password managers) and rules with `\`; the log and settings closed before a rename; the database path escaped; `edit_file` keeping CRLF files CRLF and grep ignoring the `\r`; the system prompt telling the model it writes bash in Git Bash; a sandboxed environment refused with the reason (OQ-044); RTK's Windows build pinned; git hooks off without `/dev/null`; worktrees in the local AppData; a `.cmd` shim run through Git Bash, never cmd.exe. Git's system config is read on Windows, waiting for the user (OQ-046).
4. **The interface on each system.** *Built 2026-11-08.* Ctrl as the primary shortcut key on Windows and Linux, per-system names (File Explorer, Credential Manager), paths with either separator, help per system.
5. **Packages and CI.** *Built 2026-11-08.* `make windows-exe` and `make windows` (NSIS installer and portable zip per architecture, unsigned), `make linux-package` (AppImage and `.deb` in the Docker image), `make release` gathering what the host can with SBOMs and checksums; a Windows CI job on every push under Git Bash, and packaging jobs on tags. Checked: both Windows exes build on a Mac, the installer compiles, the arm64 `.deb` installs on Ubuntu 24.04; not yet run on Windows, and no CI job has run (billing).
6. **Linux's remaining parts**, from the first commit: `pasta` and the listed-hosts network. *Built 2026-11-08* (OQ-037): localhost through pasta, pinned to Ubuntu 24.04's 2024-02-20, with the host's loopback TCP ports forwarded and nothing beyond; without pasta, or offline, the sandbox's own loopback; the listed hosts through the proxy, forwarded by pasta or reached through its Unix socket and a bridge that is the app's own binary. Tried in the build image (pasta built from source there) with `TestTheSandboxs*` in `internal/platform/linux`, against a stand-in proxy and against the real proxy and internet, not yet on a Linux desktop or under Ubuntu 24.04's own AppArmor profiles.

*Done when:* the Go tests pass on the Windows and Linux runners, the packages build, and the user has run a session on each system.

Not in: a native Windows sandbox (OQ-044), signing for Windows (SmartScreen warns once, as macOS does for an unsigned app), the Microsoft Store, Flatpak and Snap.

## German and Dutch (planned 2026-11-08, at the user's request)

The interface in German and Dutch, chosen in Settings (GUI-048; the PRD and issues are in `backlog/`). Our own small translation module, no dependency; only Freeloop's own text is translated.

1. **The setting, the module and the Settings page (backlog 001).** *Built 2026-11-08.* `ui.language` in `config.yaml` (`internal/config`), `frontend/src/i18n.ts` with the catalogues in `frontend/src/i18n/`, the picker beside Theme, `<html lang>`; the Settings page and what it is made of (the column, shortcuts, colours, fonts, marks, prices, usage, budgets, hooks, what waits for approval, the native dialog's buttons). A `make check-app` step switches to Deutsch and back.
2. **Formats (backlog 002).** *Built 2026-11-09.* `locale()` in `frontend/src/i18n.ts`, which every number, date and time formatter takes (a source test holds them to it); tokens, costs (in US dollars), the charts' axis times and numbers in the catalogues' placeholders in the language's style. A `make check-app` step switches the usage pane to Deutsch and back.
3. **The views used all day (backlog 003).** *Built 2026-11-09.* The sidebar and its footer, the session's header, the transcript's own words, the composer and its controls, the right panel, the Diff and Files tabs, and the app's own slash commands and mode names (`modeName` in `frontend/src/commands.ts`); what an agent advertises is shown as given. The source test reads all of them. A `make check-app` step switches the session to Deutsch and back.
4. **The remaining views (004), the help pages (005), the menu bar and notifications (006), the guards against untranslated text and clipped labels (007), and the user's review of the wording (008).** *Not built.*

## The Board as a workflow board (planned 2026-11-09, at the user's request)

GUI-016 reworded: the Board becomes a scrum board of workflow issues, designed in one conversation and written up as `backlog/prds/workflow-board.md`, sliced into the issues `backlog/issues/009` to `015`. Built a slice at a time, each by the work-an-issue method with its tests and an app-check step:

1. **009, swimlanes and the Queue icon.** *Built 2026-11-10.* Rows per project, four columns, compact cards (`placeIssues`, `cardMeta` in `frontend/src/board.ts`, `Board.tsx`); the session columns and the sessions' cards are gone; the Queue's icon in the sidebar counts waiting tasks. German and Dutch column names.
2. **010, waiting for you on the card.** *Built 2026-11-10.* An issue whose session waits for an answer is marked "Waiting for you" and the request is answered on its card (`waiting` in `placeIssues`, `RequestView` on the card); the header counts them and scrolls to the first. The demo's first sliced issue asks a question, so `make check-app` has a real waiting card.
3. **011, review and QA, and stopped issues.** In review's two states with QA done; a stopped issue back in Backlog with a mark.
4. **012, rows collapse, a filter, the Done cap.**
5. **013, untrusted rows and the empty board's actions.**
6. **014, the read-only detail panel.**
7. **015, a look in the running app (the user).**

Not in: dragging, a Sessions view on the Board, filters beyond a project, parallel runs, changes to Needs attention.

## Later (unscheduled)

PRV-013, HRN-015, SKL-009, EXT-006, EXT-007, a native Windows sandbox (OQ-044), general work agents beyond coding, team features.

## Cut order

If time runs short, remove from the current milestone in this order; each item is independent and the milestone still holds together without it.

1. M2: GUI-010 onboarding wizard (ship a documented manual setup).
2. M2: SKL-005 install from folder or zip (import from `~/.claude/skills` and copy by hand).
3. M2: MCP-004 passing servers to ACP harnesses (external harnesses use their own MCP config).
4. M2: GUI-008 notifications (the attention view and dock badge remain).
5. M2: SES-003 ACP session load (external sessions restart fresh after relaunch).
6. M3: GUI-015 multi-window, GUI-014 command palette.
7. M3: PRV-012 native Gemini and Responses providers.
8. M3: HRN-014 subagents.

## Sequencing rationale

- Providers before harness profiles: a loop that works against three provider types shakes out the normalization layer early, when it is cheap to change.
- Built-in harness before the ACP client: the built-in harness defines the event schema from real needs; mapping ACP onto a schema designed for the GUI is easier than the reverse.
- Attention view and diff panel in the MVP, not M1: both matter only once several sessions and real edits exist, and M1 proves those first.
- Docker after the MVP: the environment interface is in from M0 and the local implementation forces every tool through it, so the Docker implementation is additive rather than a refactor.
- No signing or notarization (OQ-009, the user's choice): a downloaded build asks to be allowed once in System Settings.
