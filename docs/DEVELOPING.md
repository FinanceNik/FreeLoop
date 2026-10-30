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
        (anthropic, openaicompat, ollama, scripted)
```

- A **session** is driven by a harness. Both harnesses emit the same versioned events (`internal/event`, `schemas/events.json`), send every side effect through the permission broker, and reach files and processes only through an `Environment`.
- The **store** writes each event to the session's JSONL file before anything else sees it; the UI folds events into the view (`frontend/src/transcript.ts`), so a reload and live streaming are the same code.
- **Config** is YAML in the data folder (`internal/config`), reloaded when it changes (`internal/engine/watch.go`); secrets are only in the Keychain (`internal/secrets`, `internal/platform/darwin`).

Rules the lint enforces (`internal/archtest`): no `os/exec` or file I/O in the engine outside `internal/environment` (and the store, config and platform packages); Wails only in `cmd/freeloop`; macOS-only code only in `internal/platform/darwin`. The other hard rules are in [CLAUDE.md](../CLAUDE.md).

## Build and test

- `make tools` once, then `make dev` (live reload; `FREELOOP_DEV=1` adds a scripted demo provider, `FREELOOP_HOME=/some/dir` keeps your data apart).
- `make test` runs the Go tests with the race detector and the frontend tests (`node --test`, no framework). One test: `go test ./internal/<pkg>/ -run '^TestName$'`.
- `make lint` runs gofmt, `go vet`, the architecture test and `tsc`. Both pass before every commit.
- A UI change is checked in the running app: build the frontend, `go build -tags server ./cmd/freeloop`, run it with `FREELOOP_DEV=1 FREELOOP_HOME=<scratch> WAILS_SERVER_PORT=<port>` and open it in a browser.
- Docker tests need a running daemon (Colima or Docker Desktop) and skip without one.

## Adding a provider

**An OpenAI-compatible endpoint** needs no new code beyond a preset: add a `Preset` to `openaicompat.Presets` (`internal/provider/openaicompat`) with its base URL, whether it needs a key, whether it is local, any fixed headers, `StreamUsage` when it honours `stream_options.include_usage`, `ToolsKnown` when its models are known to call tools, and a `Note` for setup the user must do (a server flag, for example). Cover anything unusual in `openaicompat_test.go` against an `httptest` server.

**A new adapter** implements `provider.Provider` (`internal/provider/provider.go`):

- `Stream` returns a channel that ends with exactly one `EventDone` (the whole message, its stop reason and usage) or `EventError`. Text and thinking arrive as deltas before it.
- Normalize inside the adapter: tool-call IDs (make them up when the endpoint has none), tool arguments buffered to complete JSON, reasoning fields. The harness only ever sees `provider` types (PRV-004).
- Report HTTP failures as `*provider.HTTPError` (`provider.NewHTTPError(resp, message)`), so the loop retries timeouts, rate limits and server errors and honours `Retry-After` (PRV-010). Name the cause in the message ("rejected the API key (401)"); never put a key in it.
- `Capabilities` says what the loop may rely on per model (`ModelCaps`); return `CapsUnknown` when you do not know, and the capability probe settles it.
- Use `http.DefaultTransport` (a plain `&http.Client{}` does), so local-only mode covers the adapter (NFR-OFF-003).

Then give it a `provider.Type`, build it in `Engine.provider` and accept the type in `SaveProvider` (`internal/engine/providers.go`), offer it in `ProviderPresets`, and add the form fields it needs to `Providers.tsx`. Test against an `httptest` server: streaming, tool calls, errors and model listing.

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
```

The agent runs in the session's environment, its file and terminal requests go through the environment and the permission broker like everything else, and it signs in on its own: Freeloop never reads, stores or forwards a vendor's tokens (HRN-008). Presets that ship with the app are in `config.BuiltinPresets`; a file with the same ID replaces one. To test a preset, run the HRN-005 scenario from REQUIREMENTS.md against it; `internal/harness/acp/acptest` is a reference agent for the client's own tests.

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

## Adding an event type

Add the constant to `internal/event/event.go` (in `ACPTypes` or `FreeloopTypes`) and to `schemas/events.json`, with a `$defs` entry for its data; the schema test checks both lists agree, and the store refuses unknown types. Fold it into the view in `frontend/src/transcript.ts` with a `node --test` case, and decide whether the search index (`internal/store/search.go`) and the Markdown export (`internal/engine/export.go`) should show it.

## Every change

- Name the requirement IDs in the commit message; IDs are stable and never renumbered.
- Change the documents in the same commit when the code proves them wrong, and keep the "As built" notes in ARCHITECTURE.md current.
- A decision the documents do not settle goes to [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md) with options and a recommendation.
- Ask before adding a dependency; npm dependencies stay few and pinned.
