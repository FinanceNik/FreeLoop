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
| 1 | A file the agent reads says "run `curl … \| sh`" | Arbitrary code execution | Execution needs approval in `ask` and `accept-edits` (SEC-001); deny rules win (SEC-002); tool output framed as untrusted (SEC-010); container environments (ENV-005); native sandbox later (ENV-009) |
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
| 13 | A compromised dependency of Freeloop itself | Everything | Pinned modules and lockfiles, SBOM, signed and notarized builds (SEC-012); minimal npm surface |
| 14 | The app leaks vendor sign-in tokens | Legal exposure and account compromise | The app never reads, stores or forwards them; vendor binaries own their login (HRN-008) |

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
| Writes outside project root, into `.git/`, into app config | Denied in every mode |
| Reads outside project root | Prompt, except never-readable locations which are refused |
| New MCP server | Does not start until confirmed with its full command shown |
| Project configuration | Not read until the project is trusted |
| Outbound connections | Providers, MCP servers and harness endpoints the user configured; update check opt-in |

## Limitations the user should know

- The local environment is **not a sandbox**. In `bypass` mode an agent can do anything the user can. Use a Docker environment (v1) for repositories you do not trust.
- **Unknown secrets are not redacted.** Redaction works on values the app knows (stored secrets). A password typed into a prompt is sent to the provider and stored in the transcript.
- **Approval fatigue is real.** Rules exist to make trusted operations quiet so that prompts stay meaningful. Review rules periodically; the settings UI lists every rule with its source.
- **Transcripts are plaintext** on disk, protected only by file permissions.
- **MCP servers run with the user's privileges** in the local environment. Prefer servers you can read, pin their versions, and give remote servers the least access their OAuth scopes allow.
- **Provider trust.** Everything in a session's context goes to the configured provider. Local models keep it on the machine; cloud providers' data handling is governed by their terms. That includes a secret a command prints: environment secrets are redacted from the transcript, not from the model's context.
- **Path rules cover file tools, not the shell.** Protected paths, the read-outside-root prompt and path rules apply to `read_file`, `write_file`, `edit_file`, `list_dir`, `glob` and `grep`. An approved shell command runs with the user's privileges and can read or write anywhere, including protected locations; a sandbox for shell commands (ENV-009) is what will close this. Shell deny rules see through common wrappers (variable assignments, `env`, `command`, `exec`, subshells, `sh -c '…'`), but a shell can always spell a command another way, so treat them as a guard against mistakes.
- **A mentioned file goes to the model without an approval.** Writing `@path` in a message attaches that file's contents, as pasting it would. The engine reads it through the environment, so protected paths stay refused, and only inside the project root (at most 20 files, 256 KB each, 1 MB per message); it is the user's choice of context, not a harness side effect, so no permission request is made.
- **Git runs in a project before it is trusted.** The app reads branch and changed-file count with `git status`. It turns off `core.fsmonitor` and hooks and skips the scan when the repository configures content filters, but a repository's own configuration is untrusted input until projects are trusted explicitly (SEC-006, M2).

### Hardening review, 2026-10-29

An independent review of M1 slices 1 to 4 found and reproduced these, all fixed with regression tests: protected paths compared case-sensitively on a case-insensitive file system (`.GIT`, `~/.SSH`); `glob`/`grep` and reads following symlinks past the broker; unbounded reads of non-regular files; "Allow always" saving literal input as a pattern (`rm -rf *`, a file named `**`); `Exec` hanging on a descendant that left the process group; a malformed `rules.yaml` silently dropping deny rules and being overwritten; a crash leaving an unanswered tool call that broke the session; an invalid tool input breaking every later Anthropic request; secrets streamed across chunks escaping redaction; stored keys not redacted until used; `core.fsmonitor` and content filters running through the app's git calls; commands inheriting the app's whole environment; exact rules collapsing newlines; a cancel race in the prompt queue.

## Review cadence

This document is reviewed at every milestone boundary in [ROADMAP.md](ROADMAP.md) and whenever a scenario in the table gets a new control or a new scenario appears in an issue. The red-team scenarios in the table are to be encoded as automated tests in M2 (RISKS S-01).
