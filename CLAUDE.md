# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Freeloop is an open-source macOS desktop app for running AI coding agents with the user's own models, switchable harnesses, Agent Skills, MCP servers and custom environments, built for someone who runs many sessions across many repos at once. Claude Code is the UX inspiration, not the base: the built-in harness is the product, and Claude Code is one external harness among several.

Status: M0 is done. M1 (thin vertical slice) is in progress: slice 1 (the agent loop end to end), slice 2 (OpenAI-compatible and Ollama providers, model discovery, capability probe and overrides), slice 3 (permission rules and "Allow always"), slice 4 (projects, environments with Keychain secrets, git state per session), a hardening pass, slice 5 (session management, nested AGENTS.md, Markdown, settings), slice 6 (favorite and hidden models, `@` file mentions, approval shortcuts) and slice 7 (the right panel, hot reload of the config files) are built; M1's real-provider runs wait for the user's API keys. M2 is in progress: slice 1 (the attention view, GUI-002), slice 2 (the diff and commit panel, GUI-005) slice 3 (harness profiles, HRN-003, HRN-004) slice 4 (slash commands, GUI-004) slice 5 (project trust and `.freeloop/` settings, SEC-006, SET-002) slice 6 (notifications and the dock badge, GUI-008) slice 7 (the first-run guide, GUI-010) slice 8 (skills: discovery, switches, `load_skill`, SKL-001, SKL-002, SKL-004) slice 9 (skill import and install, SKL-003, SKL-005) slice 10 (MCP servers on the official Go SDK, MCP-001, MCP-003, MCP-005) slice 11 (import from Claude Code, MCP-002, SET-003) slice 12 (external ACP harnesses with a Claude Code preset, HRN-004/005/006/008, MCP-004) slice 13 (model switch mid-session, PRV-006) and slice 14 (per-provider concurrency cap, OQ-013) are built. `docs/ROADMAP.md` lists what each covers and what is next. The documents in `docs/` are the specification. When this file and a document disagree, the document wins; fix this file.

## Read first

- `docs/ROADMAP.md`: milestones M0 to M3, each listing the requirement IDs it delivers and a definition of done. Find the current milestone before starting work; its "Not in" line says what to leave alone.
- `docs/ARCHITECTURE.md`: stack and alternatives, component diagram, Go interface sketches for Provider, Harness, Environment, Skill, Session and the permission broker, event schema, data layout, repository layout.
- `docs/REQUIREMENTS.md`: requirements with IDs such as `PRV-001`, priorities MVP / v1 / later, and acceptance criteria written to be testable.
- `docs/OPEN_QUESTIONS.md`: open decisions with a recommendation and the milestone they block; closed decisions with dates.
- `docs/SECURITY.md` (threat model), `docs/RISKS.md`, `docs/VISION.md`.

## Settled decisions (do not reopen without the user)

- Go core; Wails v3 (pinned to v3.0.0 in `go.mod` and `WAILS_VERSION` in the Makefile); React + TypeScript frontend built with Vite, npm as the package manager; macOS 14 minimum (OQ-021); macOS only for now.
- The engine runs in-process behind a transport-agnostic API until v1; the daemon split is EXT-004.
- SQLite through a pure-Go driver for index and metadata, one append-only JSONL file per session for transcripts, YAML for config, secrets only in the macOS Keychain behind the `Secrets` interface.
- ACP (Agent Client Protocol over stdio) is the harness protocol. MCP goes through the official Go SDK targeting the 2026-07-28 revision. Skills use the Agent Skills `SKILL.md` format. Project instructions come from `AGENTS.md` first, `CLAUDE.md` as fallback, both read-only.
- Apache-2.0. No telemetry; the only outbound traffic is to user-configured endpoints plus an opt-in update check.

## Hard rules

- **Vendor boundary (HRN-008).** The app never reads, stores, proxies or forwards vendor OAuth tokens or session cookies. A subscription is used only by spawning the unmodified vendor binary, which performs its own sign-in. The built-in harness uses API keys and local endpoints only. Vendor names appear in plain text, never as logos or in the product name.
- **One chokepoint (SEC-003).** Every side effect from every harness passes through the permission broker. Nothing auto-approves on timeout. Deny rules win at every scope; project-level config can tighten but never loosen.
- **Everything through `Environment` (ENV-001).** No `os/exec` and no direct file I/O in the engine outside `internal/environment`; a lint rule enforces it. External agents' `fs/*` and `terminal/*` protocol requests go through the same layer, so protected paths hold for them too.
- **The GUI never special-cases a harness (HRN-001).** If a feature seems to need harness-specific UI, the `Harness` interface is wrong; fix the interface.
- **No in-process plugins.** Extension points are files (profiles, rules, presets, skills) and processes (ACP harnesses, MCP servers, hooks).
- **No macOS-only code outside `internal/platform/darwin`** (NFR-XPLAT-002).
- **Never write into `~/.claude` or a project's `.claude/`.** Imports from Claude Code are one-way copies with provenance recorded.

## Architecture in brief

`internal/engine` is the only public surface; the Wails service layer and, at v1, a JSON-RPC socket are thin adapters over it. `SessionManager` runs each session as a goroutine group under its own context and process group. A session is driven by a `Harness`: the built-in loop in `internal/harness/builtin`, or the generic ACP client in `internal/harness/acp` configured by a YAML preset (Claude Code is such a preset). Both produce the same versioned events (`schemas/events.json`, a strict superset of ACP `session/update`), both route permission requests through `internal/permission`, and both reach files and processes only through `internal/environment`. The `Store` writes every event to the session's JSONL before publishing it to the event bus and fsyncs dirty transcripts every 200 ms; the UI is a reducer over those events, so replay from disk and live streaming are the same code path. Providers are thin adapters over the official Anthropic, OpenAI and Google SDKs plus one OpenAI-compatible adapter; a per-model capability table (tool choice, parallel calls, stable call IDs) drives what the built-in loop attempts.

## Commands

Prerequisites: Go (Homebrew), Node with npm, Xcode command line tools, and `make tools` once to install the pinned `wails3` CLI into `$(go env GOPATH)/bin`.

- `make dev`: run the app with live reload. It sets `FREELOOP_DEV=1`, which adds a scripted demo provider so the whole loop can be tried without an API key. Set `FREELOOP_HOME=/some/dir` to keep dev data out of `~/Library/Application Support/freeloop`.
- `make build` / `make package`: production binary in `bin/` / ad-hoc signed `bin/Freeloop.app`.
- `make test`: Go tests under `internal/` with the race detector, then frontend tests (`node --test`, no test framework). One Go test: `go test ./internal/<pkg>/ -run '^TestName$'`.
- `make lint`: gofmt, `go vet`, the architecture boundary test (`internal/archtest`: ENV-001, Wails only in `cmd/freeloop`, macOS code only in `internal/platform/darwin`) and `tsc`.
- `make test-keychain`: the Keychain backend against the real login keychain (creates and deletes one item under service `freeloop-test`).
- `make bindings`: regenerate `frontend/bindings` (git-ignored, regenerated on every build) after changing a bound Go service.

Layout notes: the engine composes `internal/environment` (files and processes), `internal/permission` (broker, protected-path policy, rule matching; the engine's `ruleStore` keeps global rules in `rules.yaml` and session rules in transcripts), `internal/provider` (neutral types and the tool-calling probe; adapters `anthropic`, `openaicompat` with presets, `ollama`, `scripted`), `internal/harness/builtin` (the loop and tools), `internal/harness/acp` (the generic ACP client on `coder/acp-go-sdk`, presets in `config.HarnessPreset`, a reference agent in `acp/acptest` for tests), `internal/mcp` (MCP clients on the official SDK; stdio servers start through `environment.Local.Spawn`; the engine side is `internal/engine/mcp.go`), `internal/skill` (Agent Skills parsing and discovery over an environment's file system; the engine side is `internal/engine/skills.go`) `internal/store` (sessions and projects in SQLite, transcripts in JSONL); project trust and `.freeloop/` settings are in `internal/engine/trust.go`, environments are `config.Environment` files and profiles `config.Profile` files (`internal/engine/profiles.go`, `Profiles.tsx`), `internal/engine/watch.go` polls the config files and reloads them (SET-001), and git state comes from `internal/engine/git.go`; the frontend's `src/transcript.ts` folds events into the conversation view and `src/mentions.ts` finds and ranks `@` mentions (the engine's `mentions.go` embeds the files), `src/shortcuts.ts` maps approval keys, `src/commands.ts` handles the composer's slash commands, `SessionPanel.tsx` is the right panel (fed by `panelData` in `transcript.ts`), `Attention.tsx` the attention view over `Engine.Attention`, `Onboarding.tsx` the first-run guide, `DiffPanel.tsx` the Diff tab over `internal/engine/changes.go` with `src/diff.ts` parsing git's unified diff, `src/markdown.ts` parses agent Markdown into a tree that `Markdown.tsx` renders as React elements (never `dangerouslySetInnerHTML`: model output must not reach the DOM as markup, since the page can call the engine); all of them are unit-tested with `node --test`. Bypass mode needs `permissions.allowBypass: true` in `config.yaml`. One Go module; the Wails entry point is `cmd/freeloop` and `EngineService` there is the only Wails-aware code; `frontend/embed.go` embeds `frontend/dist`, so build the frontend before compiling `cmd/freeloop`; the Wails build tasks are in `Taskfile.yml` and `build/`.

## Working conventions

- Put the requirement ID in commit messages and PR titles, for example `SES-002: append-only JSONL writer`. IDs are stable; retire one with `(dropped)`, never renumber.
- When the implementation proves a document wrong, change the document in the same commit.
- A decision you cannot make from the docs goes into `docs/OPEN_QUESTIONS.md` with options and a recommendation; do not decide silently. Close a question with its outcome and the date.
- More than one Claude Code session may be working in this checkout at the same time. Stage only the files you changed; never `git add -A` or `git commit -a`.
- Ask before installing anything system-wide or adding a dependency; keep npm dependencies few and pinned.
