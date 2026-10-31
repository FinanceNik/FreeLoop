# Requirements

Project: **Freeloop**. Status: planning complete; M0 in progress. Companion documents: [VISION.md](VISION.md), [ARCHITECTURE.md](ARCHITECTURE.md), [ROADMAP.md](ROADMAP.md), [RISKS.md](RISKS.md), [SECURITY.md](SECURITY.md), [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md).

## Conventions

- **ID**: `AREA-NNN`. IDs are stable; retired requirements are marked `(dropped)` rather than renumbered.
- **Priority**: `MVP` = in the first public release, `v1` = the first release meant for other people, `later` = after v1 or unscheduled.
- **Accept**: acceptance criteria, written so a test or a five-minute manual check can pass or fail them.
- Milestones that group these requirements into shippable slices are in [ROADMAP.md](ROADMAP.md). The thin vertical slice (M1) is a strict subset of MVP.

Decisions these requirements encode (from the planning interview, October 2026): macOS only for now; Go core, Wails shell, React/TypeScript frontend; the built-in harness is the base and Claude Code is one external harness; the app never touches vendor subscription tokens; sessions run across multiple repos concurrently; Claude Code style permission modes plus rules; secrets in the macOS Keychain; SQLite plus JSONL on disk; no telemetry; Apache-2.0.

## Functional requirements

### Providers and models (PRV)

- **PRV-001** · MVP · **Provider registry with built-in types.** Three provider implementations: Anthropic (native Messages API), OpenAI-compatible (generic chat completions with tools, with presets for OpenAI, OpenRouter, LM Studio, llama.cpp server, vLLM, Ollama's `/v1`), and Ollama native (for model listing and pull status). *Accept:* a user can add one provider of each type from the UI and see it listed; presets prefill base URL and required headers.
- **PRV-002** · MVP · **Add and test a provider.** Form with name, type, base URL, API key (stored per SEC-004), optional default headers. "Test connection" performs a minimal request. *Accept:* a wrong key or URL produces a readable error naming the provider; a correct one shows the models found.
- **PRV-003** · MVP · **Model discovery.** List models from the provider's endpoint where one exists; otherwise allow manual model IDs. Favorites and hidden models. *Accept:* after adding Ollama the locally pulled models appear within 2 s; a manual model ID can be used in a session without restarting.
- **PRV-004** · MVP · **Streaming completion with tool calling, normalized.** All providers stream text, tool calls (including streamed tool-argument deltas) and usage into the internal event schema (EXT-002). *Accept:* the same built-in harness session transcript is produced, modulo content, against Anthropic, OpenRouter and Ollama for a prompt that triggers one `read_file` tool call.
- **PRV-005** · MVP · **Model capability flags.** Per-model and per-provider: tool calling, `tool_choice` support, parallel tool calls, streamed tool arguments, stable tool-call IDs, structured output, vision, reasoning controls, context window, max output; auto-detected where possible (a tool-calling probe on first use for local models), user-overridable. *Accept:* choosing a model without tool calling for the built-in harness is blocked with an explanation; an override makes it selectable.
- **PRV-006** · MVP · **Switch model mid-session (built-in harness).** *Accept:* the next turn uses the new model; the transcript records the switch.
- **PRV-007** · MVP · **Token accounting per turn and session.** Input, output, cache read/write where reported. *Accept:* totals in the session header match the sum of per-turn usage.
- **PRV-008** · v1 · **Cost estimates.** Editable price table per model; session and per-day totals. *Accept:* a session shows an estimated cost that updates per turn.
- **PRV-009** · v1 · **Prompt caching.** Use provider caching (Anthropic cache breakpoints; OpenAI automatic) for system prompt, skills index and tool definitions. *Accept:* cache-read tokens are non-zero on the second turn of a session against Anthropic.
- **PRV-010** · v1 · **Retry and fallback policy.** Exponential backoff on 429/5xx; optional fallback model per profile. *Accept:* a forced 429 is retried and surfaced as a non-fatal notice.
- **PRV-011** · v1 · **Reasoning controls normalized.** Effort/budget settings mapped per provider, thinking shown collapsed. *Accept:* changing effort changes the request payload for both Anthropic and OpenAI-compatible providers.
- **PRV-012** · v1 · **Native Google Gemini and OpenAI Responses providers.** For features the compatible layer cannot express. *Accept:* Gemini and GPT models can be used with tool calling without the OpenAI-compatible shim.
- **PRV-013** · later · **Out-of-process provider adapters.** For providers with unusual APIs (see EXT-007).

### Harnesses (HRN)

- **HRN-001** · MVP · **One harness interface.** Every harness, built-in or external, exposes the same operations: create session, prompt, cancel, set mode, list capabilities, emit events. The GUI never special-cases a harness. *Accept:* the conversation, approvals and diff panels work unchanged for the built-in harness and for the Claude Code harness.
- **HRN-002** · MVP · **Built-in harness.** An agentic loop in Go with tools `read_file`, `write_file`, `edit_file` (exact string replace, multi-occurrence guard), `list_dir`, `glob`, `grep`, `shell` (through the session's environment, ENV-001), `load_skill` (SKL-004), and MCP tools (MCP-003). Stops on model end-turn, max turns, max tokens or cancel. *Accept:* "add a failing test for X then make it pass" completes against a repo with at least one edit and one shell call, each shown as a tool call in the transcript.
- **HRN-003** · MVP · **Declarative harness profiles.** YAML files defining: display name, harness (built-in or external), model and provider, system prompt template, enabled tools, permission policy (SEC-002), skills, MCP servers, loop limits, instruction-file policy. Profiles live in the app config dir and optionally in a project's `.freeloop/profiles/`. *Accept:* two profiles that differ in model and tool set can run side by side in two sessions; invalid YAML is reported with line and field.
- **HRN-004** · MVP · **Choose harness and profile at session creation.** *Accept:* the new-session dialog lists profiles grouped by harness, with the project's default preselected.
- **HRN-005** · MVP · **Generic ACP client.** Launch an external agent process by command, args, env and cwd; speak Agent Client Protocol over stdio; map initialize, session new/prompt/cancel, tool call updates, permission requests, plan and mode updates into the internal event schema; pass MCP servers (MCP-004); fulfil the agent's file-system and terminal requests through the session's environment. *Accept:* a reference ACP agent (the Gemini CLI or the ACP example agent) completes a prompt that reads and edits a file, with the edit approved through the app's approval UI.
- **HRN-006** · MVP · **Claude Code harness.** Preconfigured external harness that runs the unmodified Claude Code binary through its ACP adapter, with login performed by Claude Code's own flow and the app never reading, storing or forwarding its tokens (see HRN-008). *Accept:* a user signed in to Claude Code with a subscription runs a session from the app; the app's config and database contain no Anthropic OAuth token; permission prompts appear in the approval inbox.
- **HRN-007** · v1 · **Verified presets for other agents.** Gemini CLI (API-key authentication, since Google-account sign-in for individuals ended in mid-2026), Codex CLI, Goose, OpenCode: tested command lines, quirk handling, documented auth method. *Accept:* each preset passes the HRN-005 acceptance scenario.
- **HRN-008** · MVP · **Subscription boundary.** The app never collects, stores, proxies or intermediates vendor OAuth tokens or session cookies; the only subscription usage is through an unmodified vendor binary performing its own sign-in. The built-in harness uses API keys and local endpoints only. *Accept:* a code search finds no OAuth handling for vendor consumer accounts; the Claude Code adapter does not pass credentials in env or args; documentation states the boundary.
- **HRN-009** · MVP · **Plan mode in built-in harness.** Read-only tool set until the user approves the plan. *Accept:* in plan mode a write attempt is refused and recorded; approving switches the session to the chosen mode.
- **HRN-010** · MVP · **Instruction files.** The built-in harness loads `AGENTS.md` from the project root (and nested, when files under that directory are touched); falls back to `CLAUDE.md` read-only when no `AGENTS.md` exists; profile system prompt is prepended. *Accept:* a rule stated only in `AGENTS.md` is visibly followed; the files are never modified by the app.
- **HRN-011** · v1 · **Custom code harnesses.** "New harness" scaffolds an ACP agent from a Python or Go template that already speaks the protocol, registers it, and supports restart-on-change. *Accept:* from the template to a working session in under 15 minutes, following the docs only.
- **HRN-012** · v1 · **Switch harness or profile mid-session.** Implemented as a new session seeded with a handoff summary and the same environment. *Accept:* the new session's first turn can answer a question about work done in the old one.
- **HRN-013** · v1 · **Context management.** Automatic compaction when near the model's context limit; manual compact; visible context meter. MVP behaviour: warn at 80 %, stop at limit. *Accept:* a long session continues past the raw context limit after compaction with the task still progressing.
- **HRN-014** · v1 · **Subagents and parallel tool calls in the built-in harness.** *Accept:* two independent `read_file` calls in one model turn execute concurrently; a subagent's transcript is viewable from the parent.
- **HRN-015** · later · **Harness marketplace or registry.** Discover community harnesses.
- **HRN-016** · v1 · **One task on several profiles, compared.** The new-session form can send one first message to two to four profiles at once, each in its own worktree; a compare view shows each run's status, changed files, lines, tokens and last reply, and keeping one archives the others. *Accept:* a task sent to two profiles makes two worktree sessions with the same message; the compare view shows both runs' changes; "Keep" leaves one active and archives the other with its worktree.

### Skills (SKL)

- **SKL-001** · MVP · **Agent Skills format is native.** A skill is a folder containing `SKILL.md` with YAML frontmatter (`name`, which must equal the folder name, `description`, and the optional `license`, `compatibility`, `metadata` and `allowed-tools` fields per the spec) plus optional `scripts/`, `references/` and `assets/`. *Accept:* an unmodified skill from the user's `~/.claude/skills` loads without changes.
- **SKL-002** · MVP · **Discovery and toggles.** Skills are discovered from the app's skill store, from `.freeloop/skills/` in a project, and (read-only) from `.claude/skills/` and `.agents/skills/` in a project; each can be enabled globally, per profile or per project. *Accept:* the skills panel lists every skill with source, scope and an enable switch; a disabled skill does not appear in the model's skill index.
- **SKL-003** · MVP · **Import from Claude Code.** One-way copy from `~/.claude/skills` with provenance (source path, date) recorded; re-import shows a diff. *Accept:* importing twice does not duplicate; the original files are untouched.
- **SKL-004** · MVP · **Progressive disclosure.** The system prompt lists enabled skills by name and description only; `load_skill` returns the body; referenced files are read on demand; scripts run through the environment and the permission broker like any shell call. *Accept:* the system prompt size grows by less than 100 tokens per enabled skill; a script in a skill triggers a permission prompt unless allowed by rule.
- **SKL-005** · MVP · **Install from folder or zip.** *Accept:* dropping a folder onto the skills panel installs it after a preview of its frontmatter and scripts.
- **SKL-006** · v1 · **Install from git URL and update.** Pin to a commit; "check for updates" shows a diff. *Accept:* install, update and rollback each complete with the skill remaining loadable.
- **SKL-007** · v1 · **Authoring.** Create from template, open in external editor, validate frontmatter against the spec, test-run with a chosen model. *Accept:* validation flags a missing `description`; a test run shows whether the model picked the skill for a sample prompt.
- **SKL-008** · v1 · **Skills for external harnesses.** Best-effort projection of enabled skills into each harness's native discovery location, documented per adapter. *Accept:* an enabled skill is visible to the Claude Code harness in a session started from the app.
- **SKL-009** · later · **Skill registry browsing.** Search and install from public skill registries.

### MCP servers (MCP)

- **MCP-001** · MVP · **Manage servers.** Add stdio (command, args, env, secret refs) and Streamable HTTP (URL, headers) servers; enable per profile or globally; start, stop, restart; show health, negotiated protocol revision, tool list and last error. Targets the MCP 2026-07-28 revision (stateless, `server/discover`) through the official Go SDK, which also speaks the 2025-11-25 and 2025-06-18 revisions for existing servers. *Accept:* a stdio server and a remote HTTP server both show their tools within 5 s of enabling.
- **MCP-002** · MVP · **Import from Claude Code config.** One-way import of `mcpServers` from `~/.claude.json` and a project's `.mcp.json`, previewed before applying. *Accept:* imported servers work without edits; secrets found in env are moved to the Keychain and replaced by references.
- **MCP-003** · MVP · **MCP tools in the built-in harness.** Namespaced `mcp__<server>__<tool>`; tool descriptions passed to the model; results returned as tool results; permission rules apply by namespace. *Accept:* a rule `allow mcp__github__*` auto-approves those tools and nothing else.
- **MCP-004** · MVP · **MCP servers passed to ACP harnesses.** Enabled servers are provided on ACP session creation so external agents use the same servers. *Accept:* a server enabled in a profile is listed by the Claude Code harness in a session started with that profile.
- **MCP-005** · MVP · **Trust on first start.** Before a new server first runs, show the full command or URL and ask for confirmation; warn when the command fetches remote packages (`npx`, `uvx`, `docker run` of an unpinned image). *Accept:* no server starts without one confirmation per server configuration change.
- **MCP-006** · v1 · **OAuth for remote servers.** OAuth 2.1 authorization code flow with PKCE per the MCP authorization spec; client registration through Client ID Metadata Documents with Dynamic Client Registration as a fallback; issuer validation; tokens in the Keychain, one item per server. *Accept:* a remote server requiring OAuth is usable after one browser round trip.
- **MCP-007** · v1 · **Resources and prompts.** Attach a resource to the composer; run a server prompt as a slash command. *Accept:* a resource's content appears in the next user turn; a prompt with arguments renders a form.
- **MCP-008** · v1 · **Input-required round trips.** Under MCP 2026-07-28 a server asks for more input by returning an `input_required` result (the Multi Round-Trip Request pattern that replaced server-initiated elicitation); the request is shown in the approval inbox as a form and the original call is retried with the answers. Older servers' `elicitation/create` requests take the same path. *Accept:* a server that asks for a confirmation blocks until the user answers in the inbox, then completes.
- **MCP-009** · later · **Server registry browsing.** Search and install from the MCP registry.

### Environments (ENV)

- **ENV-001** · MVP · **Environment abstraction.** Every tool execution, file read or write, PTY and process launch goes through an environment interface providing exec, file system, PTY, env vars, secret refs and working directory. *Accept:* the built-in harness has no direct `os/exec` or file-system call outside the environment package (enforced by a lint rule).
- **ENV-002** · MVP · **Local environment.** A folder on this Mac, usually a git checkout; per-environment env vars, secret references, tool allow/deny list, default permission mode, default profile. *Accept:* a secret set on an environment is visible to a shell tool call as an env var and absent from the transcript.
- **ENV-003** · MVP · **Projects and sessions bind to environments.** A project is a repo path with a default environment; a session chooses an environment at creation. Several sessions may share one environment. *Accept:* three sessions on three repos run concurrently, each with its own cwd and env vars.
- **ENV-004** · MVP · **Secret injection.** Secrets are resolved from the Keychain at process start and passed as env vars (or temp files with 0600 mode when a tool needs a file); never written to config or transcript; redacted in logs by value. *Accept:* grepping the config dir and transcripts for a known secret value finds nothing after a session that used it.
- **ENV-005** · v1 · **Docker and devcontainer environment.** Run external harnesses and all tool calls inside a container; reuse `.devcontainer/devcontainer.json` when present; bind-mount the repo; forward ports; lifecycle (create, start, stop, rebuild). *Accept:* the HRN-002 scenario runs with every shell call executing inside the container, verified by `uname -a` output.
- **ENV-006** · v1 · **External harness inside an environment.** ACP over stdio tunnelled through `docker exec` (and later `ssh`). *Accept:* the Claude Code harness runs inside a devcontainer with its own login state.
- **ENV-007** · v1 · **Setup and health.** Post-create script, health check, "environment is unhealthy" state surfaced on the session. *Accept:* a failing health check blocks new sessions with a reason.
- **ENV-008** · later · **SSH remote environment.** Exec and file access over SSH/SFTP.
- **ENV-009** · later · **Sandboxed local execution.** Restricted file-system and network for the shell tool on macOS without Docker.
- **ENV-010** · later · **Network policy per environment.** Allow/deny lists for outbound hosts.

### GUI and UX (GUI)

- **GUI-001** · MVP · **Main window.** Left sidebar of projects, each expanding to sessions with status badges (running, waiting for you, idle, error); center conversation; right panel with tabs Diff, Approvals, Info. *Accept:* every session state is distinguishable in the sidebar without opening it.
- **GUI-002** · MVP · **Attention view.** A list of every session waiting for approval, input or in error, across all projects, with inline approve/deny and one-click jump. *Accept:* with five sessions prompting at once, all five can be resolved from the attention view without opening each session.
- **GUI-003** · MVP · **Conversation rendering.** Streaming markdown, code blocks with copy, collapsible tool calls showing input and output, inline diffs for edits, collapsed thinking, visible turn boundaries and timestamps. *Accept:* a 5,000-event transcript scrolls at 60 fps on an M1 (virtualized list).
- **GUI-004** · MVP · **Composer.** Multiline input, `@path` file mentions with fuzzy picker, slash commands for profile-defined prompts and mode changes, message queueing while the agent runs, cancel. *Accept:* a message typed while the agent runs is sent at the next turn boundary; cancel stops within 1 s.
- **GUI-005** · MVP · **Diff and commit panel.** Changed files versus the session's base commit; per-file diff; stage, commit with message (AI-suggested, editable), discard file. *Accept:* a commit made from the panel appears in `git log` with the chosen message.
- **GUI-006** · MVP · **Approval dialog.** Shows tool, full input, affected paths or command, risk hints; options Allow once, Allow always (creates a rule, scope selectable), Deny with optional reason; keyboard shortcuts. *Accept:* "Allow always" for `shell: git status` creates a visible rule that auto-approves the next identical call.
- **GUI-007** · MVP · **Session header controls.** Harness, profile, model, permission mode (switchable), token totals, environment, git branch and dirty state. *Accept:* changing permission mode takes effect on the next tool call.
- **GUI-008** · MVP · **Notifications.** macOS notification and dock badge when a session needs attention or finishes while the app is unfocused; per-project mute. *Accept:* a notification click opens the right session.
- **GUI-009** · MVP · **Settings UI.** Providers, profiles (form for common fields, YAML editor with validation for the rest), skills, MCP servers, environments, general. *Accept:* every MVP setting can be changed without editing files by hand.
- **GUI-010** · MVP · **First-run onboarding.** Add a provider, test it, optionally import Claude Code skills and MCP servers, add a first project. *Accept:* a fresh install reaches a first agent reply in under 5 minutes with an API key at hand.
- **GUI-011** · MVP · **Appearance.** Follows system light/dark; adjustable font size. *Accept:* switching system appearance updates the app live.
- **GUI-012** · v1 · **Integrated terminal.** PTY per session in the session's environment. *Accept:* `ls` in the terminal shows the session's cwd inside a Docker environment.
- **GUI-013** · v1 · **File tree and viewer.** Read-only browser of the project with open-in-editor. *Accept:* a file changed by the agent is highlighted in the tree.
- **GUI-014** · v1 · **Command palette and keyboard-first navigation.** *Accept:* every sidebar and panel action is reachable from the palette.
- **GUI-015** · v1 · **Multiple windows.** A session can be opened in its own window. *Accept:* two windows stream two sessions simultaneously.
- **GUI-016** · v1 · **Board view.** One card per session as an alternative home screen. *Accept:* every unarchived session the sidebar lists shows as a card in its status's column (needs you, running, idle), and a card opens its session.
- **GUI-017** · v1 · **Workspace of up to four sessions.** The main window shows one, two, three or four sessions side by side, each pane streaming live with its own composer and approvals; the layout and its sessions are kept across restarts. *Accept:* four sessions in four projects stream at once in one window, an approval key acts only in the focused pane, and the layout returns after a restart.
- **GUI-018** · v1 · **Usage dashboard.** Token use and estimated cost over 7 days, 30 days, 90 days or 12 months, by kind of token (input, output, cache write, cache read), model, provider or agent, or project, with or without cache reads, with the sessions that used the most and a CSV export. *Accept:* the dashboard's totals equal the sum of its rows, a range change re-scopes every figure, chart and table, a deleted session's tokens still count, and every value in the chart is also in a table.
- **GUI-019** · v1 · **Review comments on the diff.** A comment on a line of the Diff tab collects with others into a review the user sends to the session as one message, with each comment's file, line and surrounding lines. *Accept:* three comments on two files reach a built-in session and the Claude Code preset as one message naming each file and line; nothing in the review flow depends on the harness.

### Sessions and history (SES)

- **SES-001** · MVP · **Concurrent sessions.** Create a session by choosing project, environment, harness/profile and model; at least ten sessions run concurrently with independent state, streams and approvals. *Accept:* ten sessions each running a shell command finish without cross-talk in transcripts or cwd.
- **SES-002** · MVP · **Append-only JSONL transcript per session.** Every event (EXT-002) is appended and flushed before the UI renders it. *Accept:* killing the app mid-stream loses at most the in-flight event.
- **SES-003** · MVP · **Restore on restart.** Sessions reopen in an idle state with full history; external harness sessions resume via ACP session load where the agent supports it, otherwise a new agent session is seeded with a summary. *Accept:* after restart, a continued prompt in a built-in session has the prior turns in context.
- **SES-004** · MVP · **List, filter, auto-title.** Filter by project, status, harness, date; titles generated from the first exchange, editable. *Accept:* a session is findable by its title within one filter step.
- **SES-005** · MVP · **Archive and delete.** Archive hides; delete removes the transcript and database rows after confirmation. *Accept:* a deleted session's files are gone from disk.
- **SES-006** · MVP · **Git state per session.** Branch, HEAD, dirty file count shown live; base commit recorded at session start for the diff panel. *Accept:* the diff panel still shows the session's changes after an unrelated commit on the same branch.
- **SES-007** · v1 · **Full-text search across sessions.** *Accept:* a phrase from a tool result three weeks old is found in under 1 s.
- **SES-008** · v1 · **Export.** Markdown and JSON export of a session. *Accept:* the Markdown export renders tool calls and diffs legibly.
- **SES-009** · v1 · **Fork from a message.** *Accept:* the fork has the history up to that message and a new transcript file.
- **SES-010** · v1 · **Worktree per session (opt-in).** Create a worktree and branch at session start, clean up on archive, merge-back or PR helper. *Accept:* two sessions on the same repo edit the same file without interfering.
- **SES-011** · v1 · **Pull request creation.** Through the GitHub CLI with an AI-drafted, editable description. *Accept:* the PR URL appears in the session after creation.
- **SES-012** · v1 · **Scheduled and recurring sessions.** A schedule starts a new session in a project with a prompt hourly, daily, on weekdays or weekly while the app runs; each run goes through the permission broker like any session. *Accept:* a due schedule starts one session tagged `scheduled` with its prompt; a run missed while the app was closed runs once when it opens; a run that needs an approval waits for it.
- **SES-013** · v1 · **Tags and saved filters.** Sessions carry tags; the sidebar filters by tag (from a menu or `#tag` in the search box) and keeps named filters. *Accept:* a tag added in one session shows in the sidebar at once, filtering by it lists only tagged sessions, and a saved filter returns after a restart.
- **SES-014** · v1 · **Checkpoints and rewind.** Before a turn changes files, the session's working tree is snapshotted; any user message offers "rewind to here", which restores the files to that point and continues the conversation from it, the later turns kept in the transcript as rewound. *Accept:* after rewinding past an edit, the file is back to its earlier content and the next turn's model context ends at the chosen message.
- **SES-015** · v1 · **Images in prompts.** Paste or drop up to ten images into the composer; they are scaled down in the app, kept with the session and sent to models and agents that accept images. *Accept:* a pasted screenshot reaches an Anthropic, OpenAI and Gemini model and the Claude Code preset; the transcript shows it after a restart; a fork keeps it; an external agent that declares no image support refuses before anything is sent, saying so, and a model that rejects images fails the turn with the provider's reason.
- **SES-016** · v1 · **Worktree setup and run scripts.** A project names files to copy into each new worktree, a setup command run once when the worktree is made, a run command (such as a dev server) and an archive command; each worktree session gets its own block of ports. *Accept:* a new worktree session of a trusted project has its `.env` copied and `npm install` run before its first prompt starts, with the output visible; the run command starts in a terminal tab with `FREELOOP_PORT` set; two sessions get different ports; an untrusted project runs nothing.
- **SES-017** · v1 · **Start from a GitHub issue.** The new-session form lists a trusted project's open GitHub issues through the GitHub CLI; choosing one titles the session after it, tags it, and puts the issue's text, comments and link into the first message for the user to edit. *Accept:* choosing issue #12 makes a session titled "#12 …" tagged `issue-12` whose message box holds the issue's body and comments; nothing is sent until the user sends it; local-only mode and an untrusted project list nothing.
- **SES-018** · v1 · **Pull request checks.** After a pull request is opened from a session, the Diff tab shows its CI checks and refreshes them while any run; "Send failures to the agent" puts the failing checks' names, links and the end of their logs into the message box. *Accept:* a failing check shows as failed with its link; the message names it and quotes its log's end; local-only mode checks nothing.

### Permissions and security (SEC)

- **SEC-001** · MVP · **Permission modes per session.** `ask` (default), `accept-edits` (file edits inside the project auto-approved), `plan` (read-only), `bypass` (everything auto-approved). Bypass must be enabled once in settings, shows a persistent banner, and resets to `ask` on new sessions. *Accept:* each mode's behaviour matches this table for a write, a shell call and an MCP call.
- **SEC-002** · MVP · **Rule engine.** Allow and deny rules by tool and pattern: shell command prefix (`shell(git *)`), path glob for file tools, MCP namespace, skill name. Scopes: global, environment, profile, project (`.freeloop/`), session. Deny always wins; more specific scope wins among allows. *Accept:* a project deny rule overrides a global allow for the same command; rules are listed with their scope and source file.
- **SEC-003** · MVP · **Single permission broker.** Every permission request from any harness passes through one broker that applies mode and rules, then queues the rest for the user in the attention view. No request is auto-approved by timeout. *Accept:* an ACP agent's permission request and a built-in tool's request look identical in the inbox and in the transcript.
- **SEC-004** · MVP · **Secret storage.** API keys, tokens and environment secrets are stored in the macOS Keychain; config files hold references only; the UI never shows a stored secret after saving; known secret values are redacted from logs and transcripts. *Accept:* config files contain no key material; a secret pasted into chat is redacted in the stored transcript.
- **SEC-005** · MVP · **Protected paths.** By default, writes outside the project root, inside `.git/`, and inside the app's own config directory are denied regardless of mode. Reads are covered by SEC-013. *Accept:* in bypass mode, writing to `~/.ssh/config` is still refused and recorded.
- **SEC-006** · MVP · **Project trust.** First use of a project asks for trust before reading `AGENTS.md`/`CLAUDE.md`, `.freeloop/` config, project skills or project MCP servers. *Accept:* an untrusted project runs with global config only and says so.
- **SEC-007** · MVP · **Audit trail.** Every tool execution, permission decision and mode change is in the transcript with timestamp and actor (user, rule, mode). *Accept:* for any file change the transcript shows which call made it and how it was approved.
- **SEC-008** · MVP · **Process isolation for external harnesses and MCP servers.** Separate processes; environment scrubbed to the configured variables plus a minimal base; stdio only. *Accept:* `env` printed from inside an external harness shows no app secrets other than those configured for its environment.
- **SEC-009** · MVP · **No undeclared network.** Outbound connections only to configured providers, MCP servers and harness endpoints; no telemetry; update checks opt-in. *Accept:* with no providers configured, a network monitor shows zero outbound connections from the app over 10 minutes.
- **SEC-010** · MVP · **Untrusted tool output.** The built-in harness frames tool results and fetched content as data; instructions found there never change permission state or trigger actions without the usual approval. *Accept:* an `AGENTS.md`-style instruction planted in a file read by the agent cannot cause an auto-approved write.
- **SEC-011** · v1 · **Sandboxed execution.** Through the Docker environment (ENV-005); native macOS sandboxing later (ENV-009). *Accept:* per ENV-005.
- **SEC-012** · v1 · **Signed and notarized builds, pinned dependencies, SBOM.** *Accept:* Gatekeeper opens the downloaded app without warnings; the release includes an SBOM.
- **SEC-013** · MVP · **Reads outside the project root.** File reads, directory listings and searches outside the session's project root require approval in every mode except `bypass`; a fixed list of never-readable locations (`~/.ssh`, `~/.gnupg`, `~/.aws`, browser profiles, the Keychain database directory, the app's own config directory) is refused in every mode including `bypass`. *Accept:* in `accept-edits` mode a `read_file` of `~/.zshrc` prompts; a read of `~/.ssh/id_ed25519` is refused and recorded even in `bypass`.

### Settings (SET)

- **SET-001** · MVP · **Human-readable config.** YAML files under `~/Library/Application Support/freeloop/`, editable by hand, validated and hot-reloaded. *Accept:* editing a profile file on disk updates the UI within 2 s; a validation error is shown without crashing.
- **SET-002** · MVP · **Project-level overrides.** `.freeloop/` in a repo may contain profiles, environment defaults, MCP servers, skills and rules; applied after trust (SEC-006). *Accept:* a project profile appears in the new-session dialog for that project only.
- **SET-003** · MVP · **Import from Claude Code.** Permission rules from `settings.json`, MCP servers (MCP-002) and skills (SKL-003), previewed, one-way. *Accept:* the preview lists each item with its destination; nothing is written back to `~/.claude`.
- **SET-004** · MVP · **Defaults.** Default provider and model, profile, permission mode, notification preferences, editor to open files in. *Accept:* a new session uses the defaults without extra clicks.
- **SET-005** · v1 · **Backup and export of config without secrets.** *Accept:* restoring the export on a fresh install recreates all providers (prompting for keys) and profiles.
- **SET-006** · later · **Keyboard shortcut customization.**

### Extensibility (EXT)

- **EXT-001** · MVP · **File- and process-based extension points only.** Profiles, skills, MCP servers, providers, environments and harnesses are configured through files and processes; no in-process plugin loading. *Accept:* every extension type in this document can be added without rebuilding the app.
- **EXT-002** · MVP · **Documented event schema.** A versioned JSON schema for session events (message deltas, thinking, tool call lifecycle, permission request and decision, plan, mode, usage, error, end). Transcripts use it; the GUI consumes it; external harness events are mapped to it. *Accept:* `schemas/events.json` validates every transcript produced by the test suite.
- **EXT-003** · v1 · **Harness SDK templates.** Python and Go templates that speak ACP, with a test harness. *Accept:* per HRN-011.
- **EXT-004** · v1 · **Headless control.** Local JSON-RPC API over a Unix socket and a small CLI (`freeloop sessions list`, `freeloop prompt <id> "..."`). *Accept:* a session created from the CLI appears in the GUI live.
- **EXT-005** · v1 · **Hooks.** Shell hooks on session start, before and after tool use, and on permission request, per profile or project, with the event as JSON on stdin and a documented exit-code contract. *Accept:* a pre-tool hook can block a shell command with a message shown in the transcript.
- **EXT-006** · later · **UI plugins.**
- **EXT-007** · later · **Out-of-process provider adapters.**

## Non-functional requirements

### Performance (NFR-PERF)

- **NFR-PERF-001** · MVP · UI remains interactive (input latency under 100 ms) with ten sessions streaming concurrently on an M1 with 16 GB.
- **NFR-PERF-002** · MVP · Opening a 5,000-event transcript renders in under 1 s; scrolling is virtualized.
- **NFR-PERF-003** · MVP · Cold start to usable window under 2 s; restoring 50 archived and 10 active sessions under 3 s.
- **NFR-PERF-004** · v1 · Idle memory under 300 MB for the app with ten idle sessions, excluding external harness processes.
- **NFR-PERF-005** · MVP · Streaming tokens appear within 50 ms of arrival from the provider.

### Security (NFR-SEC)

- **NFR-SEC-001** · MVP · Secrets only in the Keychain (SEC-004); no secret material in logs, crash reports or exports.
- **NFR-SEC-002** · MVP · Every side-effecting action is attributable in the transcript (SEC-007).
- **NFR-SEC-003** · MVP · Threat model documented in `docs/SECURITY.md` before the first public release: prompt injection, malicious skills, malicious MCP servers, hostile repos, local attacker with the same user account.
- **NFR-SEC-004** · v1 · Builds are reproducible from a tagged commit, signed and notarized; dependencies pinned with checksums.
- **NFR-SEC-005** · v1 · Sandboxing available for shell execution (Docker environment); roadmap to native sandbox.

### Privacy (NFR-PRIV)

- **NFR-PRIV-001** · MVP · No telemetry, analytics or crash reporting leaves the machine. Diagnostics are local files the user can share manually.
- **NFR-PRIV-002** · MVP · Transcripts, config and indexes are stored locally only; export is an explicit user action.
- **NFR-PRIV-003** · MVP · The app sends to a provider only what the user can see in the transcript plus the system prompt, tool definitions and skill index, which are inspectable.

### Offline and local-only operation (NFR-OFF)

- **NFR-OFF-001** · MVP · With a local model provider and local MCP servers, every MVP feature works with networking disabled, except features that are inherently remote (remote MCP servers, git push, PR creation).
- **NFR-OFF-002** · MVP · No startup network dependency; no feature degrades because an update check or remote asset failed.
- **NFR-OFF-003** · v1 · A "local-only" switch blocks all non-loopback outbound connections from the app and reports attempts.

### Cross-platform (NFR-XPLAT)

- **NFR-XPLAT-001** · MVP · Supported: macOS 14 and newer, Apple Silicon and Intel.
- **NFR-XPLAT-002** · MVP · No macOS-only code outside a platform package; Keychain, notifications and file dialogs are behind interfaces so Linux and Windows backends can be added without touching the engine.
- **NFR-XPLAT-003** · later · Linux build; Windows build with WSL awareness.

### Accessibility (NFR-A11Y)

- **NFR-A11Y-001** · MVP · Full keyboard operation of conversation, approvals and session switching; visible focus.
- **NFR-A11Y-002** · MVP · Labels for all controls, announced status changes for approvals, contrast at WCAG AA, respects reduced motion and system text size.
- **NFR-A11Y-003** · v1 · VoiceOver pass of all MVP screens with issues fixed.

### Reliability and maintainability (NFR-REL)

- **NFR-REL-001** · MVP · A GUI crash never loses transcript events already received (SES-002); sessions recover on relaunch.
- **NFR-REL-002** · MVP · An external harness or MCP server crash is isolated to its session and shown as an error with a restart action.
- **NFR-REL-003** · MVP · Engine packages have unit tests; the provider and ACP layers have replayable fixtures; CI runs on a macOS runner on every commit.
- **NFR-REL-004** · v1 · Contributor documentation: architecture overview, how to add a provider, a harness preset, an environment.

## Cut from MVP, and why

- Docker, SSH and sandboxed environments (ENV-005 to ENV-010): the local folder covers the daily workflow; environments are the largest matrix of edge cases and would double the MVP.
- Verified presets for Gemini CLI, Codex CLI, Goose, OpenCode (HRN-007): the generic ACP client ships in MVP, so these are configuration plus quirk handling, best done once real users ask.
- Custom code harnesses and the SDK (HRN-011, EXT-003): declarative profiles cover the "switch harnesses" need for the first months; the SDK is cheap once ACP is stable in the app.
- Worktree per session and PR creation (SES-010, SES-011): the primary workflow is parallel sessions across repos, not within one repo.
- Integrated terminal, file tree, command palette, multi-window (GUI-012 to GUI-015): nice to have; each is a week of polish.
- Full-text search, export, fork (SES-007 to SES-009): list and filter suffice for the first release.
- MCP OAuth, resources, prompts, elicitation (MCP-006 to MCP-008): the tool path covers most servers.
- Cost estimates, caching, retries, reasoning controls, native Gemini/Responses providers (PRV-008 to PRV-012): the compatible layer gets every model working; optimizations follow.
