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
- **Config** is YAML in the data folder (`internal/config`), reloaded when it changes (`internal/engine/watch.go`); secrets are only in the Keychain, together in one item (`secrets.Vault`, `internal/platform/darwin`).

Rules the lint enforces (`internal/archtest`): no `os/exec` or file I/O in the engine outside `internal/environment` (and the store, config and platform packages); Wails only in `cmd/freeloop`; macOS-only code only in `internal/platform/darwin`. The other hard rules are in [CLAUDE.md](../CLAUDE.md).

## Build and test

- `make tools` once, then `make dev` (live reload; `FREELOOP_DEV=1` adds a scripted demo provider, `FREELOOP_HOME=/some/dir` keeps your data apart).
- The development switches (`FREELOOP_DEV`, `FREELOOP_DEV_*`, `FREELOOP_RELEASE_API`, `FREELOOP_HOME`) work in every build but the one users get: `make dmg` and `make release` (and `make package SHIPPED=1`) link it with `-X main.shipped=1`, and that build ignores them and logs their names once at start. `make build`, `make package`, `make dev` and the served build keep them. Read a new switch with `devEnv` in `cmd/freeloop/devenv.go`; the architecture test fails on any other read of a `FREELOOP_` variable there, and on one elsewhere outside `internal/store`'s test-only event check.
- Voice input (GUI-050) runs whisper.cpp's `whisper-cli`. A Mac app carries it: `make package` runs `make whisper-cli` (`scripts/whisper-cli.sh`), which builds it from the pinned source tarball into `bin/whisper-cli` and needs cmake (`brew install cmake`); a build from `make dev` finds it there too. On Windows and Linux the app downloads whisper.cpp's own release when the user asks. To move to a newer whisper.cpp, change the version and tarball checksum in the script and the release assets, their sizes and GitHub's digests in `config.BuiltinSpeech`; the model is pinned to a Hugging Face revision there too. `FREELOOP_DEV_VOICE=scripted` (set by `make check-app`) fakes the download, records a tone and answers with a fixed text, as the check has no microphone.
- What a commit costs (GUI-051) is folded from the transcripts like the activity table: `Store.FoldWork` folds a session again whole when it moved on, within a time budget, into `work_commits` (one row per SHA), `work_usage` (each turn's use) and `usage_buckets` (five-minute use, kept for good). They are made with `IF NOT EXISTS` outside the migrations, as `limit_reports` is, so an older build still opens the database; `sessions.work_seq` is an added column. `git_state` events list the commits HEAD moved forward over; for transcripts from before that, the fold asks the engine's resolver once per move and keeps the answer in `work_resolved`. The engine joins the rows and prices them at query time.
- The phone server (GUI-052) is `internal/remote`, an adapter over the engine like `internal/rpc`, started by `cmd/freeloop/phone.go`, which also holds keep-awake. It serves only its listed routes; anything new the phone may do goes through an engine method made for it (`PromptFrom`, `DecideOnce`), never through the bindings. The phone app is a second Vite build (`vite.phone.config.ts`, root `frontend/phone`) into `dist/phone`; it imports no Wails runtime, so shared components reach links and the clipboard through `src/host.ts`. Its Content Security Policy is a header from the server. `make check-app` gives it a port of its own.
- `make test` runs the Go tests, `cmd/` included, with the race detector and the frontend tests (`node --test`, no framework). One test: `go test ./internal/<pkg>/ -run '^TestName$'`.
- `make lint` runs gofmt, `go vet`, the architecture test and `tsc`. Both pass before every commit.
- Ask the user with `ask` (`frontend/src/ask.ts`), never the page's `confirm`, `alert` or `prompt`: Wails gives the web view no way to show them, so the packaged app answers no without showing anything, while a browser shows them. A test holds this. A served build has no native dialogs and its `ask` never answers, so a check there answers that call itself.
- The app's icon is `build/appicon.png`, drawn by `scripts/make-icon.swift`; `scripts/make-icon.sh` runs that and makes `build/darwin/icons.icns`, `build/windows/icon.ico` (through the pinned `wails3`) and the PNGs in `build/linux/icons` (`ICONS_ONLY=1` skips the drawing). There is no `Assets.car`: it needs full Xcode to make, and it would win over the `.icns`.
- **Windows and Linux builds (NFR-XPLAT-003).** `Taskfile.yml`'s `build`, `package` and `run` pick the host's platform from `build/darwin`, `build/windows` or `build/linux`. Pass the architecture to these tasks as an environment variable (`ARCH=arm64 wails3 task windows:package`), because Task does not pass command-line variables to an included file's vars.
  - `make windows-exe` cross-compiles `bin/windows-{amd64,arm64}/Freeloop.exe` from any host (pure Go, `CGO_ENABLED=0`, `-H windowsgui`, `-X main.shipped=1`). Its resource comes from `wails3 generate syso`, written next to `main` for the one build and then removed: the icon, the version, and `build/windows/freeloop.exe.manifest` (Windows 10 and later, per-monitor v2 DPI awareness, long paths, common controls v6).
  - `make windows` also builds, per architecture, `bin/Freeloop-<version>-windows-<arch>-setup.exe` (`build/windows/installer.nsi`) and `-portable.zip`. The installer installs per user under `%LOCALAPPDATA%\Programs\Freeloop`, refuses Windows older than 10 1809 (ConPTY, AF_UNIX), offers Microsoft's WebView2 bootstrapper when the runtime is missing, and registers an uninstaller that keeps the user's data. It needs NSIS 3 (`brew install makensis`, `apt install nsis`) and `zip`, and the target says so when they are missing. Like the Mac build, it is unsigned.
  - `make linux-package` builds `bin/Freeloop-<version>-linux-<arch>.AppImage` and `.deb` in the Docker image (`build/linux/Dockerfile`, which adds the pinned `wails3` and a checksummed linuxdeploy; `build/linux/package.sh` runs inside). Each architecture builds natively in its own container. By default only the Docker machine's own architecture is built; `LINUX_ARCHS="amd64 arm64"` adds the other one, emulated through `--platform`. That works under QEMU, but under Rosetta (Colima's `vz` with `rosetta: true`, and Docker Desktop's default) the Go toolchain crashes with heap errors while building. So on Apple Silicon the amd64 packages come from CI's amd64 runner, an amd64 Linux machine, or a Docker that emulates with QEMU only. The `.deb` (nfpm inside `wails3`, `build/linux/nfpm/nfpm.yaml`) depends on `libgtk-3-0` and `libwebkit2gtk-4.1-0`, recommends `bubblewrap`, a Secret Service and `git`, and installs `/usr/bin/freeloop`, `freeloop.desktop` and hicolor icons. The AppImage bundles WebKitGTK through linuxdeploy's GTK plugin.
- `make check-app` drives the real interface on the real engine in a real WebKit view, on scratch data: sending, approving, following the agent, panes, a setting reaching its file. It runs the served build, since a packaged app cannot be scripted, so a native dialog is the one thing it answers itself. Add a step for a bug only the running app can show, and put the bug back once to see the step fail: its first version passed with the scrolling bug in place. A second pass (`scripts/appcheck/lived-in.js`, data from `lived-in.sh`) runs the same app on a data folder set up as a user's is after weeks: a default profile on an external agent, a global rule allowing shell, 40 skills with long descriptions, a default token optimizer, a hook written by hand, an MCP server that cannot start, 30 sessions. A bug that shows only on settings a fresh folder lacks goes there; the demo running on the default profile's agent was one. Both scenarios share the helpers in `lib.js`.
- A UI change is checked in the running app: build the frontend, `go build -tags server ./cmd/freeloop`, run it with `FREELOOP_DEV=1 FREELOOP_HOME=<scratch> WAILS_SERVER_PORT=<port>` and open it in a browser. **That build serves the whole engine on a local port, with no sign-in:** any process on this Mac, other users' and sandboxed commands included, can call it, and through a terminal run commands as you. Use it on scratch data, stop it when done, and never ship it. Keychain entries are shared across data folders, so give a test provider a name of its own: one with a real provider's name overwrites its key, or set `FREELOOP_DEV_SECRETS=memory` to leave the Keychain out; with that, a scratch folder's approvals last only until the app stops. That build has no dialogs: `FREELOOP_DEV_BACKUP=<file>` is the backup a restore previews.
- Docker tests need a running daemon (Colima or Docker Desktop) and skip without one.
- `make memcheck` packages the app, starts it on a scratch data folder, runs a demo turn in ten sessions through the command line and checks the idle footprint of the app and its WebKit processes against the 300 MB of NFR-PERF-004 (`scripts/memcheck.sh`; the window opens while it runs). `TestIdleSessionsMemory` guards the engine's share in `make test`.

## The command line

While the app runs, the same binary is a command line for it (EXT-004): `Freeloop.app/Contents/MacOS/Freeloop help` lists the commands (link it as `freeloop` on your PATH). `freeloop new ~/code/app -provider work -model claude-x` starts a session and prints its ID, `freeloop prompt <id> "fix the failing test"` streams the reply, `freeloop approvals` / `freeloop approve <request>` answer what waits, and `freeloop optimizer <id> rtk` switches a session's token optimizer. `freeloop check -project . shell git push origin main` says whether a call would be allowed, asked about or denied, and by which rule (SEC-018); it exits 0, 2 or 3 for those and 1 on an error, so a script or CI can assert a rule holds. Scripts can speak the JSON-RPC directly; `internal/rpc` has the methods.

## Adding a provider

**An OpenAI-compatible endpoint** needs no new code beyond a preset: add a `Preset` to `openaicompat.Presets` (`internal/provider/openaicompat`) with its base URL, whether it needs a key, whether it is local, any fixed headers, `StreamUsage` to ask for `stream_options.include_usage` (a server refusing it with 400 or 422 is asked again without, and then not asked again), `ToolsKnown` when its models are known to call tools, and a `Note` for setup the user must do (a server flag, for example). Cover anything unusual in `openaicompat_test.go` against an `httptest` server.

**A new adapter** implements `provider.Provider` (`internal/provider/provider.go`):

- `Stream` returns a channel that ends with exactly one `EventDone` (the whole message, its stop reason and usage) or `EventError`. Text and thinking arrive as deltas before it.
- Normalize inside the adapter: tool-call IDs (make them up when the endpoint has none), tool arguments buffered to complete JSON, reasoning fields. The harness only ever sees `provider` types (PRV-004).
- Report HTTP failures as `*provider.HTTPError` (`provider.NewHTTPError(resp, message)`), so the loop retries timeouts, rate limits and server errors and honours `Retry-After` (PRV-010). An error event inside a stream that began fine is reported the same way, with the status its kind stands for (an overload as 529, a rate limit as 429). Dropped connections and timeouts are retried without one. Name the cause in the message ("rejected the API key (401)"); never put a key in it.
- `Capabilities` says what the loop may rely on per model (`ModelCaps`); return `CapsUnknown` when you do not know, and the capability probe settles it.
- Use `http.DefaultTransport` (a plain `&http.Client{}` does), so local-only mode covers the adapter (NFR-OFF-003).

`anthropic`, `openairesponses` and `gemini` are the adapters on official SDKs to copy from. Then give it a `provider.Type`, build it in `Engine.provider` and accept the type in `SaveProvider` (`internal/engine/providers.go`), offer it in `ProviderPresets`, and add the form fields it needs to `Providers.tsx`. Test against an `httptest` server: streaming, tool calls, errors and model listing.

The in-app demo (GUI-045) runs on `scripted.Tour`, as the provider `freeloop-tour`: it exists in every build, unlike `demo`, which `FREELOOP_DEV=1` adds, and it is never listed or saved, so no picker offers it. Its steps are the demo's script; changing one means updating `internal/engine/demo_test.go` and the "the demo runs a tour and removes itself" step in `scripts/appcheck/scenario.js`.

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
skills: claude-plugin             # optional: an agent on the Claude Agent SDK: it takes the session's skills as a Claude Code plugin (SKL-008) and reports tokens per model call (PRV-008)
hideText: ["[DEBUG]"]             # optional: starts of agent messages meant for machines, left out of the conversation
missingFileIsEmpty: true          # optional: for an agent that cannot recognize ACP's "not found" when reading a file it will create
inputCountsCache: true            # optional: the input tokens it reports for a turn include the cached ones, as OpenAI's and Gemini's APIs count them (PRV-008)
decidesItself:                    # optional: what the agent approves by itself, for the Policy page and freeloop check (SEC-018)
  - mode: auto                    # optional: only in this mode
    text: In auto mode it approves its own tool calls, which never reach Freeloop.
```

Read the agent's real mode IDs from its `session/new` reply, not its docs, and choose the mode in which it **asks**, for `accept_edits` and `bypass` too if need be: the broker applies the session's real mode to what it asks, while a mode that approves by itself acts out of the app's sight. `plan` must be a mode that cannot write; with none, leave it out, and the app refuses to use the agent for reviews.

The built-in presets are checked by `make verify-agents`, which runs each real agent at its pinned version on a local stand-in for its model, also in a repository whose own config tells the agent to stop asking (give a preset a `repo` in the test if its agent reads such a file); add a case to `TestPresetScenarios` (`internal/harness/acp/presets_test.go`) with a new one, and rerun it when you change a version. It also checks how each agent reports its usage: per model call, a turn's total, or none, and the model it names (PRV-008). ACP leaves open whether a turn's `inputTokens` include the cached ones: read the agent's code, and set `inputCountsCache` when they do (Gemini CLI and Copilot CLI; Codex's adapter takes them out itself), or a turn that mostly read the cache is priced about ten times over. A turn's total is never priced as one long prompt, since it adds up several calls; a new version that changes that fails there, and a preset for an agent that reports no tokens leaves its sessions unpriced, said as "not reported by the agent".

A preset file starts an agent only after the user approves it in Settings, and any change ends that; one made with "New harness…" is approved already. The same holds for a hook in `config.yaml` or a profile that was not written in the app (`internal/engine/approvals.go`), and for what changes how much is asked or confined: an allow rule in `rules.yaml`, a profile or an environment, an environment's settings, `permissions.allowBypass`, a profile's `mode: accept_edits` and a file's choice of token optimizer. Nothing is sent to an address from a file until it is approved either: a provider's, `web.searchURL`, `mcp.registryURL`, a profile's `fallback`, and the update check. A test that writes one of those files after `Open` calls `approveFiles`. A new setting that can make the app ask less, confine less or send elsewhere belongs in `fileItems`, which the restore preview reads too.

The agent runs in the session's environment, its file and terminal requests go through the environment and the permission broker like everything else, and it signs in on its own: Freeloop never reads, stores or forwards a vendor's tokens (HRN-008). Presets that ship with the app are in `config.BuiltinPresets`; a file with the same ID replaces one. To test a preset, run the HRN-005 scenario from REQUIREMENTS.md against it; `internal/harness/acp/acptest` is a reference agent for the client's own tests.

## Making a release

A release comes from a tag, made by CI (the `release` job in `.github/workflows/ci.yml`):

1. **Set the version** in `internal/engine/updates.go` (`AppVersion`), `build/config.yml`, `frontend/package.json` and its lockfile, and the two `Info.plist` files, which take only numbers: `0.1.0` for `0.1.0-beta.1`. `TestAppVersionMatchesTheBuild` fails on any you miss.
2. **Write `docs/release-notes/<version>.md`**: what it is, how to open an unsigned download, what is missing. The job refuses a tag without one.
3. **Push and wait for CI to pass** (`gh run watch`).
4. **Dry run.** On the Actions page, run CI with *dry run* checked, or `gh workflow run CI -f dry_run=true`. The release job builds every file of a release and keeps them as an artifact; download it and check `shasum -a 256 -c SHA256SUMS` and the version in the app.
5. **Tag and push the tag**: `git tag v<version> && git push origin v<version>`. CI runs every check, the real agents and the reproducibility check, then makes a **draft** pre-release with the files and the notes. The tag must be `v` plus `AppVersion`.
6. **Read the draft** on the Releases page and publish it.

**When CI cannot run** (no Actions minutes left), the same files come from a Mac: check out the tag, run `make test lint check-app`, then `make release` (with makensis installed and Docker running, so that the Windows and Linux files are built too), and make the draft yourself with `gh release create v<version> --draft --prerelease --verify-tag --title "Freeloop <version>" --notes-file docs/release-notes/<version>.md bin/release/*`. CI's reproducibility check (`make verify-repro REF=v<version>`) is then yours to run too.

**CI's jobs.** `test`, on Linux, runs on every push and pull request: dependency checksums, `gofmt` and `go vet`, the Go tests with every event checked against the schema, and the frontend tests. Tests that need macOS carry a `darwin` build tag. `windows`, on `windows-latest` under Git Bash, runs on every push and pull request too: `go vet`, the Go tests (with `-race` through the runner's MinGW gcc), the frontend tests, and the shipped build for both Windows architectures. On tags and from the Actions page, `package-windows` (Ubuntu with NSIS) and `package-linux` (an amd64 and an arm64 Ubuntu runner, each in the Docker image) build those platforms' release files, and the `release` job adds them to the Mac's before writing `SHA256SUMS`. `build`, on macOS, runs on tags, weekly and from the Actions page (`gh workflow run CI`): the build, `make test`, lint, `make check-app`, and on tags and weekly the real agents and the reproducibility check. A macOS minute counts ten on a private repository, which is why the app check runs locally after an interface change rather than on every push.

What it builds:

`make release` writes `bin/release` for `RELEASE_PLATFORMS`. By default that is whatever the host can build: `mac` on a Mac, `windows` where makensis is installed, `linux` where Docker runs (for the Docker machine's architecture, see above). Name platforms to build exactly those and fail on any that cannot be built; `RELEASE_PLATFORMS=mac` gives the Mac-only release of before. What each platform adds:

- **Mac:** the DMG, its bill of materials (`Freeloop.sbom.cdx.json`, CycloneDX 1.6), the tools it was built with (`BUILDINFO.txt`), `THIRD_PARTY_NOTICES.txt`, and `APP-FILES.sha256` for every file in the bundle.
- **Windows:** the two installers and two zips, `Freeloop-windows-<arch>.sbom.cdx.json`, `THIRD_PARTY_NOTICES-windows.txt`, `BUILDINFO-windows.txt`, and `WINDOWS-FILES.sha256` for the two exes.
- **Linux:** the AppImages and `.deb`s, `Freeloop-linux-<arch>.sbom.cdx.json`, `THIRD_PARTY_NOTICES-linux.txt`, and `BUILDINFO-linux-<arch>.txt`, which records the image's Ubuntu, glibc, GTK, WebKitGTK and linuxdeploy.

`SHA256SUMS` covers every file (`make release-sums` rewrites it after files are added by hand).

### Which build is this, and keeping one copy (NFR-REL-006)

The version in `build/config.yml` is the same for every build of a release, so each build also carries its commit. `Taskfile.yml` computes `COMMIT` (`git rev-parse --short=7 HEAD`, plus `+dirty` when `git status --porcelain` is not empty, and nothing outside a repository); the three platforms' `build:*` tasks link it into the binary with `-X .../internal/engine.BuildCommit=…`, and the Mac's `create:app:bundle` writes it into the bundle's `Info.plist` as `FreeloopCommit` before signing, so a bundle can be read without starting it. A development build (`DEV=true`) is not stamped; `engine.Build()` then reads Go's own VCS record, which is also what the served build of `make check-app` says. `Build()` feeds Settings (`EngineService.AppBuild`) and the first line of the problem report. A reproducible build keeps working: two clean clones of one commit get one stamp, and a dirty tree says so.

`make install` (`scripts/install.sh`, tested by `cmd/freeloop/install_test.go` in a throwaway repository with a fake build and a temporary home folder) is the Mac's one way to leave one current copy. It quits the app through its bundle identifier and never forces it, refuses to run in a process whose ancestors include the app, moves the old bundle to the Trash before building and back if the build or the check fails, and moves every other bundle with the identifier to the Trash. Its test-only switches (`INSTALL_BUILD`, `LSREGISTER`, `OSASCRIPT`, `INSTALL_SPOTLIGHT`, `INSTALL_QUIT_TRIES`) are listed in its header. The stamp's `git` calls pass `safe.directory='*'`, so a checkout that belongs to another user, as in the Docker image, can still be read.

`make verify-repro REF=<tag>` builds a commit twice from clean clones and compares every file; CI does this for tags. To check a release yourself, check out its tag and install what `BUILDINFO.txt` names: the same Go (`go.mod`'s toolchain fetches it), Node and Xcode command line tools, since the linker and the SDK shape the binary. Then run `make package SHIPPED=1` (a release's app is the shipped build) and compare `bin/Freeloop.app` with `APP-FILES.sha256`.

`verify-repro` also builds the two Windows exes in both clones and compares them. They are cross-compiled pure Go, so any host produces the same bytes, which can be checked against `WINDOWS-FILES.sha256`. The installers, zips, AppImages and `.deb`s carry timestamps and are not compared. Linux builds are not compared either.

Two things differ by design. A DMG is stamped with the time, so compare the app inside it. Should a build ever carry a Developer ID signature, which releases do not (OQ-009), it is dated, so remove it first (`codesign --remove-signature`) and sign both ad hoc.

`make notices` writes `bin/THIRD_PARTY_NOTICES.txt`, the license and notice files of the same things, which goes in the DMG and the release. It names on stderr anything with no such file, for you to check before a release; `@wailsio/runtime` is one, with its license in the Wails Go module that is also listed.

The bill lists what is in the app: Go modules from the binary's build information, and the frontend's packages from its lockfile without those only its build uses. It does not list the agents a preset downloads when you use it (their versions are in `internal/config/presets.go`), nor the operating system's frameworks.

## Logs and recovery

**After a crash.** `running.json` in the data folder lists the process groups the app has running: commands, agents, MCP servers, syncs and terminals, each with its first process's start time and program name. A clean quit leaves it empty; after a crash or `kill -9`, the next start stops what is still there and logs it (NFR-REL-005, `internal/environment/ledger.go`). A process is stopped only if it still has the recorded start time, so one that later got the same number is never touched.

`logs/freeloop.log` holds the log and, in a launched app, crash traces; a development build also keeps its terminal. Everything through `log` passes the engine's redaction. Deleting `freeloop.db` is safe: it is rebuilt from the daily copy, `meta.json` and the transcripts, with projects untrusted unless the copy says otherwise. A data folder takes one copy of the app at a time, so `make dev` beside an installed app needs its own `FREELOOP_HOME`.

## Before the repository goes public

The repository is private. Publishing exposes all of its history, so do these first, once no branch is in flight:

1. **Commit identities.** Every commit carries its author's name and email, which a scan of the contents does not show. Rewrite them to the address meant to be public (`git filter-repo --mailmap`), force-push and reclone. That changes every commit hash, including those the roadmap quotes, so do it last. Set `user.email` for this repository first, or the next commit brings the old one back.
2. **What GitHub holds beyond git.** Issues, pull requests, Actions logs and old branches become public too. Read them, and delete the branches that are not meant to be seen.
3. **Scan again.** Search all of history, not only the current tree, for home paths, addresses, machine names and key patterns. The keys in tests are made up; check any that is new.
4. **Turn on private vulnerability reporting** in the repository's settings, which is what `docs/SECURITY.md` points people to. The update check also works only once the repository is public.

## Writing your own harness

To build an agent of your own rather than wrap an existing one (HRN-011, EXT-003), start from a template: Profiles → "New harness…", give it a name, pick Python (standard library only) or Go (on `github.com/coder/acp-go-sdk`) and a folder. Freeloop writes the agent into `<folder>/<name>/`, registers a preset that runs it (`python3 <dir>/agent.py`, or `go -C <dir> run .`) and opens the agent file in your editor. Then:

1. Start a session with it from the new-session form. As written the agent says "You said: …" back, and `read <path>` shows a tool call that asks Freeloop for a file.
2. Replace `respond` in `agent.py` or `main.go` with your agent: call a model, use the client's methods for files, commands and permission requests (each goes through Freeloop's broker), stream text with `say`.
3. Run its test (`python3 test_agent.py` or `go test ./...`), which plays the client over the protocol the way Freeloop does.
4. Save and send the next message: the preset watches the folder (`watch:`), so an idle session starts the agent again with your code.

The templates live in `internal/harness/templates` as `.tmpl` files; their tests run in `make test`.

## Adding an environment

**A configuration** of an existing kind is a file `environments/<id>.yaml` (`config.Environment`): `kind` (`local`, `docker` or `ssh`), variables in `env`, `secrets` as `keychain://` references, `disabledTools`, `defaultMode`, environment-scope `rules`, and for Docker `image` or `devcontainer: true`, `setup` and `health`. The Environments page edits the same file.

**A new kind** implements `environment.Environment` (`internal/environment`), and `environment.Spawner` if agents and MCP servers should run in it:

- Every file access goes through the `Guard` the engine passes in (`Policy.Guard`), so protected paths hold whatever the harness does. `Local` is the reference; `Docker` shows how a remote kind keeps files on the host and runs processes elsewhere.
- `Exec` honours `ExecSpec`: the timeout (and ends the process tree when it fires or the context is cancelled), the output cap, `Stdin` and `SeparateStderr` (hooks rely on both), and variables passed without exposing values on a command line.
- Add the kind to `config.Environment.Validate`, build it in `Engine.runtimeFor` (`internal/engine/runtime.go`, see `dockerEnv`), and stop or remove what it starts with the session.
- Keep the app's own git commands on the host (`runtime.host`), and keep anything the host reads and acts on (the project's `.freeloop/`, `.git/config`, hooks) out of the reach of commands in the environment.

## The native sandbox

`config.Environment.Sandbox` turns it on (ENV-009). `internal/engine/sandbox.go` decides what a session's commands reach, `environment.Local` wraps `Exec` with a `Confiner`, and `internal/platform/darwin/seatbelt.go` is the only macOS-specific part. Only `Exec` is confined; `Spawn` is not, so agents and MCP servers stay outside. A caller that runs a command itself sets `permission.Request.Contained`, which is what lets the broker skip the approval. Never set it for a request an agent will carry out on its own. If a tool fails in the sandbox, first look for what it writes under home: most fixes are a variable in `SandboxCaches` or a folder in `sandboxTools`. Add anything that closes a way out to `TestSeatbeltRefuses`, with a control if the refusal could have another cause. A host list (ENV-010) adds `netguard.Egress`: its variables are in `environment.ProxyEnv`, and `FREELOOP_SANDBOX_NET=1 go test ./internal/platform/darwin` tries real tools against the real internet.

## Updating prices

`listPrices` in `internal/config/builtinprices.go` holds the vendors' list prices, with the pages they come from; set `PricesAsOf` when you change them. Add a model's name only: dated names get a row of their own. A model that charges more above a prompt size has `longOver` and a row named with `LongPromptSuffix`, ahead of it. Its calls are counted under that name, so that the sums per model can be priced.

Claude Code sessions report tokens per model call through the adapter's raw SDK messages (`internal/harness/acp/calls.go`). Other external agents report at the end of each turn, as ACP has it, and name no model, so they show no cost unless their session names one.

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

Account limits (GUI-028) follow the same raw messages as per-call tokens: `rate_limit_event` is in `sdkMessageFilter`, and `internal/harness/acp/limits.go` turns its `unifiedWindows`, whose utilization is a share of one, into a generic `limit_update`, so the frontend knows no vendor. Check that field again when the preset's adapter version changes. An agent on another protocol reports limits by emitting the same event.

The model is told once when an optimizer comes on (`optimizerNote`, through `harness.Prompt.Note`, which the transcript does not show). A built-in optimizer can name `Gain`, the arguments that print its savings as JSON (`parseGain`); a file cannot, so a file's optimizer shows only the count of commands it shortened.

Only a built-in can offer a one-click install, through `Download` in `config.BuiltinOptimizers`: a version and, per architecture, a `.tar.gz` URL and its SHA-256. A file cannot, so no file makes the app fetch anything.

A file runs only after the user approves it on the Settings page, and any change ends that. For the built-in agent Freeloop asks from `/` with `HOME` set to nothing, so the program reads no project's or agent's settings; for an agent's hook it asks from the project with the real `HOME`, so the agent's deny rules count. Only plain lines are asked about, and a rewrite must be `program` and the same command. Before listing a tool in `rewrites`, try it raw and rewritten in identical repositories, and with a copy of the tool planted in the repository, since a program may prefer that copy; that is how the `--output` and `tsc` problems were found. Run the program with what you add through `internal/engine/optimizers.go` before trusting it, under the sandbox too: check that exit codes survive, that refusals stay visible, and where it writes. A variable that stops it reporting home belongs in `Env`. Never use a program's own agent hook: RTK's answers "allow" when a repository's settings say so (OQ-029).

## Adding a workflow

A workflow (WFL-001) is a file `workflows/<id>.yaml` in the data folder, or an entry in `config.BuiltinWorkflows` to ship it; a file with a built-in's ID replaces it. Unknown fields are refused, and so is any mode that asks less than accept edits:

```yaml
name: Plan, slice, run AFK
description: One line for Settings.
backlog: backlog              # optional: the project folder with prds/ and issues/; never .freeloop, .git or a folder other tools act on
skills: [grill-the-brief]     # the skills its stages use, for Settings to show
stages:
  - command: grill            # the slash command; not one of the app's own (ReservedCommands)
    title: Grill the brief
    description: One line for the palette.
    mode: plan                # optional: ask, plan or accept_edits, switched to before the prompt
    skill: grill-the-brief    # optional: must be installed and on, or the stage refuses
    prompt: "Use the skill grill-the-brief... The brief: {{input}}"
  - command: afk
    title: Run the AFK issues
    kind: afk                 # prompt (default), afk, review or cleanup; one of each but prompt
afk:
  mode: accept_edits          # ask or accept_edits
  sandbox: true               # the sandboxed environment where there is one
  maxIssues: 10
  commands: ["git commit *"]  # shell rule patterns offered to allow up front for the run's sessions
  skill: work-an-issue
  prompt: "Your issue is {{issue}}: {{issueText}} ..."
review:
  profile: reviewer           # optional; the run's profile when empty
  focus: "whether it does what {{issue}} asks"
```

A stage's prompt may use `{{input}}` (what followed the command), `{{backlog}}`, `{{issues}}` (the backlog's issues with their state) and `{{commits}}` (the last ten); the AFK prompt and the review's focus also `{{issue}}` and `{{issueText}}`. Anything else is refused when the file loads, so a typo shows in Settings and not in a prompt.

A file's workflow is used only after the user approves it on the Settings page (`ApproveWorkflow`, a mark in `workflowApprovals`), and any change to the file ends that, because its stages switch modes and its run asks for commands to be allowed. Only a built-in workflow ships skills (`internal/config/workflowskills/`, embedded); "Install skills" copies them into the app's store with `freeloop:workflows/<id>` as their source (`InstallWorkflowSkills`), never over a skill of the same name from elsewhere or one the user edited. A file's workflow names skills the user installs from the Skills page.

A stage's prompt goes through `enqueue` as the user's message with `noMentions`, so its `@` words attach nothing, and it is the same for every harness: no stage may need a harness of its own (HRN-001). The engine's side is `internal/engine/workflows.go`; the composer's is `frontend/src/workflows.ts`.

### The backlog and the AFK run

`internal/backlog` parses issue and PRD files and places each issue (done, ready, blocked by an open or missing issue or a cycle); a file it cannot read counts as open and for a person. `internal/engine/backlog.go` reads a backlog only in a trusted project, through `localEnv`, and archives what is finished (`ArchiveBacklog`: `git rm` and a commit of only those paths that names them; git history is the archive).

An AFK run (`internal/engine/afk.go`, `afkruns.json`) is a list of steps moved on by `runAFK`, which the schedule loop calls between two `runQueue` passes, at its tick and whenever a session's status changes (`KickRuns` from `observe`). Each issue is a queue task (`config.Task` with `RunID`, `Mode`, `EnvironmentID`, `Allow` and `ShareWorktreeOf`), so the queue's limits, slots and budgets apply; the first task makes the run's worktree and the rest share it. `Allow` becomes the session's own allow rules: the commands `PrepareAFK` offered that the user ticked, and those the user typed in (`AFKStart.Extra`, checked by `extraCommand`); nothing else may be in it. Done is read from the issue file in the worktree after the session's turn ends, never from what the agent says. Keep the steps a list: parallel runs (WFL-007) will have several working at once.

## A stuck session (HRN-020)

`internal/engine/stuck.go` follows every saved event through the store's `OnAppend` and says a session is stuck, once a turn, when its account limit is rejected, its turn limit is reached, one tool fails three times in a row (refusals do not count), two rewinds happen in a turn, or two turns in a row end in an error. Anything it emits goes out on its own goroutine, since it runs inside the store's append. `HandOff` stops the old turn, ends its pending requests (`Broker.CancelSession`), continues with HRN-012's `SwitchProfile`, records `handed_off` in the old session, and returns the message that carries the last request over; with a profile's `onLimit` it sends that message itself when the turn ends.

## Adding an event type

Add the constant to `internal/event/event.go` (in `ACPTypes` or `FreeloopTypes`) and to `schemas/events.json`, with a `$defs` entry for its data; the schema test checks both lists agree, and the store refuses unknown types. Fold it into the view in `frontend/src/transcript.ts` with a `node --test` case, and decide whether the search index (`internal/store/search.go`) and the Markdown export (`internal/engine/export.go`) should show it.

## Text in the interface (GUI-048)

Every word Freeloop writes in the window comes from a catalogue in `frontend/src/i18n/`: `en.ts` is the source, `de.ts` and `nl.ts` are typed against it through `translationOf` (`catalogue.ts`). A component calls `t("settings.theme")`, or `t("settings.fontSize", { size })` for a text with a `{placeholder}`; an entry with `one` and `other` forms is a plural, picked by `count` through `Intl.PluralRules`. Text with an element in it, such as `<code>` or a link, goes through `tNodes` (`i18nNodes.tsx`), so each language puts the element where its word order needs it. A table of labels holds catalogue keys (`PlainKey`) and translates where it renders, never at load, so a change of language shows at once. What a model, an agent or a file wrote is shown as given, and so are names: the language names in the picker, file names, tool names, and the modes, commands, workflows, skills and profiles an agent or a file names. The app's own permission modes are named by `modeName(id)` in `commands.ts`, which shows a mode it does not know as given. Text sent to a model or an agent, such as a review's message, stays English. An error from the engine is shown as it came, after a translated heading (`errorHeading` in `errorlist.ts`, or a key such as "Could not list models: {error}").

To add a string, add it to `en.ts` and to `de.ts` and `nl.ts`; `tsc` fails until all three have it with the same placeholders. German and Dutch address no one: an infinitive or the passive instead of du/Sie or je/u (`i18n.test.ts` checks), and Commit, Branch, Token, Prompt and Worktree stay English. `i18nSource.test.ts` reads the components already translated and fails on English written into them; add a component to its list once it is translated. To add a language, add its catalogue, its entry in `LANGUAGES` and `CATALOGUES` (`i18n.ts`), and its value to `config.Languages`.

Numbers, dates and times follow the language too. `locale()` in `i18n.ts` is the language in force as a BCP 47 tag, in the region the system prefers for it when its list has one (`en-GB`, `de-AT`), so a British Mac keeps its 24-hour clock in English. Every `toLocaleString`, `toLocaleDateString`, `toLocaleTimeString` and `Intl` formatter takes it as its first argument, never `[]` or a fixed `"en"`, and `i18nSource.test.ts` fails on one that does not. `numberFormat(options)` is a number formatter for it, kept per locale and options; a number in a `{placeholder}` is written through it. The app's own formatters build on these: `compact` and `formatTokens` (7.9M, 7,9 Mio., 7,9 mln.), `formatCost` (always US dollars, as provider prices are: $146.51, 146,51 $), `timeLabel` for a chart's axis (6:30p, 18:30) and `shortTokens` for a pane's header, which keeps k and M with the locale's decimal sign. A value written with `t()` or a formatter and kept by `useMemo` or `memo` (the transcript's `buildView`, a list of commands, a tool card) needs `locale()` among what it depends on, or it keeps the old language until something else changes.

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
