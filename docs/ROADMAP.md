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

Status: built 2026-10-28; CI has not yet run on GitHub.

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
- Signed and notarized builds; Homebrew cask; a VoiceOver pass with no blocking issues.
- At least five people other than the author have used it for a week and their blocking issues are fixed.

### Feature order (planned 2026-10-29)

With M2's buildable work done, the v1 features come in this order, chosen for what most limits running many sessions on your own models; each is a slice with its own commit, tests and docs.

1. **Worktree per session (SES-010, OQ-008).** *Built 2026-10-29.* A session can get its own git worktree and branch, so two sessions on one repository never edit the same checkout; its Diff panel shows its own changes; merge back into the main checkout from the panel; the worktree is removed on archive when it holds no uncommitted work. The new-session form asks when another session already works in the project and remembers the answer per project.
2. **Context compaction (HRN-013).** *Built 2026-10-29.* A context meter; at 80 % of the model's window the built-in harness summarises older turns into one message, records the compaction in the transcript and continues; a manual compact command.
3. **Docker and devcontainer environment (ENV-005, ENV-006, ENV-007, OQ-014).** *Built 2026-10-29, tested against Colima.* An environment kind that runs every command, file access and external agent inside a container built from `.devcontainer/devcontainer.json` or an image, the repository bind-mounted, with setup and health checks.
4. **Checkpoints and rewind (SES-014).** *Built 2026-10-29.* A snapshot of the working tree before each turn that changes files; "rewind to here" on a user message restores the files and the context.
5. **Search across sessions (SES-007).** *Built 2026-10-29.* An SQLite full-text index over transcripts, searched from the sidebar.
6. **Cost estimates (PRV-008, OQ-023).** *Built 2026-10-30.* A price table per model, cost per turn, session and day.
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

1. **Workspace (GUI-017).** *Groundwork built 2026-10-31: the layout rules (`frontend/src/workspace.ts`), keeping the layout in `workspace.json` (`internal/engine/workspace.go`), a session view that is safe to show more than once (approval keys, palette actions and the live region act only in the focused pane) and the ⌥⌘1–4 and ⌥⌘←/→ shortcuts. The component that draws the panes is not built yet.* Up to four sessions side by side in the main window.
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
2. **Worktree setup and run scripts (SES-016).** *Built 2026-10-31; checked in the browser on a scratch project (copy, setup, ports, Run serving on the session's port, a failed setup with Start anyway) and in `TestWorktreeScripts`. As built, the setup state lives in the app's run only: after a restart a session no longer waits for a setup that was interrupted, and the archive command runs on archive, not on delete.*
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

## Later (unscheduled)

ENV-008 SSH, ENV-009 native sandbox, ENV-010 network policy, PRV-013, HRN-015, SKL-009, MCP-009, SET-006, EXT-006, EXT-007, NFR-XPLAT-003 (Linux, then Windows), general work agents beyond coding, team features.

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
- Signing and notarization at v1 at the latest: before then, friends-and-family installs can right-click to open; a public release to strangers should not require that.
