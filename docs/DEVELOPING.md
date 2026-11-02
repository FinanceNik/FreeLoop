# Developing Freeloop

How the code fits together and how to extend it (NFR-REL-004). [ARCHITECTURE.md](ARCHITECTURE.md) has the full design and the "As built" notes per feature; [REQUIREMENTS.md](REQUIREMENTS.md) the requirement IDs this guide refers to.

## The shape of it in one page

```
React UI (frontend/src)  ──Wails bindings──▶  EngineService (cmd/freeloop, thin adapter)
                                                     │
                                                     ▼
                                     internal/engine  (the only public surface)
                ┌───────────────┬───────────────┬──┴─────────────┬───────────────────┐
                ▼               ▼               ▼                ▼                   ▼
        harness/builtin   harness/acp     permission       environment          store
        (the loop, tools) (ACP client)    (broker, rules,  (files, processes:   (SQLite index,
                │               │          protected paths, Local, Docker)       JSONL transcripts,
                ▼               │          hooks)                                search index)
        provider/*  ◀───────────┘
        (anthropic, openaicompat, openairesponses, gemini, ollama, scripted)
```

- A **session** is driven by a harness. Both harnesses emit the same versioned events (`internal/event`, `schemas/events.json`), send every side effect through the permission broker, and reach files and processes only through an `Environment`.
- The **store** writes each event to the session's JSONL file before anything else sees it; the UI folds events into the view (`frontend/src/transcript.ts`), so a reload and live streaming are the same code.
- **Config** is YAML in the data folder (`internal/config`), reloaded when it changes (`internal/engine/watch.go`); secrets are only in the Keychain (`internal/secrets`, `internal/platform/darwin`).

Rules the lint enforces (`internal/archtest`): no `os/exec` or file I/O in the engine outside `internal/environment` (and the store, config and platform packages); Wails only in `cmd/freeloop`; macOS-only code only in `internal/platform/darwin`. The other hard rules are in [CLAUDE.md](../CLAUDE.md).

## Build and test

- `make tools` once, then `make dev` (live reload; `FREELOOP_DEV=1` adds a scripted demo provider, `FREELOOP_HOME=/some/dir` keeps your data apart).
- `make test` runs the Go tests with the race detector and the frontend tests (`node --test`, no framework). One test: `go test ./internal/<pkg>/ -run '^TestName$'`.
- `make lint` runs gofmt, `go vet`, the architecture test and `tsc`. Both pass before every commit.
- A UI change is checked in the running app: build the frontend, `go build -tags server ./cmd/freeloop`, run it with `FREELOOP_DEV=1 FREELOOP_HOME=<scratch> WAILS_SERVER_PORT=<port>` and open it in a browser. **That build serves the whole engine on a local port, with no sign-in:** any process on this Mac, other users' and sandboxed commands included, can call it, and through a terminal run commands as you. Use it on scratch data, stop it when done, and never ship it. Keychain items are shared across data folders, so give a test provider a name of its own: one with a real provider's name overwrites its key.
- Docker tests need a running daemon (Colima or Docker Desktop) and skip without one.
- `make memcheck` packages the app, starts it on a scratch data folder, runs a demo turn in ten sessions through the command line and checks the idle footprint of the app and its WebKit processes against the 300 MB of NFR-PERF-004 (`scripts/memcheck.sh`; the window opens while it runs). `TestIdleSessionsMemory` guards the engine's share in `make test`.

## The command line

While the app runs, the same binary is a command line for it (EXT-004): `Freeloop.app/Contents/MacOS/Freeloop help` lists the commands (link it as `freeloop` on your PATH). `freeloop new ~/code/app -provider work -model claude-x` starts a session and prints its ID, `freeloop prompt <id> "fix the failing test"` streams the reply, `freeloop approvals` / `freeloop approve <request>` answer what waits, and `freeloop optimizer <id> rtk` switches a session's token optimizer. Scripts can speak the JSON-RPC directly; `internal/rpc` has the methods.

## Adding a provider

**An OpenAI-compatible endpoint** needs no new code beyond a preset: add a `Preset` to `openaicompat.Presets` (`internal/provider/openaicompat`) with its base URL, whether it needs a key, whether it is local, any fixed headers, `StreamUsage` when it honours `stream_options.include_usage`, `ToolsKnown` when its models are known to call tools, and a `Note` for setup the user must do (a server flag, for example). Cover anything unusual in `openaicompat_test.go` against an `httptest` server.

**A new adapter** implements `provider.Provider` (`internal/provider/provider.go`):

- `Stream` returns a channel that ends with exactly one `EventDone` (the whole message, its stop reason and usage) or `EventError`. Text and thinking arrive as deltas before it.
- Normalize inside the adapter: tool-call IDs (make them up when the endpoint has none), tool arguments buffered to complete JSON, reasoning fields. The harness only ever sees `provider` types (PRV-004).
- Report HTTP failures as `*provider.HTTPError` (`provider.NewHTTPError(resp, message)`), so the loop retries timeouts, rate limits and server errors and honours `Retry-After` (PRV-010). An error event inside a stream that began fine is reported the same way, with the status its kind stands for (an overload as 529, a rate limit as 429). Dropped connections and timeouts are retried without one. Name the cause in the message ("rejected the API key (401)"); never put a key in it.
- `Capabilities` says what the loop may rely on per model (`ModelCaps`); return `CapsUnknown` when you do not know, and the capability probe settles it.
- Use `http.DefaultTransport` (a plain `&http.Client{}` does), so local-only mode covers the adapter (NFR-OFF-003).

`anthropic`, `openairesponses` and `gemini` are the adapters on official SDKs to copy from. Then give it a `provider.Type`, build it in `Engine.provider` and accept the type in `SaveProvider` (`internal/engine/providers.go`), offer it in `ProviderPresets`, and add the form fields it needs to `Providers.tsx`. Test against an `httptest` server: streaming, tool calls, errors and model listing.

## Adding a harness preset

An external agent that speaks ACP over stdio is a preset, not code (HRN-005). Save a YAML file as `presets/harnesses/<id>.yaml` in the data folder:

```yaml
name: My agent
description: What it is, shown where presets are chosen.
command: npx                      # run through the login shell, so npx and the like resolve
args: ["-y", "my-agent-acp@1.2.3"] # pin a version
env:                              # optional; never credentials for a subscription agent
  MY_AGENT_LOG: warn
modes:                            # the app's modes to the agent's session modes
  ask: default
  accept_edits: acceptEdits
  plan: plan
notes: Sign in once with `my-agent login` in a terminal.
skills: claude-plugin             # optional: the agent takes the session's skills as a Claude Code plugin (SKL-008)
hideText: ["[DEBUG]"]             # optional: starts of agent messages meant for machines, left out of the conversation
missingFileIsEmpty: true          # optional: for an agent that cannot recognize ACP's "not found" when reading a file it will create
```

Read the agent's real mode IDs from its `session/new` reply, not its docs, and choose the mode in which it **asks**, for `accept_edits` and `bypass` too if need be: the broker applies the session's real mode to what it asks, while a mode that approves by itself acts out of the app's sight. `plan` must be a mode that cannot write; with none, leave it out, and the app refuses to use the agent for reviews.

The built-in presets are checked by `make verify-agents`, which runs each real agent at its pinned version on a local stand-in for its model; add a case to `TestPresetScenarios` (`internal/harness/acp/presets_test.go`) with a new one, and rerun it when you change a version.

The agent runs in the session's environment, its file and terminal requests go through the environment and the permission broker like everything else, and it signs in on its own: Freeloop never reads, stores or forwards a vendor's tokens (HRN-008). Presets that ship with the app are in `config.BuiltinPresets`; a file with the same ID replaces one. To test a preset, run the HRN-005 scenario from REQUIREMENTS.md against it; `internal/harness/acp/acptest` is a reference agent for the client's own tests.

## Making a release

`make release` writes `bin/release`: the DMG, its bill of materials (`Freeloop.sbom.cdx.json`, CycloneDX 1.6), the tools it was built with (`BUILDINFO.txt`), and checksums (`SHA256SUMS`, and `APP-FILES.sha256` for every file in the bundle).

`make verify-repro REF=<tag>` builds a commit twice from clean clones and compares every file; CI does this for tags. To check a release yourself, check out its tag and install what `BUILDINFO.txt` names: the same Go (`go.mod`'s toolchain fetches it), Node and Xcode command line tools, since the linker and the SDK shape the binary. Then run `make package` and compare `bin/Freeloop.app` with `APP-FILES.sha256`.

Two things differ by design. A DMG is stamped with the time, so compare the app inside it. A signature from a Developer ID (OQ-009) is dated, so remove it first (`codesign --remove-signature`) and sign both ad hoc.

The bill lists what is in the app: Go modules from the binary's build information, and the frontend's packages from its lockfile without those only its build uses. It does not list the agents a preset downloads when you use it (their versions are in `internal/config/presets.go`), nor the operating system's frameworks.

## Writing your own harness

To build an agent of your own rather than wrap an existing one (HRN-011, EXT-003), start from a template: Profiles → "New harness…", give it a name, pick Python (standard library only) or Go (on `github.com/coder/acp-go-sdk`) and a folder. Freeloop writes the agent into `<folder>/<name>/`, registers a preset that runs it (`python3 <dir>/agent.py`, or `go -C <dir> run .`) and opens the agent file in your editor. Then:

1. Start a session with it from the new-session form. As written the agent says "You said: …" back, and `read <path>` shows a tool call that asks Freeloop for a file.
2. Replace `respond` in `agent.py` or `main.go` with your agent: call a model, use the client's methods for files, commands and permission requests (each goes through Freeloop's broker), stream text with `say`.
3. Run its test (`python3 test_agent.py` or `go test ./...`), which plays the client over the protocol the way Freeloop does.
4. Save and send the next message: the preset watches the folder (`watch:`), so an idle session starts the agent again with your code.

The templates live in `internal/harness/templates` as `.tmpl` files; their tests run in `make test`.

## Adding an environment

**A configuration** of an existing kind is a file `environments/<id>.yaml` (`config.Environment`): `kind` (`local` or `docker`), variables in `env`, `secrets` as `keychain://` references, `disabledTools`, `defaultMode`, environment-scope `rules`, and for Docker `image` or `devcontainer: true`, `setup` and `health`. The Environments page edits the same file.

**A new kind** implements `environment.Environment` (`internal/environment`), and `environment.Spawner` if agents and MCP servers should run in it:

- Every file access goes through the `Guard` the engine passes in (`Policy.Guard`), so protected paths hold whatever the harness does. `Local` is the reference; `Docker` shows how a remote kind keeps files on the host and runs processes elsewhere.
- `Exec` honours `ExecSpec`: the timeout (and ends the process tree when it fires or the context is cancelled), the output cap, `Stdin` and `SeparateStderr` (hooks rely on both), and variables passed without exposing values on a command line.
- Add the kind to `config.Environment.Validate`, build it in `Engine.runtimeFor` (`internal/engine/runtime.go`, see `dockerEnv`), and stop or remove what it starts with the session.
- Keep the app's own git commands on the host (`runtime.host`), and keep anything the host reads and acts on (the project's `.freeloop/`, `.git/config`, hooks) out of the reach of commands in the environment.

## The native sandbox

`config.Environment.Sandbox` turns it on (ENV-009). `internal/engine/sandbox.go` decides what a session's commands reach, `environment.Local` wraps `Exec` with a `Confiner`, and `internal/platform/darwin/seatbelt.go` is the only macOS-specific part. Only `Exec` is confined; `Spawn` is not, so agents and MCP servers stay outside. A caller that runs a command itself sets `permission.Request.Contained`, which is what lets the broker skip the approval. Never set it for a request an agent will carry out on its own. If a tool fails in the sandbox, first look for what it writes under home: most fixes are a variable in `SandboxCaches` or a folder in `sandboxTools`. Add anything that closes a way out to `TestSeatbeltRefuses`, with a control if the refusal could have another cause. A host list (ENV-010) adds `netguard.Egress`: its variables are in `environment.ProxyEnv`, and `FREELOOP_SANDBOX_NET=1 go test ./internal/platform/darwin` tries real tools against the real internet.

## Adding a token optimizer

An optimizer is a file `optimizers/<id>.yaml` in the data folder, or an entry in `config.BuiltinOptimizers` to ship it. A file with a built-in's ID replaces it:

```yaml
name: RTK
program: rtk                # a name, found the way commands find it; never a path
rewrite: [rewrite]          # arguments before the command to rewrite
rewriteExits: [0, 3]        # exit codes that mean it printed a rewrite
probeCommand: git status    # a known command...
probeWant: rtk git status   # ...and what it must become, which tells it from another program with the same name
version: [--version]        # optional
rewrites: {git: git, cat: read}  # the tools it may rewrite, and its word for each
leaveAlone: [--output]      # optional: options that keep a command from being rewritten
leaveAloneCommands: [git log]  # optional: commands it shortens in a way the reader cannot tell
env:                        # optional: added to every command while it is on; names start with the program's
  RTK_TELEMETRY_DISABLED: "1"
dataEnv:                    # optional: variables pointing at files in the session's own folder
  RTK_DB_PATH: tracking.db
install: brew install rtk   # optional, one line: typed into a terminal for the user to run
```

A file runs only after the user approves it on the Settings page, and any change ends that. Freeloop asks from `/` with `HOME` set to nothing, so the program reads no project's or agent's settings. Only plain lines are asked about, and a rewrite must be `program` and the same command. Before listing a tool in `rewrites`, try it raw and rewritten in identical repositories, and with a copy of the tool planted in the repository, since a program may prefer that copy; that is how the `--output` and `tsc` problems were found. Run the program with what you add through `internal/engine/optimizers.go` before trusting it, under the sandbox too: check that exit codes survive, that refusals stay visible, and where it writes. A variable that stops it reporting home belongs in `Env`. Never use a program's own agent hook: RTK's answers "allow" when a repository's settings say so (OQ-029).

## Adding an event type

Add the constant to `internal/event/event.go` (in `ACPTypes` or `FreeloopTypes`) and to `schemas/events.json`, with a `$defs` entry for its data; the schema test checks both lists agree, and the store refuses unknown types. Fold it into the view in `frontend/src/transcript.ts` with a `node --test` case, and decide whether the search index (`internal/store/search.go`) and the Markdown export (`internal/engine/export.go`) should show it.

## Accessibility

Pure rules are in `frontend/src/a11y.ts`, and `a11y.test.ts` computes the contrast of every text colour on every surface in each theme. To keep it true: define colours as theme tokens, never dim text with `opacity`, and size text in `rem` (`calc(12rem / 14)` is 12 at the default), so the Font size setting reaches it. A dot, colour or icon that carries meaning also needs words, in `.sr-only` if they would clutter the view. `aria-label` on a plain `div` or `span` is ignored: give it a role.

To check with VoiceOver (⌘F5), which no test can replace:

1. Move through the sidebar with Tab: each session says its title and status, and opening one says its title.
2. Open Board, Usage and Settings: each is announced, and the next Tab lands in it.
3. From the top of the page, the first Tab offers "Skip to the main content".
4. Send a message and jump by heading (VO+⌘H): you hear "You" and "Agent" before each message.
5. When the agent asks: you hear "Approval needed", the card says "Waiting for your approval", and Y and N answer it.
6. Tool cards say Done, Failed or Running before their titles.
7. Type `@` in the message box and use the arrows: each entry is read with its place in the list.
8. In the panel, use the arrows across the tabs: they move focus without opening anything, and Return opens one.
9. In the Terminal tab, type a command and hear the output; Shift+Esc leaves it.
10. Open ⌘K, and a picture at full size: Tab stays inside, and Escape returns you to where you were.
11. On Usage, the arrows walk the chart's columns, and "Show as a table" reads the same data. On its Activity tab, the arrows walk the weekday-and-hour cells, and each says its count.
12. Press ⌘+ and ⌘-: the text changes, you hear its size, and ⌘0 still opens Needs attention.

## Every change

- Name the requirement IDs in the commit message; IDs are stable and never renumbered.
- Change the documents in the same commit when the code proves them wrong, and keep the "As built" notes in ARCHITECTURE.md current.
- A decision the documents do not settle goes to [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md) with options and a recommendation.
- Ask before adding a dependency; npm dependencies stay few and pinned.
