# Contributing

Thank you for your interest in Freeloop.

**External code contributions open after v1 scoping.** Until then the design is still moving quickly and pull requests from outside are not reviewed or merged. What helps now:

- **Issues.** Bugs, crashes, and agents, providers or MCP servers that do not work as expected. Include the macOS version, how you built the app, and the steps. Leave API keys and other secrets out of logs and transcripts you attach.
- **Security reports.** Do not open a public issue; follow [docs/SECURITY.md](docs/SECURITY.md).
- **Presets and skills.** An ACP agent preset or an Agent Skill that works for you can be shared as a file in an issue; adding either needs no change to the app.

When contributions open, the policy (a Developer Certificate of Origin sign-off is the current recommendation, see OQ-017 in [docs/OPEN_QUESTIONS.md](docs/OPEN_QUESTIONS.md)) and a code of conduct will be added here.

## For the record: how the code is worked on

[docs/DEVELOPING.md](docs/DEVELOPING.md) explains how the code fits together and how to add a provider, a harness preset, an environment or an event type.

- Requirements have stable IDs (`PRV-001`, `SEC-003`…) in [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md); commit messages and pull request titles name the IDs they deliver.
- When the implementation proves a document wrong, the document changes in the same commit. Decisions the documents do not settle go to [docs/OPEN_QUESTIONS.md](docs/OPEN_QUESTIONS.md) with options and a recommendation.
- `make test` and `make lint` pass before every commit. The lint includes an architecture test: no process or file access outside `internal/environment` (and the store, config and platform packages), Wails only in `cmd/freeloop`, macOS-only code only in `internal/platform/darwin`.
- New dependencies are added only after agreement; npm dependencies stay few and pinned.
