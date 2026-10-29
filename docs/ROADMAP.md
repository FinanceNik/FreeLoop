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

Status (2026-10-29): M2 slice 1 added the attention view (GUI-002): a sidebar entry with a count opens a list of every session waiting for approval, waiting for input or in error, across projects, oldest request first, with Allow once, Allow always (this session) and Deny on each request, the error message and a Dismiss button for a failed session, and a jump to each session. A failed turn now leaves its session in the `error` status until the next prompt or a dismiss. Slice 2 added the diff and commit panel (GUI-005) as the Diff tab of the right panel: every file that differs from the session's base commit (added, modified, deleted, untracked; changes committed since the start shown as committed), a unified diff per file with line numbers, stage and unstage per file or all, commit with a message the session's model can suggest, and discard with a second click; files the agent's tools edited are marked and link to the edit.

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
- Sessions: SES-007, SES-008, SES-009, SES-010, SES-011.
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

## Later (unscheduled)

ENV-008 SSH, ENV-009 native sandbox, ENV-010 network policy, PRV-013, HRN-015, SKL-009, MCP-009, SES-012, SES-013, GUI-016, SET-006, EXT-006, EXT-007, NFR-XPLAT-003 (Linux, then Windows), general work agents beyond coding, team features.

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
