# Security

Threat model and security policy for **Freeloop**. This document satisfies NFR-SEC-003 in [REQUIREMENTS.md](REQUIREMENTS.md) and is referenced by [RISKS.md](RISKS.md). It describes what the app protects, against whom, how, and what it does not protect against. Requirement IDs point to the controls.

## Reporting a vulnerability

Until the first public release, open a private security advisory on the GitHub repository. Before the MVP ships this section will name a contact address and a response target.

## What the app is

A desktop application that runs AI agents with the user's privileges on the user's machine. Agents read and write files, run shell commands, call tools provided by third-party MCP servers, and send project content to model providers chosen by the user. The agent's instructions come partly from the user and partly from content the agent reads, which may be hostile. That combination is the central security problem.

## Assets

| Asset | Where it lives | Why it matters |
|---|---|---|
| Provider API keys, MCP OAuth tokens, environment secrets | macOS Keychain only (SEC-004) | Direct financial and data loss if leaked |
| Project source and data | Project directories chosen by the user | Integrity: unwanted changes; confidentiality: sent to a provider or exfiltrated |
| The user's shell and home directory | The local environment runs as the user | Everything the user can do, an agent in `bypass` mode can do |
| Transcripts | `sessions/<id>/events.jsonl` | Contain code, tool output, and anything the user pasted |
| Configuration: rules, profiles, environments | YAML in the app support directory and `.freeloop/` in projects | Tampering with rules turns asking into auto-approval |
| Vendor sign-in state of external harnesses | Owned by the vendor binary, never by the app (HRN-008) | Both a legal and a security boundary |

## Trust boundaries

1. **User to app.** Trusted. The user's decisions in approval dialogs are authoritative.
2. **App to model provider.** The transport is trusted (TLS, the user's chosen endpoint). The model's output is **untrusted**: it is a proposal, not a command.
3. **App to external harness process.** Spawned by the app, speaks ACP over stdio. Treated as untrusted code: every file-system and terminal request it makes passes through the same environment and permission layers as the built-in tools (ARCHITECTURE.md section 12).
4. **App to MCP servers.** Third-party code and third-party data. Tool descriptions and results are untrusted content; the process is confined only by the environment it runs in.
5. **App to project content.** Untrusted until the user trusts the project (SEC-006), and the *content* stays untrusted even then: dependencies, fixtures, issues and documentation are all injection vectors.
6. **App to skills.** Installed deliberately by the user, so semi-trusted, but a skill's scripts are code and run under the same approvals as any shell command.
7. **Environment boundary.** The local environment offers no isolation beyond the user account. A Docker environment (v1) adds a container boundary. The policy layer never assumes isolation it was not told about.

## Adversaries

- **A1 Remote author of content the agent reads.** Plants instructions in a repository file, a web page, a dependency README, an issue, or an MCP tool result, hoping the model follows them. Most likely and most important.
- **A2 Author of a malicious extension.** Publishes an MCP server, a skill, or a harness preset that behaves well until it does not: poisoned tool descriptions, exfiltrating tool arguments, scripts that read beyond their purpose.
- **A3 A misbehaving model or provider.** Rare, but identical in effect to A1 and covered by the same controls.
- **A4 Local attacker with the same user account.** Another process or person who can read the user's files. Defended only by file permissions and the Keychain.
- **A5 The user's own haste.** Bypass mode on an untrusted repo; a rule like `allow shell(*)`; a secret pasted into a prompt.

Out of scope: an attacker with root or kernel access, physical access to an unlocked machine, a compromised macOS, or a compromised provider account on the vendor side.

## Scenarios and controls

| # | Scenario | Impact | Controls |
|---|---|---|---|
| 1 | A file the agent reads says "run `curl … \| sh`" | Arbitrary code execution | Execution needs approval in `ask` and `accept-edits` (SEC-001); deny rules win (SEC-002); tool output framed as untrusted (SEC-010); container environments (ENV-005); the native sandbox (ENV-009) |
| 2 | Injected instruction to read `~/.ssh/id_ed25519` and post it | Credential theft | Never-readable locations refused in every mode; reads outside the project root prompt (SEC-013); fetch and shell approvals; network policy later (ENV-010) |
| 3 | Injected instruction to write to `~/.zshrc` or `.git/hooks` | Persistence, code execution on next shell or commit | Protected paths denied in every mode (SEC-005) |
| 4 | An external ACP agent uses `fs/write_text_file` on a path outside the project | Bypasses tool-level checks | Environment file-system layer enforces policy for protocol requests too (ARCHITECTURE.md section 12; RISKS S-09) |
| 5 | MCP tool description tells the model to call another tool with secrets | Exfiltration | Descriptions visible in UI (MCP-001); namespace rules so one server cannot be blanket-allowed by accident (MCP-003); trust on first start (MCP-005) |
| 6 | `npx some-mcp-server@latest` pulls a compromised package | Code execution as the user | Warning on unpinned remote packages at first start (MCP-005); recommendation to pin versions; environment confinement at v1 |
| 7 | A skill's script reads and uploads the repo | Data loss | Preview of scripts at install (SKL-005); scripts run through the permission broker (SKL-004); no automatic updates (SKL-006) |
| 8 | A cloned repo ships `.freeloop/rules.yaml` with `allow shell(*)` and a hook | Silent auto-approval, code execution on open | Project configuration is inert until trusted (SEC-006); project-scope allow rules can never override global deny rules (SEC-002) |
| 9 | A secret is pasted into the prompt or printed by a command | Secret in transcript on disk and sent to the provider | Redaction of known secret values before persistence (SEC-004); nothing stops an unknown secret, see limitations |
| 10 | A process with the user's privileges edits `rules.yaml` | Policy bypass | Config directory is write-protected from tools (SEC-005) and `0600`; no stronger defence against A4 |
| 11 | Bypass mode on a hostile repo | Everything in rows 1 to 3 at once | Opt-in in settings, persistent banner, resets per session, unavailable in untrusted projects (SEC-001) |
| 12 | Runaway loop burns API credit | Financial | Max turns and tokens per session (HRN-002); per-provider concurrency cap; cost display (PRV-008) |
| 13 | A compromised dependency of Freeloop itself | Everything | Modules and npm packages pinned by checksum, CI's actions by commit, a budget for shipped npm packages (a test holds these); an SBOM with each release and builds anyone can reproduce (`make verify-repro`); minimal npm surface. Signed and notarized builds (SEC-012) wait on OQ-009, and until then a download is only as good as where it came from |
| 14 | The app leaks vendor sign-in tokens | Legal exposure and account compromise | The app never reads, stores or forwards them; vendor binaries own their login (HRN-008) |
| 15 | An MCP server's input request asks for a password or a key, or links to a phishing page | Credential theft | The form always asks the user, in every mode and whatever the rules (MCP-008); it names the server, shows its message and any link in full as plain text, and never fills itself in; the answers go only to the server that asked and are not written to the transcript |
| 16 | A remote MCP server's OAuth metadata points at a service on the user's network, or its sign-in page is a `file:` or app URL | Requests from the app to internal services; a local file or app opened | The sign-in's requests refuse private and loopback addresses unless the server itself is on one; only `https` (or loopback `http`) sign-in pages open, and only after the user chose "Sign in…" (MCP-006) |

## Design rules that follow

- **One chokepoint.** Every side effect from any harness passes through the permission broker (SEC-003). Nothing is auto-approved on timeout.
- **Policy below the protocol.** Path and execution policy is enforced in the environment layer, so neither a tool, a skill, nor an external agent's protocol request can route around it.
- **Deny wins.** A deny rule at any scope beats an allow at any scope. Project-level configuration can tighten but never loosen.
- **Secrets never touch disk.** Keychain references in config; values resolved at spawn; child processes receive only configured variables (SEC-008).
- **Untrusted by default.** Projects, tool output, model output and MCP content are untrusted until a human says otherwise, and content stays untrusted after the project is trusted.
- **Readable state.** Transcripts are the audit log (SEC-007): every change traces to a call and a decision with its actor.
- **No undeclared network.** Only user-configured endpoints; no telemetry (SEC-009).

## Defaults

| Setting | Default |
|---|---|
| Permission mode for a new session | `ask` |
| Bypass mode | Disabled until enabled in settings; never in untrusted projects |
| Writes outside project root, into `.git/`, into the project's `.freeloop/`, into app config | Denied in every mode |
| Reads outside project root | Prompt, except never-readable locations which are refused |
| New MCP server | Does not start until confirmed with its full command shown |
| Project configuration | Not read until the project is trusted |
| Outbound connections | Providers, MCP servers and harness endpoints the user configured; update check opt-in (off by default; once a day to GitHub's API, sending nothing about the app) |

## Limitations the user should know

- The local environment is **not a sandbox**. In `bypass` mode an agent can do anything the user can. Use a Docker environment for repositories you do not trust: its commands and external agents run in a container that sees the project folder and the image, not the rest of your files. The container still has network access and runs as the image's user (often root), and it can change anything in the project folder, which is shared with the host, except what the host reads and acts on: the project's `.freeloop/` (hooks, rules, profiles; created empty when missing so the container cannot create it) and the devcontainer files are mounted read-only. Values from `devcontainer.json` go into the container on the docker command line and never into the environment of the docker CLI on the host, and its Dockerfile and build context must resolve inside the project. The repository's `.git/config`, `config.worktree` and `hooks/` are read-only in the container too, so a command there cannot plant a hook or a filter in them (a worktree session also gets the main checkout's `.git`, which its worktree points into, under the same rule). The rest of `.git` stays writable, because git in the container needs it, and that is a limit: **your own git on the Mac, in a terminal, runs what a container planted through `.git/commondir`, or in a `.git` it created in a folder that had none.** Treat such a checkout as untrusted outside the app. The app's own git refuses a moved common directory. A read-only path that is a symbolic link stops the container from starting, since Docker would mount what it points to. A worktree session's setup, run and archive commands run in the container, never on the Mac, where they would run with the environment's secrets whatever the container wrote into the worktree; should the session's environment later stop being Docker, they refuse to run on the Mac in a worktree a container has had. Only `copy` happens on the Mac, from the main checkout into the worktree, through the same write guards as the agent's file tools. Stopping a command ends it inside the container, children included, not just the docker client on the host. The app's git commands on the host refuse to scan or stage in a repository whose own configuration, `config.worktree` and includes counted, names content filters, signing programs, config-based hooks, merge drivers, a promisor remote or protocol settings, since a command in the container can write `.git`. Checkpoints leave nested repositories out and the app's status and diff ask nothing of them, because git would start in one under that repository's own config.
- **The native sandbox holds shell commands, not everything.** In a sandboxed environment (ENV-009, Seatbelt through `sandbox-exec`) the built-in agent's `shell` tool and an external agent's `terminal/*` requests, the setup, run and archive commands, and hooks run confined: they write only in the project, its caches and temp files; home is closed except toolchains and shell startup files; `.git/config`, git hooks, `.freeloop/` and other files tools act on later are read-only; deny rules on paths hold; only this Mac's loopback is reachable, or nothing, or for the user's own commands the internet. So such commands need no approval, and leaving the sandbox asks every time and cannot be allowed always. If the sandbox will not apply, or a self-test finds it lets a write through, nothing runs. What it does not cover: an external agent's **own** tools, which run in its process (so its requests still ask, and a Docker environment is the answer for agents you do not trust); MCP servers; what you type in the terminal; the file tools, which keep their own guards and approvals. It also has limits inside: loopback reaches anything else listening on your Mac, `/tmp` is shared, reads outside home are open, and a command can still write a Makefile that you later run yourself. Apple marks `sandbox-exec` deprecated with nothing to replace it for this use.
- **The integrated terminal is yours, unguarded.** What you type in a session's Terminal tab runs as you, in the session's environment, with no approval and no protected paths, like any terminal window; it is not recorded or shown to the model. In a Docker session it is a shell in the container.
- **Unknown secrets are not redacted.** Redaction works on values the app knows (stored secrets). A password typed into a prompt is sent to the provider and stored in the transcript.
- **Approval fatigue is real.** Rules exist to make trusted operations quiet so that prompts stay meaningful. Review rules periodically; the settings UI lists every rule with its source.
- **Transcripts are plaintext** on disk, protected only by file permissions.
- **MCP servers run with the user's privileges** in the local environment. Prefer servers you can read, pin their versions, and give remote servers the least access their OAuth scopes allow. A server starts only after you approve its exact configuration, and only with the base environment plus the variables and secrets you gave it; but once running it can do anything you can, and what its tools return reaches the model as data. Every MCP tool call asks unless a rule allows it, whatever read-only annotations the server sends.
- **Provider trust.** Everything in a session's context goes to the configured provider. Local models keep it on the machine; cloud providers' data handling is governed by their terms. That includes a secret a command prints: environment secrets are redacted from the transcript, not from the model's context.
- **Path rules cover file tools, not the shell.** Protected paths, the read-outside-root prompt and path rules apply to `read_file`, `write_file`, `edit_file`, `list_dir`, `glob` and `grep`. An approved shell command runs with the user's privileges and can read or write anywhere, including protected locations, unless the environment is sandboxed (ENV-009). Shell deny rules see through common wrappers (variable assignments, `env`, `command`, `exec`, subshells, `sh -c '…'`) and git's global options (`git -C dir push`), but a shell can always spell a command another way, so treat them as a guard against mistakes.
- **A mentioned file goes to the model without an approval.** Writing `@path` in a message attaches that file's contents, as pasting it would. The engine reads it through the environment, so protected paths stay refused, and only inside the project root (at most 20 files, 256 KB each, 1 MB per message); it is the user's choice of context, not a harness side effect, so no permission request is made. A deny rule on the file holds, because a message can carry text the user did not write, such as an issue's. Text the user did not type at all (a subagent's task, a review's diff, an MCP prompt) attaches nothing.
- **Project trust is one decision per folder.** Until a project is trusted (SEC-006) the app reads none of its instruction files or `.freeloop/` settings, except the hook commands it defines, which the trust prompt lists so the user sees what trusting would run, and runs only git plumbing that its configuration cannot extend (`rev-parse`, `symbolic-ref`, `rev-list`, `config --get`). Once trusted, `git status` runs with `core.fsmonitor` and hooks off and is skipped where the repository configures content filters. Trust is not re-asked when the project's files change; distrust it from the project's banner or the new-session form if its contents change hands. A project's `.freeloop/` can only tighten: allow rules and modes looser than ask are refused, so a hostile repository cannot approve its own commands.
- **Worktree commands are the user's too.** Creating, removing and merging a session's worktree (SES-010) run git in the project's main checkout without a permission request, under the same guards as the diff panel; a merge happens only on your click, never by the agent, and skips hooks.
- **The diff panel's git actions are the user's, not the agent's.** Stage, commit and discard (GUI-005) run git through the session's environment without a permission request, because the user clicks them. They run with hooks and `core.fsmonitor` off, without external diff or textconv drivers, only on files the panel lists, and not at all in a repository whose local config (with includes) sets content filters, a signing program or config-based hooks (`filter.*`, `gpg.*`, `hook.*`), whose `core.hooksPath` setting would not cover the last. Commits from the panel therefore skip `pre-commit` and other hooks. Discard asks for a second click; a discarded new file is deleted. "Suggest message" sends the staged diff (at most 40 KB) to the session's provider.
- **Hooks are the user's commands and can only say no.** Hooks (EXT-005) run without a permission request, like the diff panel's git actions, because the user configured them; a project's `.freeloop/hooks.yaml` runs only once the project is trusted, and the new-session form lists its commands. A hook can block a tool call or a permission request, never approve one, and it runs only for calls the path policy and the deny rules let through, so it can neither widen what an agent may do nor be triggered by a call that is denied anyway.
- **MCP resources and prompts enter only by the user's hand.** A resource is read and a prompt rendered only when the user attaches or runs it (MCP-007), from a server the session uses and the user approved; the server's text becomes part of the user's message, shown in the conversation as sent. Like any context it can carry injected instructions, which the permission broker answers as it does for everything else the agent then tries.

- **A skill is instructions the model will follow.** Installing a skill, or trusting a project that ships skills, puts its description in every session's system prompt and its body in the context when the model loads it. Skills are never run on install: their scripts run only when the model calls the shell tool, which asks like any command (or matches your rules). Read a skill from a stranger before installing it; the preview lists its scripts.

- **An external agent decides what it asks about.** Freeloop routes every permission request an ACP agent makes through the broker, and serves its file and terminal requests through the environment, asking for any it was not just allowed. But an agent such as Claude Code runs its own tools inside its own process: what it does without asking, under its own settings (for example allow rules in `~/.claude/settings.json` or its bypass mode), is outside Freeloop's broker. The app mirrors its permission mode into the agent where the preset maps it, and says so when a preset has no plan mode to mirror; a review by such an agent is refused, since nothing would keep it read-only. An agent's report of its own mode can tighten the session's and loosen it only after the user approved its `switch_mode` request, never to bypass: the mode decides what the broker lets through, so an agent must not be able to choose it. One that switches itself to a mode its preset does not name is told the session's mode again. The presets for other agents (HRN-007) map every mode but plan to the agent's mode that asks, because several default to approving by themselves: goose's `auto`, Codex's `agent`, and OpenCode, which allows everything until `OPENCODE_PERMISSION` says otherwise. Plan means each agent's own idea of read-only: Gemini CLI's and OpenCode's plan, Codex's read-only sandbox, and goose's chat, which has no tools at all. `make verify-agents` checks all of this against the pinned versions, and a newer version of an agent can change it. The agent gets only the base environment plus the environment's variables and the session's MCP servers; the app never passes it vendor credentials (HRN-008).

- **The headless API is the user's.** The command line talks to the running app over a Unix socket in the data folder, which only the user can enter (or in their private `$TMPDIR`), so any process running as the user can start sessions and answer approvals through it, as it could by driving the app. It adds no new power: everything goes through the engine and the permission broker.
- **Local-only mode covers the app's own connections.** With `network.localOnly` on (NFR-OFF-003) every connection through the app's HTTP stack (providers, their SDKs, remote MCP servers) may reach only this Mac: loopback addresses and `localhost` names; other host names are refused before any DNS lookup, proxies are bypassed, and each attempt is listed in Settings. Containers created while it is on get no network. Processes the app starts on the Mac (commands, external agents, stdio MCP servers) run as the user and are not covered; run them in a Docker environment for that.

### Hardening review, 2026-10-29

An independent review of M1 slices 1 to 4 found and reproduced these, all fixed with regression tests: protected paths compared case-sensitively on a case-insensitive file system (`.GIT`, `~/.SSH`); `glob`/`grep` and reads following symlinks past the broker; unbounded reads of non-regular files; "Allow always" saving literal input as a pattern (`rm -rf *`, a file named `**`); `Exec` hanging on a descendant that left the process group; a malformed `rules.yaml` silently dropping deny rules and being overwritten; a crash leaving an unanswered tool call that broke the session; an invalid tool input breaking every later Anthropic request; secrets streamed across chunks escaping redaction; stored keys not redacted until used; `core.fsmonitor` and content filters running through the app's git calls; commands inheriting the app's whole environment; exact rules collapsing newlines; a cancel race in the prompt queue.

### Hardening review, 2026-10-31

Three reviews (engine lifecycle, security boundaries, malformed input). The fixes that bear on this document: a write through a symlink whose target does not exist is refused and files open with `O_NOFOLLOW`; an external agent's reused approval covers only its own kind of action, once; prefix allow rules approve plain words only, closing zsh's `=(...)`, glob qualifiers and `${(e)...}`; path rules ignore case and searches leave out what a deny rule hides; "Allow always" never widens a call it cannot narrow to a tool-wide rule. `docs/ROADMAP.md` part 8 lists the rest.

### Engine calls from a framed page, 2026-11-01

Every engine method is reachable with a POST to `/wails/runtime`, and Wails checks where such a request comes from only in its server build; in the desktop build the check is `return true`. The app preview (GUI-020) puts a page the agent may have written into a frame in the app's own WebKit, so this became reachable. Tried against the packaged app with a hostile page in the frame:

- A `no-cors` fetch reaches the app's URL scheme **with its origin stripped**, and without a guard it ran: the page created a session. Origin checks alone do not stop it. A page cannot set a custom header on such a request, so `x-wails-client-id`, which the app's own calls always carry, is required.
- A fetch with custom headers gets no preflight on the custom scheme and arrives with the header, but keeps its origin, so a foreign origin is refused. Both checks are needed; each stops what the other misses.
- A form post keeps its origin and has no header. `sendBeacon` refuses the scheme. Reading the parent's page is blocked, and the frame is sandboxed against popups and navigating the window.

- A page also chooses the **host** of what it sends to the custom scheme, and Wails skips its host check for `wails://`: `fetch("wails://localhost:41000/wails/runtime")` from `http://localhost:41000` arrives with both set to the same value. An earlier version of the guard compared them and let this through, which a second review found and the packaged app confirmed. The desktop build now takes no http origin at all; only the server build, which listens on a real address, compares them.

`netguard.RuntimeCallAllowed`, in a middleware ahead of Wails' handler in `cmd/freeloop/main.go`, enforces this. **Rerun that test after every Wails upgrade**: it depends on how WebKit delivers requests to a custom scheme, which the unit test can only describe, and the server build used for browser checks has Wails' own check and hides the problem. What stays reachable from a frame is `window.webkit.messageHandlers.external`, which Wails exposes to every frame: it can start a window drag or resize, nothing more, since `wails:event:emit` is off unless a window opts in and Freeloop's do not.

### Hardening review, 2026-11-01

Two reviews, one of the code since the last and one of older surfaces no review had covered; each finding was reproduced before it was fixed unless noted. Regression tests are in `internal/engine/security_test.go` and next to the code.

- **Engine calls.** The host comparison above let a framed page reach every engine method. Fixed and rerun in the packaged app: the page's calls are refused and the app's own pass.
- **Agent-chosen mode.** A `current_mode_update` naming `bypassPermissions` put a plan-mode session in bypass, skipping `allowBypass`, after which the broker allowed every `fs/*` and `terminal/*` request.
- **Nested repositories.** A checkpoint's `git add` started git in a nested repository, whose config ran a filter on the Mac in an untrusted project and from a container; the panel's status and diff could do the same. `diff.ignoreSubmodules` does not stop `add`, and `.gitmodules` overrides it elsewhere, so snapshots leave such repositories out and the app passes `--ignore-submodules`.
- **Git config that runs programs.** Added to what the panel refuses: `hook.*`, which `core.hooksPath` does not cover; a promisor remote, through which a plain diff ran an `ext::` URL; a moved `.git/commondir`. Added to what a push refuses: `url.*` and `protocol.*`. A remote named like an option is refused, and the `ext` and `fd` transports are off for the app's pushes.
- **Docker.** A `.freeloop` symlink would have been a host folder mounted into the container (a real daemon does follow the link: a container read a file outside the project through one). Setup and archive commands ran on the Mac and could run what the container wrote; they now run in the container (OQ-026).
- **Worktree copy.** `copy:` followed a repository's symlink out of the project and made private files world-readable. Such links are left out, files keep their mode, and the trust prompt lists what is copied.
- **Mentions.** The `task` tool and a diff's text could attach a file a deny rule refuses.
- **Search key.** It followed a redirect to another host.
- **Trust gates.** The compare page's `diffStat` and the pull request checks ran without one.
- **Secrets.** A title or output cut before redaction kept the secret's start, which no longer matched; cut pieces of at least 16 characters are now recognized. A Claude Code import kept secrets in plain text when named like `OPENAI_KEY`, inside a URL, or on a command line (refused now, since a command line cannot move to the Keychain). A restored backup no longer carries MCP approvals, whose fingerprints anyone can compute. Exports and backups are private to the user.
- **Terminal output.** The command line drops control characters, so a title cannot hide a command behind escape sequences at an approval. A devcontainer image named like an option is refused.

Left as they are:

- A restored backup still replaces rules and hooks: restoring is the user's act, the confirmation says so, and it should be a backup of your own.
- A secret named `DOCKER_*` goes on the docker command line, where another process of yours can see it, because in the docker CLI's environment it could steer the CLI.

### The native sandbox, 2026-11-01

Tried before it was designed, and now held by `internal/platform/darwin/seatbelt_test.go`. File and network rules alone are not a sandbox: with them, `open` still started an app outside it and the clipboard stayed readable, because those go through system services. The profile denies every Mach service, which closes that, AppleScript, `launchctl` and the Keychain. Two rules exist because of what auto-running would otherwise allow: only a command the app itself runs can skip approval, since an agent that asks and then runs a command does so outside the sandbox; and caches are per project and used only inside it, since a shared Go build cache would let a sandboxed command poison what your own builds later use.

## Review cadence

This document is reviewed at every milestone boundary in [ROADMAP.md](ROADMAP.md) and whenever a scenario in the table gets a new control or a new scenario appears in an issue. The red-team scenarios in the table are to be encoded as automated tests in M2 (RISKS S-01).
